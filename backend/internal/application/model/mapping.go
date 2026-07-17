package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
	modeldomain "github.com/chenyme/grok2api/backend/internal/domain/model"
	"github.com/chenyme/grok2api/backend/internal/repository"
)

type MappingTargetInput struct {
	Provider      account.Provider
	UpstreamModel string
	Priority      int
	Enabled       bool
}

type MappingInput struct {
	ExternalID     string
	Enabled        bool
	EffortOverride string
	Targets        []MappingTargetInput
}

func (s *Service) ListMappings(ctx context.Context) ([]modeldomain.Mapping, error) {
	return s.models.ListMappings(ctx)
}

func (s *Service) GetMapping(ctx context.Context, id uint64) (modeldomain.Mapping, error) {
	if id == 0 {
		return modeldomain.Mapping{}, invalidInput("映射 ID 无效")
	}
	value, err := s.models.GetMapping(ctx, id)
	return value, mapRepositoryError(err)
}

func (s *Service) CreateMapping(ctx context.Context, input MappingInput) (modeldomain.Mapping, error) {
	value, err := s.normalizeMappingInput(0, input)
	if err != nil {
		return modeldomain.Mapping{}, err
	}
	for _, target := range value.Targets {
		if _, ensureErr := s.ensureMappingTargetRoute(ctx, value.ExternalID, target); ensureErr != nil {
			return modeldomain.Mapping{}, ensureErr
		}
	}
	created, err := s.models.CreateMapping(ctx, value)
	return created, mapRepositoryError(err)
}

func (s *Service) UpdateMapping(ctx context.Context, id uint64, input MappingInput) (modeldomain.Mapping, error) {
	if id == 0 {
		return modeldomain.Mapping{}, invalidInput("映射 ID 无效")
	}
	if _, err := s.models.GetMapping(ctx, id); err != nil {
		return modeldomain.Mapping{}, mapRepositoryError(err)
	}
	value, err := s.normalizeMappingInput(id, input)
	if err != nil {
		return modeldomain.Mapping{}, err
	}
	for _, target := range value.Targets {
		if _, ensureErr := s.ensureMappingTargetRoute(ctx, value.ExternalID, target); ensureErr != nil {
			return modeldomain.Mapping{}, ensureErr
		}
	}
	updated, err := s.models.UpdateMapping(ctx, value)
	return updated, mapRepositoryError(err)
}

func (s *Service) DeleteMapping(ctx context.Context, id uint64) error {
	if id == 0 {
		return invalidInput("映射 ID 无效")
	}
	return mapRepositoryError(s.models.DeleteMapping(ctx, id))
}

func (s *Service) normalizeMappingInput(id uint64, input MappingInput) (modeldomain.Mapping, error) {
	externalID, ok := modeldomain.NormalizeExternalID(input.ExternalID)
	if !ok {
		return modeldomain.Mapping{}, invalidInput("externalId 不能为空、不能携带 Provider 前缀，且长度不能超过 255 个字符")
	}
	if len(input.Targets) == 0 {
		return modeldomain.Mapping{}, invalidInput("至少配置一个渠道目标")
	}
	seen := make(map[string]struct{}, len(input.Targets))
	targets := make([]modeldomain.MappingTarget, 0, len(input.Targets))
	enabledCount := 0
	for index, target := range input.Targets {
		if !target.Provider.IsValid() {
			return modeldomain.Mapping{}, invalidInput(fmt.Sprintf("第 %d 个目标的 provider 无效", index+1))
		}
		upstream, validUpstream := modeldomain.NormalizeUpstreamModel(target.Provider, target.UpstreamModel)
		if !validUpstream {
			return modeldomain.Mapping{}, invalidInput(fmt.Sprintf("第 %d 个目标的 upstreamModel 无效", index+1))
		}
		capability := defaultCapabilityForProvider(target.Provider, upstream)
		if _, err := s.validateProviderCapability(target.Provider, capability); err != nil {
			return modeldomain.Mapping{}, err
		}
		key := string(target.Provider) + "\x00" + upstream
		if _, exists := seen[key]; exists {
			return modeldomain.Mapping{}, invalidInput("同一映射内不能重复相同来源与上游模型组合")
		}
		seen[key] = struct{}{}
		priority := target.Priority
		if priority <= 0 {
			priority = index + 1
		}
		if target.Enabled {
			enabledCount++
		}
		targets = append(targets, modeldomain.MappingTarget{
			Provider: target.Provider, UpstreamModel: upstream, Priority: priority, Enabled: target.Enabled,
		})
	}
	if enabledCount == 0 {
		return modeldomain.Mapping{}, invalidInput("至少启用一个渠道目标")
	}
	// 规范化为稳定 1..n，保持相对顺序。
	for i := 0; i < len(targets); i++ {
		for j := i + 1; j < len(targets); j++ {
			if targets[j].Priority < targets[i].Priority {
				targets[i], targets[j] = targets[j], targets[i]
			}
		}
	}
	for index := range targets {
		targets[index].Priority = index + 1
	}
	effort, ok := modeldomain.NormalizeEffortOverride(input.EffortOverride)
	if !ok {
		return modeldomain.Mapping{}, invalidInput("effortOverride 仅支持 low、medium、high，或 max/xhigh（会收口为 high）")
	}
	return modeldomain.Mapping{ID: id, ExternalID: externalID, Enabled: input.Enabled, EffortOverride: effort, Targets: targets}, nil
}

