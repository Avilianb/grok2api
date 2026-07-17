package model

import (
	"testing"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
)

func TestNormalizeExternalID(t *testing.T) {
	t.Parallel()
	if _, ok := NormalizeExternalID("  "); ok {
		t.Fatal("empty external id should be invalid")
	}
	if _, ok := NormalizeExternalID("Build/grok-4.3"); ok {
		t.Fatal("provider-prefixed external id should be invalid")
	}
	if _, ok := NormalizeExternalID("Console/grok-4.3"); ok {
		t.Fatal("console-prefixed external id should be invalid")
	}
	value, ok := NormalizeExternalID("  claude-fable-5  ")
	if !ok || value != "claude-fable-5" {
		t.Fatalf("NormalizeExternalID = %q %v", value, ok)
	}
}

func TestEnabledTargetsByPriority(t *testing.T) {
	t.Parallel()
	mapping := Mapping{
		Targets: []MappingTarget{
			{ID: 3, Provider: account.ProviderBuild, UpstreamModel: "grok-4.5", Priority: 2, Enabled: true},
			{ID: 1, Provider: account.ProviderConsole, UpstreamModel: "grok-4.3", Priority: 1, Enabled: true},
			{ID: 2, Provider: account.ProviderWeb, UpstreamModel: "grok-3", Priority: 1, Enabled: false},
			{ID: 4, Provider: account.ProviderWeb, UpstreamModel: "grok-4", Priority: 1, Enabled: true},
		},
	}
	got := mapping.EnabledTargetsByPriority()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	// priority 1 with lower id first, then priority 2
	if got[0].ID != 1 || got[1].ID != 4 || got[2].ID != 3 {
		t.Fatalf("order = %#v", got)
	}
}

func TestNormalizeEffortOverride(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"", "", true},
		{"low", "low", true},
		{"minimal", "low", true},
		{"medium", "medium", true},
		{"high", "high", true},
		{"xhigh", "high", true},
		{"max", "high", true},
		{"MAX", "high", true},
		{"ultra", "", false},
	}
	for _, tc := range cases {
		got, ok := NormalizeEffortOverride(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("NormalizeEffortOverride(%q) = %q %v, want %q %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	if NormalizeIncomingEffort("max") != "high" {
		t.Fatal("NormalizeIncomingEffort(max) should be high")
	}
}
