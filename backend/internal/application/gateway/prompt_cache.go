package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/domain/audit"
)

const promptCacheIdentityVersion = "v1"

// resolvePromptCacheIdentity 将客户端缓存键或会话标识转换为固定长度的上游身份。
// 客户端、Provider、上游模型和协议共同参与摘要，防止共享账号池中的跨租户碰撞。
// 客户端未提供会话 seed 时回退到 client-key 稳定身份，避免每次请求随机
// x-grok-session-id 导致账号不粘滞、上游 prompt cache 永远 miss。
func resolvePromptCacheIdentity(clientKeyID uint64, provider accountdomain.Provider, upstreamModel string, operation audit.Operation, explicitKey, sessionSeed string) string {
	seed := strings.TrimSpace(explicitKey)
	if seed == "" {
		seed = strings.TrimSpace(sessionSeed)
	}
	if seed == "" && clientKeyID != 0 {
		seed = fmt.Sprintf("client-key:%d", clientKeyID)
	}
	model := strings.ToLower(strings.TrimSpace(upstreamModel))
	if clientKeyID == 0 || provider == "" || model == "" || seed == "" {
		return ""
	}
	if operation == "" {
		operation = audit.OperationResponses
	}
	source := fmt.Sprintf("grok2api:prompt-cache:%s:%d:%s:%s:%s:%s", promptCacheIdentityVersion, clientKeyID, provider, model, operation, seed)
	digest := sha256.Sum256([]byte(source))
	hexID := hex.EncodeToString(digest[:16])
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexID[0:8], hexID[8:12], hexID[12:16], hexID[16:20], hexID[20:32])
}
