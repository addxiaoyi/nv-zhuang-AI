package security

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
)

type SecurityConfig struct {
	AuthRequired        bool              `json:"authRequired"`
	APIKeysEnv          string            `json:"apiKeysEnv"`
	TrustedOrigins      []string          `json:"trustedOrigins"`
	AllowPrivateNetwork bool              `json:"allowPrivateNetwork"`
	RateLimit           RateLimitConfig   `json:"rateLimit"`
	Concurrency         ConcurrencyConfig `json:"concurrency"`
	Namespaces          []NamespaceRule   `json:"namespaces"`
}

type RateLimitConfig struct {
	RequestsPerMinute         int `json:"requestsPerMinute"`
	Burst                     int `json:"burst"`
	LimiterIdleMinutes        int `json:"limiterIdleMinutes"`
	DailyRequestsPerPrincipal int `json:"dailyRequestsPerPrincipal"`
}

type ConcurrencyConfig struct {
	Global       int `json:"global"`
	PerPrincipal int `json:"perPrincipal"`
}

// NamespaceRule 把若干后端 Token 归并到同一个额度命名空间。
//
// 用途：同一合作方轮换 Token（旧 Token 下线、新 Token 上线）时，让每日额度
// 与并发预算保持连续，而不是换一个 Token 就从零重新计数。
//
// 匹配方式为精确等值，不支持通配符：tokenHash 为 Token 的 SHA-256 前 16 位
// 十六进制，与日志中出现的 principal_id 同源。
//
// 命中命名空间后，该 Token 的额度由整个命名空间共享，而不是各自独立。多个
// Token 指向同一命名空间不会被放大成多倍额度；反之，如果把需要独立预算的
// 调用方放进同一命名空间，它们会互相挤占。未配置命名空间的 Token 仍各自独立。
type NamespaceRule struct {
	Name      string `json:"name"`
	TokenHash string `json:"tokenHash"`
}

func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		AuthRequired: true,
		APIKeysEnv:   "TRYON_API_KEYS",
		TrustedOrigins: []string{
			"https://paipiange.github.io",
			"https://studio.0st.top",
			"http://localhost:3000",
			"http://127.0.0.1:3000",
		},
		AllowPrivateNetwork: false,
		RateLimit: RateLimitConfig{
			RequestsPerMinute:         10,
			Burst:                     3,
			LimiterIdleMinutes:        30,
			DailyRequestsPerPrincipal: 100,
		},
		Concurrency: ConcurrencyConfig{
			Global:       2,
			PerPrincipal: 1,
		},
	}
}

func LoadSecurityConfig(ctx context.Context) (*SecurityConfig, error) {
	cfg := DefaultSecurityConfig()
	val, err := g.Cfg().Get(ctx, "security")
	if err != nil || val.IsEmpty() {
		return cfg, nil
	}

	if err := val.Scan(cfg); err != nil {
		return nil, err
	}

	if cfg.APIKeysEnv == "" {
		cfg.APIKeysEnv = "TRYON_API_KEYS"
	}
	if len(cfg.TrustedOrigins) == 0 {
		cfg.TrustedOrigins = DefaultSecurityConfig().TrustedOrigins
	}
	if cfg.RateLimit.RequestsPerMinute <= 0 {
		cfg.RateLimit.RequestsPerMinute = 10
	}
	if cfg.RateLimit.Burst <= 0 {
		cfg.RateLimit.Burst = 3
	}
	if cfg.RateLimit.LimiterIdleMinutes <= 0 {
		cfg.RateLimit.LimiterIdleMinutes = 30
	}
	if cfg.RateLimit.DailyRequestsPerPrincipal <= 0 {
		cfg.RateLimit.DailyRequestsPerPrincipal = 100
	}
	if cfg.Concurrency.Global <= 0 {
		cfg.Concurrency.Global = 2
	}
	if cfg.Concurrency.PerPrincipal <= 0 {
		cfg.Concurrency.PerPrincipal = 1
	}
	if cfg.Namespaces == nil {
		cfg.Namespaces = []NamespaceRule{}
	}

	return cfg, nil
}
