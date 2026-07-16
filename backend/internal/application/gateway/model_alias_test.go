package gateway

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/domain/audit"
	modeldomain "github.com/chenyme/grok2api/backend/internal/domain/model"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
	"github.com/chenyme/grok2api/backend/internal/repository"
)

func TestRewriteAliasedModelAppliesOperationEffort(t *testing.T) {
	publicModel := "grok-4.3"
	tests := []struct {
		name      string
		operation audit.Operation
		assert    func(*testing.T, map[string]any)
	}{
		{name: "responses", operation: audit.OperationResponses, assert: func(t *testing.T, payload map[string]any) {
			reasoning, _ := payload["reasoning"].(map[string]any)
			if reasoning["effort"] != "high" {
				t.Fatalf("reasoning = %#v", reasoning)
			}
		}},
		{name: "chat", operation: audit.OperationChat, assert: func(t *testing.T, payload map[string]any) {
			if payload["reasoning_effort"] != "high" {
				t.Fatalf("reasoning_effort = %#v", payload["reasoning_effort"])
			}
		}},
		{name: "messages", operation: audit.OperationMessages, assert: func(t *testing.T, payload map[string]any) {
			config, _ := payload["output_config"].(map[string]any)
			if config["effort"] != "high" {
				t.Fatalf("output_config = %#v", config)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := rewriteAliasedModel([]byte(`{"model":"grok-4.3-high"}`), publicModel, "high", test.operation)
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			if payload["model"] != publicModel {
				t.Fatalf("model = %#v", payload["model"])
			}
			test.assert(t, payload)
		})
	}
}

func TestResolveAliasRoutesKeepsDeclaredPublicRouteWhenUpstreamIsShared(t *testing.T) {
	canonical := modeldomain.Route{
		ID: 1, PublicID: "Build/gpt-5.6-sol", Provider: account.ProviderBuild, UpstreamModel: "grok-4.5",
	}
	resolver := &aliasRouteResolver{
		candidates: map[string][]modeldomain.Route{canonical.PublicID: {canonical}},
	}
	registry := provider.NewRegistry(aliasRouteAdapter{aliases: []provider.ModelAlias{{
		Alias: "gpt-5.6-sol-high", PublicModel: canonical.PublicID,
		Provider: account.ProviderBuild, UpstreamModel: "grok-4.5", ReasoningEffort: "high",
	}}})
	service := &Service{models: resolver, providers: registry}

	routes, effort, err := service.resolvePublicModelRoutes(context.Background(), "gpt-5.6-sol-high")
	if err != nil {
		t.Fatal(err)
	}
	if effort != "high" || len(routes) != 1 || routes[0].ID != canonical.ID {
		t.Fatalf("alias routes = %#v, effort = %q", routes, effort)
	}
}

type aliasRouteResolver struct {
	candidates map[string][]modeldomain.Route
}

func (r *aliasRouteResolver) Get(context.Context, uint64) (modeldomain.Route, error) {
	return modeldomain.Route{}, repository.ErrNotFound
}

func (r *aliasRouteResolver) GetByPublicID(ctx context.Context, publicID string) (modeldomain.Route, error) {
	values, err := r.GetByPublicIDCandidates(ctx, publicID)
	if err != nil {
		return modeldomain.Route{}, err
	}
	return values[0], nil
}

func (r *aliasRouteResolver) GetByPublicIDCandidates(_ context.Context, publicID string) ([]modeldomain.Route, error) {
	values, ok := r.candidates[publicID]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return values, nil
}

type aliasRouteAdapter struct{ aliases []provider.ModelAlias }

func (aliasRouteAdapter) Provider() account.Provider { return account.ProviderBuild }

func (a aliasRouteAdapter) ModelAliases() []provider.ModelAlias { return a.aliases }
