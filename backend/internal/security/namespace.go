package security

import "strings"

// Principal 表示一次请求所属的调用方身份。
//
// 默认每个后端 Token 的 SHA-256 前 16 位十六进制（PrincipalID）就是一个独立身份，
// 天然隔离。命名空间用于同一合作方轮换 Token 时让额度池保持连续：新 Token 与旧
// Token 指向同一命名空间，它们的每日额度与并发共用一份预算，而不是各算各的。
type Principal struct {
	// ID 是额度、并发与限流使用的键。命中命名空间时为命名空间名，否则为 Token 哈希。
	ID string
	// Namespace 命中的命名空间名；空字符串表示沿用 Token 默认身份。
	Namespace string
}

// ResolvePrincipal 按命名空间索引解析调用方身份。index 为空时直接沿用 principalID。
func ResolvePrincipal(principalID string, index map[string]string) Principal {
	principalID = strings.TrimSpace(principalID)
	if principalID == "" {
		return Principal{ID: "anonymous"}
	}
	if ns, ok := index[strings.ToLower(principalID)]; ok && ns != "" {
		return Principal{ID: ns, Namespace: ns}
	}
	return Principal{ID: principalID}
}

// BuildNamespaceIndex 把配置展开为「Token 哈希 -> 命名空间名」的索引。
//
// 只有 tokenHash 规则会进入索引。origin 不能参与身份归并：Origin 请求头由客户端
// 单方声明，任何人伪造一个 Origin 即可共用他人额度池、侵占其预算。Origin 只用于
// CORS 白名单判定，不是身份凭证。
func BuildNamespaceIndex(rules []NamespaceRule) map[string]string {
	index := make(map[string]string, len(rules))
	for _, rule := range rules {
		name := strings.TrimSpace(rule.Name)
		hash := strings.ToLower(strings.TrimSpace(rule.TokenHash))
		if name == "" || hash == "" {
			continue
		}
		index[hash] = name
	}
	return index
}