func defaultCapabilityForProvider(providerValue account.Provider, upstream string) modeldomain.Capability {
	if providerValue != account.ProviderWeb {
		return modeldomain.CapabilityResponses
	}
	switch strings.TrimSpace(upstream) {
	case "grok-imagine-image", "grok-imagine-image-quality":
		return modeldomain.CapabilityImage
	case "imagine-image-edit", "grok-imagine-image-edit":
		return modeldomain.CapabilityImageEdit
	case "grok-imagine-video":
		return modeldomain.CapabilityVideo
	default:
		return modeldomain.CapabilityChat
	}
}

// ensureMappingTargetRoute 保证映射目标对应的内部路由存在，供网关与密钥权限使用。
func (s *Service) ensureMappingTargetRoute(ctx context.Context, externalID string, target modeldomain.MappingTarget) (modeldomain.Route, error) {
	publicID, ok := modeldomain.NormalizePublicID(target.Provider, externalID)
	if !ok {
		return modeldomain.Route{}, invalidInput("映射对外名称无效")
	}
	if route, err := s.models.GetByPublicIDIncludingDisabled(ctx, publicID); err == nil {
		if route.Provider == target.Provider && route.UpstreamModel == target.UpstreamModel {
			if !route.Enabled {
				enabled := true
				return s.Update(ctx, route.ID, UpdateInput{Enabled: &enabled})
			}
			return route, nil
		}
	} else if !errors.Is(err, repository.ErrNotFound) {
		return modeldomain.Route{}, err
	}
	if routes, err := s.models.ListByProviderUpstream(ctx, target.Provider, target.UpstreamModel); err == nil && len(routes) > 0 {
		return routes[0], nil
	} else if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return modeldomain.Route{}, err
	}
	return s.Create(ctx, CreateInput{
		PublicID: externalID, Provider: target.Provider, UpstreamModel: target.UpstreamModel,
		Capability: defaultCapabilityForProvider(target.Provider, target.UpstreamModel), Enabled: true,
	})
}

// resolveMappedRoutes 按映射优先级返回当前可用的内部路由。
func (s *Service) resolveMappedRoutes(ctx context.Context, publicModel string) ([]modeldomain.Route, error) {
	routes, _, err := s.resolveMappedRoutesWithEffort(ctx, publicModel)
	return routes, err
}

// MappingEffortOverride 在启用的映射命中时返回 effort 覆写；未配置则返回空串。
func (s *Service) MappingEffortOverride(ctx context.Context, publicModel string) string {
	_, effort, err := s.resolveMappedRoutesWithEffort(ctx, publicModel)
	if err != nil {
		return ""
	}
	return effort
}

func (s *Service) resolveMappedRoutesWithEffort(ctx context.Context, publicModel string) ([]modeldomain.Route, string, error) {
	externalID, ok := modeldomain.NormalizeExternalID(publicModel)
	if !ok {
		return nil, "", repository.ErrNotFound
	}
	mapping, err := s.models.GetMappingByExternalID(ctx, externalID)
	if err != nil {
		return nil, "", err
	}
	if !mapping.Enabled {
		return nil, "", repository.ErrNotFound
	}
	result := make([]modeldomain.Route, 0, len(mapping.Targets))
	seen := make(map[uint64]bool)
	for _, target := range mapping.EnabledTargetsByPriority() {
		route, resolveErr := s.resolveAvailableTargetRoute(ctx, mapping.ExternalID, target)
		if resolveErr != nil {
			if errors.Is(resolveErr, repository.ErrNotFound) {
				continue
			}
			return nil, "", resolveErr
		}
		if seen[route.ID] {
			continue
		}
		seen[route.ID] = true
		result = append(result, route)
	}
	if len(result) == 0 {
		return nil, "", repository.ErrNotFound
	}
	return result, mapping.EffortOverride, nil
}

func (s *Service) resolveAvailableTargetRoute(ctx context.Context, externalID string, target modeldomain.MappingTarget) (modeldomain.Route, error) {
	publicID, ok := modeldomain.NormalizePublicID(target.Provider, externalID)
	if !ok {
		return modeldomain.Route{}, repository.ErrNotFound
	}
	if candidates, err := s.models.GetByPublicIDCandidates(ctx, publicID); err == nil {
		for _, route := range candidates {
			if route.Provider == target.Provider && route.UpstreamModel == target.UpstreamModel {
				return route, nil
			}
		}
	} else if !errors.Is(err, repository.ErrNotFound) {
		return modeldomain.Route{}, err
	}
	routes, err := s.models.ListByProviderUpstream(ctx, target.Provider, target.UpstreamModel)
	if err != nil {
		return modeldomain.Route{}, err
	}
	return routes[0], nil
}
