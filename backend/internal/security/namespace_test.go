package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

func hashOf(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:8])
}

func TestBuildNamespaceIndexOnlyUsesTokenHash(t *testing.T) {
	rules := []NamespaceRule{
		{Name: "studio", TokenHash: "0123456789abcdef"},
		{Name: "", TokenHash: "aaaaaaaaaaaaaaaa"},      // 无名字，忽略
		{Name: "nohash", TokenHash: ""},                // 无哈希，忽略
		{Name: "upper", TokenHash: "BBBBBBBBBBBBBBBB"}, // 统一转小写
		{Name: "dup", TokenHash: "0123456789abcdef"},   // 后者覆盖前者
	}
	idx := BuildNamespaceIndex(rules)

	if got := idx["0123456789abcdef"]; got != "dup" {
		t.Fatalf("duplicate rule should be overridden by later one, got %q", got)
	}
	if _, ok := idx["aaaaaaaaaaaaaaaa"]; ok {
		t.Fatal("rule without name must be ignored")
	}
	if _, ok := idx[""]; ok {
		t.Fatal("rule without tokenHash must be ignored")
	}
	if idx["bbbbbbbbbbbbbbbb"] != "upper" {
		t.Fatal("tokenHash should be normalized to lowercase")
	}
	if len(idx) != 2 {
		t.Fatalf("unexpected index size: %d", len(idx))
	}
}

func TestResolvePrincipal(t *testing.T) {
	studioHash := hashOf("studio-token-at-least-32-characters-xx")
	otherHash := hashOf("other-token-at-least-32-characters-yyy")
	idx := map[string]string{studioHash: "studio"}

	// 命中命名空间
	p := ResolvePrincipal(studioHash, idx)
	if p.ID != "studio" || p.Namespace != "studio" {
		t.Fatalf("unexpected principal: %+v", p)
	}

	// 未配置命名空间的 Token 保持自身哈希，彼此隔离
	p2 := ResolvePrincipal(otherHash, idx)
	if p2.ID != otherHash || p2.Namespace != "" {
		t.Fatalf("unexpected principal: %+v", p2)
	}
	if p2.ID == p.ID {
		t.Fatal("different tokens must never share a quota key")
	}

	// 空索引 / anonymous
	if got := ResolvePrincipal("", nil); got.ID != "anonymous" {
		t.Fatalf("empty principal should fall back to anonymous, got %q", got.ID)
	}

	// 空格不应影响匹配
	if got := ResolvePrincipal("  "+studioHash+" ", idx); got.ID != "studio" {
		t.Fatalf("whitespace should be trimmed, got %q", got.ID)
	}
}

// 同一命名空间下的多个 Token 共享一份额度，而不是各自获得一份
func TestNamespaceSharesDailyQuota(t *testing.T) {
	quota := NewDailyQuotaTracker(2)
	idx := BuildNamespaceIndex([]NamespaceRule{
		{Name: "studio", TokenHash: hashOf("studio-old-token-at-least-32-chars-aaa")},
		{Name: "studio", TokenHash: hashOf("studio-new-token-at-least-32-chars-bbb")},
	})

	oldID := ResolvePrincipal(hashOf("studio-old-token-at-least-32-chars-aaa"), idx).ID
	newID := ResolvePrincipal(hashOf("studio-new-token-at-least-32-chars-bbb"), idx).ID

	if !quota.CheckAndDeduct(oldID) {
		t.Fatal("first call should pass")
	}
	if !quota.CheckAndDeduct(newID) {
		t.Fatal("second call via rotated token should still share the same 2-call budget")
	}
	if quota.CheckAndDeduct(oldID) || quota.CheckAndDeduct(newID) {
		t.Fatal("shared namespace must be exhausted after 2 calls")
	}

	// 无关 Token 不受该命名空间影响
	unrelated := ResolvePrincipal(hashOf("unrelated-token-at-least-32-chars-ccc"), idx).ID
	if unrelated == newID {
		t.Fatal("unrelated token must not share the studio namespace")
	}
	if !quota.CheckAndDeduct(unrelated) {
		t.Fatal("unrelated token should have its own budget")
	}
}

// 命名空间不能靠伪造 Origin 头被侵占：Origin 不参与身份归并
func TestNamespaceIgnoresOriginHeader(t *testing.T) {
	idx := BuildNamespaceIndex([]NamespaceRule{
		{Name: "studio", TokenHash: hashOf("studio-token-at-least-32-characters-xx")},
	})

	// 攻击者令牌即便把自己声明的 Origin 填成 Studio 域名，也不能因此进入 studio 命名空间：
	// NamespaceRule 只有 Name 与 TokenHash，Origin 在类型层面就无从参与归并。
	attacker := ResolvePrincipal(hashOf("attacker-token-at-least-32-chars-yyyy"), idx)
	if attacker.ID == "studio" || attacker.Namespace != "" {
		t.Fatalf("attacker token must not join studio namespace: %+v", attacker)
	}
}

// 配置文件里的 security.namespaces 必须能被 Scan 正确映射到 NamespaceRule
func TestLoadSecurityConfigParsesNamespaces(t *testing.T) {
	content := `{
  "security": {
    "authRequired": true,
    "namespaces": [
      {"name": "studio", "tokenHash": "0123456789abcdef"},
      {"name": "studio", "tokenHash": "FEDCBA9876543210"}
    ]
  }
}`
	adapter, err := gcfg.NewAdapterContent(content)
	if err != nil {
		t.Fatal(err)
	}
	orig := g.Cfg().GetAdapter()
	g.Cfg().SetAdapter(adapter)
	t.Cleanup(func() { g.Cfg().SetAdapter(orig) })

	cfg, err := LoadSecurityConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Namespaces) != 2 {
		t.Fatalf("expected 2 namespace rules, got %d", len(cfg.Namespaces))
	}
	idx := BuildNamespaceIndex(cfg.Namespaces)
	if idx["0123456789abcdef"] != "studio" || idx["fedcba9876543210"] != "studio" {
		t.Fatalf("unexpected namespace index: %+v", idx)
	}
	if got := ResolvePrincipal("0123456789abcdef", idx).ID; got != "studio" {
		t.Fatalf("expected studio namespace, got %q", got)
	}
}
