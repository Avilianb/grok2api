package model

import (
	"strings"
	"time"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
)

// Mapping 描述下游对外模型名到多渠道上游目标的有序映射。
type Mapping struct {
	ID             uint64
	ExternalID     string
	Enabled        bool
	EffortOverride string // 空表示不覆写；仅 low/medium/high
	Targets        []MappingTarget
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// MappingTarget 是映射内的一个渠道目标；Priority 越小越优先。
type MappingTarget struct {
	ID            uint64
	Provider      account.Provider
	UpstreamModel string
	Priority      int
	Enabled       bool
}

// NormalizeExternalID 规范化下游对外模型名：去空白、禁止 Provider 前缀。
func NormalizeExternalID(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || len([]rune(value)) > MaxPublicIDLength {
		return "", false
	}
	for _, providerValue := range account.Providers() {
		prefix := providerValue.ModelNamespace() + "/"
		if len(value) >= len(prefix) && strings.EqualFold(value[:len(prefix)], prefix) {
			return "", false
		}
	}
	return value, true
}

// NormalizeEffortOverride 规范化映射级 effort 覆写；空表示不覆写。
// Claude ultra 的 max / xhigh 收口为 Grok 支持的 high。
func NormalizeEffortOverride(value string) (string, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "", true
	}
	switch value {
	case "minimal", "low":
		return "low", true
	case "medium":
		return "medium", true
	case "high", "xhigh", "max":
		return "high", true
	default:
		return "", false
	}
}

// NormalizeIncomingEffort 将客户端 effort 兼容到 Grok 可接受值；空输入保持为空。
func NormalizeIncomingEffort(value string) string {
	normalized, ok := NormalizeEffortOverride(value)
	if !ok {
		return strings.TrimSpace(value)
	}
	return normalized
}

// EnabledTargetsByPriority 返回启用中的目标，按 priority 升序、id 升序稳定排序。
func (m Mapping) EnabledTargetsByPriority() []MappingTarget {
	targets := make([]MappingTarget, 0, len(m.Targets))
	for _, target := range m.Targets {
		if target.Enabled {
			targets = append(targets, target)
		}
	}
	for i := 0; i < len(targets); i++ {
		for j := i + 1; j < len(targets); j++ {
			if targets[j].Priority < targets[i].Priority || (targets[j].Priority == targets[i].Priority && targets[j].ID < targets[i].ID) {
				targets[i], targets[j] = targets[j], targets[i]
			}
		}
	}
	return targets
}
