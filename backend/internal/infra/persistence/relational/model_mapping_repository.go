package relational

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/domain/model"
	"github.com/chenyme/grok2api/backend/internal/repository"
	"gorm.io/gorm"
)

func (r *ModelRepository) GetByProviderUpstream(ctx context.Context, provider account.Provider, upstreamModel string) (model.Route, error) {
	values, err := r.ListByProviderUpstream(ctx, provider, upstreamModel)
	if err != nil {
		return model.Route{}, err
	}
	return values[0], nil
}

func (r *ModelRepository) ListByProviderUpstream(ctx context.Context, provider account.Provider, upstreamModel string) ([]model.Route, error) {
	upstreamModel = strings.TrimSpace(upstreamModel)
	if !provider.IsValid() || upstreamModel == "" {
		return nil, repository.ErrNotFound
	}
	var rows []modelRouteModel
	if err := r.availableRoutes(r.db.db.WithContext(ctx)).
		Where("provider = ? AND upstream_model = ? AND enabled = ?", provider, upstreamModel, true).
		Order("id ASC").Find(&rows).Error; err != nil {
		return nil, mapError(err)
	}
	if len(rows) == 0 {
		return nil, repository.ErrNotFound
	}
	return mapModelRows(rows), nil
}

func (r *ModelRepository) ListMappings(ctx context.Context) ([]model.Mapping, error) {
	var rows []publicModelMappingModel
	if err := r.db.db.WithContext(ctx).Order("external_id ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return r.loadMappings(ctx, rows)
}

func (r *ModelRepository) GetMapping(ctx context.Context, id uint64) (model.Mapping, error) {
	var row publicModelMappingModel
	if err := r.db.db.WithContext(ctx).First(&row, id).Error; err != nil {
		return model.Mapping{}, mapError(err)
	}
	values, err := r.loadMappings(ctx, []publicModelMappingModel{row})
	if err != nil {
		return model.Mapping{}, err
	}
	return values[0], nil
}

func (r *ModelRepository) GetMappingByExternalID(ctx context.Context, externalID string) (model.Mapping, error) {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return model.Mapping{}, repository.ErrNotFound
	}
	var row publicModelMappingModel
	if err := r.db.db.WithContext(ctx).Where("external_id = ?", externalID).First(&row).Error; err != nil {
		return model.Mapping{}, mapError(err)
	}
	values, err := r.loadMappings(ctx, []publicModelMappingModel{row})
	if err != nil {
		return model.Mapping{}, err
	}
	return values[0], nil
}

func (r *ModelRepository) CreateMapping(ctx context.Context, value model.Mapping) (model.Mapping, error) {
	row := publicModelMappingModel{ExternalID: value.ExternalID, Enabled: value.Enabled, EffortOverride: value.EffortOverride}
	err := r.db.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return mapError(err)
		}
		return replaceMappingTargets(tx, row.ID, value.Targets)
	})
	if err != nil {
		return model.Mapping{}, err
	}
	return r.GetMapping(ctx, row.ID)
}

func (r *ModelRepository) UpdateMapping(ctx context.Context, value model.Mapping) (model.Mapping, error) {
	err := r.db.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing publicModelMappingModel
		if err := tx.First(&existing, value.ID).Error; err != nil {
			return mapError(err)
		}
		updates := map[string]any{"external_id": value.ExternalID, "enabled": value.Enabled, "effort_override": value.EffortOverride}
		if err := tx.Model(&publicModelMappingModel{}).Where("id = ?", value.ID).Updates(updates).Error; err != nil {
			return mapError(err)
		}
		return replaceMappingTargets(tx, value.ID, value.Targets)
	})
	if err != nil {
		return model.Mapping{}, err
	}
	return r.GetMapping(ctx, value.ID)
}

func (r *ModelRepository) DeleteMapping(ctx context.Context, id uint64) error {
	return r.db.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("mapping_id = ?", id).Delete(&publicModelMappingTargetModel{}).Error; err != nil {
			return err
		}
		result := tx.Delete(&publicModelMappingModel{}, id)
		if result.Error != nil {
			return mapError(result.Error)
		}
		if result.RowsAffected == 0 {
			return repository.ErrNotFound
		}
		return nil
	})
}

func replaceMappingTargets(tx *gorm.DB, mappingID uint64, targets []model.MappingTarget) error {
	if err := tx.Where("mapping_id = ?", mappingID).Delete(&publicModelMappingTargetModel{}).Error; err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	rows := make([]publicModelMappingTargetModel, 0, len(targets))
	for _, target := range targets {
		rows = append(rows, publicModelMappingTargetModel{
			MappingID: mappingID, Provider: string(target.Provider), UpstreamModel: target.UpstreamModel,
			Priority: target.Priority, Enabled: target.Enabled,
		})
	}
	return tx.Create(&rows).Error
}

func (r *ModelRepository) loadMappings(ctx context.Context, rows []publicModelMappingModel) ([]model.Mapping, error) {
	if len(rows) == 0 {
		return []model.Mapping{}, nil
	}
	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	var targets []publicModelMappingTargetModel
	if err := r.db.db.WithContext(ctx).Where("mapping_id IN ?", ids).Order("priority ASC, id ASC").Find(&targets).Error; err != nil {
		return nil, err
	}
	byMapping := make(map[uint64][]model.MappingTarget, len(rows))
	for _, target := range targets {
		byMapping[target.MappingID] = append(byMapping[target.MappingID], model.MappingTarget{
			ID: target.ID, Provider: account.Provider(target.Provider), UpstreamModel: target.UpstreamModel,
			Priority: target.Priority, Enabled: target.Enabled,
		})
	}
	result := make([]model.Mapping, 0, len(rows))
	for _, row := range rows {
		result = append(result, model.Mapping{
			ID: row.ID, ExternalID: row.ExternalID, Enabled: row.Enabled, EffortOverride: row.EffortOverride,
			Targets: byMapping[row.ID], CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
	}
	return result, nil
}

// aliasOwnedBySameUpstream reports whether publicID is reserved as an alias for a route
// that already serves the same provider+upstream pair.
func aliasOwnedBySameUpstream(tx *gorm.DB, publicID string, provider account.Provider, upstreamModel string) (bool, error) {
	var alias modelRouteAliasModel
	if err := tx.Where("alias = ?", publicID).First(&alias).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	var route modelRouteModel
	if err := tx.First(&route, alias.ModelRouteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return route.Provider == string(provider) && route.UpstreamModel == upstreamModel, nil
}

func ensureDiscoveredPublicID(tx *gorm.DB, publicID string, provider account.Provider, upstreamModel string) error {
	if err := ensureModelPublicIDNotAlias(tx, publicID, 0); err == nil {
		return nil
	} else if !errors.Is(err, repository.ErrConflict) {
		return err
	}
	same, lookupErr := aliasOwnedBySameUpstream(tx, publicID, provider, upstreamModel)
	if lookupErr != nil {
		return lookupErr
	}
	if same {
		return nil
	}
	return fmt.Errorf("%w: 模型公开 ID %q 已被其他路由保留为兼容名称", repository.ErrConflict, publicID)
}
