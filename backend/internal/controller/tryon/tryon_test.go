package tryon

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"backend/api/tryon/v1"
	"backend/internal/middleware"
	"backend/internal/security"
)

func createTestPNG(w, h int) string {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestControllerTryonImageValidation(t *testing.T) {
	// 启动测试服务
	s := g.Server("test-ctrl-tryon")
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.RequestID)
		group.Bind(NewV1())
	})
	s.SetPort(0)
	_ = s.Start()
	defer s.Shutdown()

	port := strings.Split(s.GetListenedAddress(), ":")[1]
	client := &http.Client{}

	// 1. 无效 Base64
	badReq := v1.TryonReq{
		PersonImage:  "!!not-valid-base64!!",
		GarmentImage: createTestPNG(300, 300),
	}
	badBody, _ := json.Marshal(badReq)
	req1, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+port+"/tryon", bytes.NewReader(badBody))
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := client.Do(req1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != 422 {
		t.Fatalf("expected 422, got %d", resp1.StatusCode)
	}
	b1, _ := io.ReadAll(resp1.Body)
	var res1 struct {
		Code int `json:"code"`
		Data struct {
			ErrorCode string `json:"error_code"`
		} `json:"data"`
	}
	_ = json.Unmarshal(b1, &res1)
	if res1.Code != 42201 || res1.Data.ErrorCode != "INVALID_IMAGE_BASE64" {
		t.Fatalf("unexpected res1: %+v", res1)
	}

	// 2. 文本图片（不支持类型）
	textB64 := base64.StdEncoding.EncodeToString([]byte("plain text not image"))
	textReq := v1.TryonReq{
		PersonImage:  textB64,
		GarmentImage: createTestPNG(300, 300),
	}
	textBody, _ := json.Marshal(textReq)
	req2, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+port+"/tryon", bytes.NewReader(textBody))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 422 {
		t.Fatalf("expected 422, got %d", resp2.StatusCode)
	}
	b2, _ := io.ReadAll(resp2.Body)
	var res2 struct {
		Code int `json:"code"`
		Data struct {
			ErrorCode string `json:"error_code"`
		} `json:"data"`
	}
	_ = json.Unmarshal(b2, &res2)
	if res2.Code != 42202 || res2.Data.ErrorCode != "UNSUPPORTED_IMAGE_TYPE" {
		t.Fatalf("unexpected res2: %+v", res2)
	}

	// 3. 尺寸过小
	smallB64 := createTestPNG(100, 100)
	smallReq := v1.TryonReq{
		PersonImage:  smallB64,
		GarmentImage: createTestPNG(300, 300),
	}
	smallBody, _ := json.Marshal(smallReq)
	req3, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+port+"/tryon", bytes.NewReader(smallBody))
	req3.Header.Set("Content-Type", "application/json")
	resp3, err := client.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != 422 {
		t.Fatalf("expected 422, got %d", resp3.StatusCode)
	}
	b3, _ := io.ReadAll(resp3.Body)
	var res3 struct {
		Code int `json:"code"`
		Data struct {
			ErrorCode string `json:"error_code"`
		} `json:"data"`
	}
	_ = json.Unmarshal(b3, &res3)
	if res3.Code != 42204 || res3.Data.ErrorCode != "IMAGE_DIMENSIONS_INVALID" {
		t.Fatalf("unexpected res3: %+v", res3)
	}
}

func TestControllerTryonQuotaAndConcurrency(t *testing.T) {
	secService := security.GetService()
	// 设置配额为 1 进行测试
	secService.DailyQuotaTracker = security.NewDailyQuotaTracker(1)
	secService.ConcurrencyManager = security.NewConcurrencyManager(1, 1)

	s := g.Server("test-ctrl-quota")
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.RequestID)
		group.Bind(NewV1())
	})
	s.SetPort(0)
	_ = s.Start()
	defer s.Shutdown()

	port := strings.Split(s.GetListenedAddress(), ":")[1]
	client := &http.Client{}

	validPerson := createTestPNG(300, 300)
	validGarment := createTestPNG(300, 300)

	// 先直接耗尽配额
	secService.DailyQuotaTracker.CheckAndDeduct("anonymous")

	// 发起请求，应当直接触发 DAILY_QUOTA_EXCEEDED (42902)
	tryReq := v1.TryonReq{
		PersonImage:  validPerson,
		GarmentImage: validGarment,
	}
	reqBody, _ := json.Marshal(tryReq)
	req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+port+"/tryon", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	var res struct {
		Code int `json:"code"`
		Data struct {
			ErrorCode string `json:"error_code"`
		} `json:"data"`
	}
	_ = json.Unmarshal(b, &res)
	if res.Code != 42902 || res.Data.ErrorCode != "DAILY_QUOTA_EXCEEDED" {
		t.Fatalf("unexpected quota res: %+v", res)
	}
}

func TestControllerTryonServerSideRejectsClientConfig(t *testing.T) {
	secService := security.GetService()
	// 赋予足够配额与并发，确保请求能被放行到服务间规则校验阶段
	secService.DailyQuotaTracker = security.NewDailyQuotaTracker(100)
	secService.ConcurrencyManager = security.NewConcurrencyManager(4, 4)

	s := g.Server("test-ctrl-server-side")
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.RequestID)
		group.Bind(NewV1())
	})
	s.SetPort(0)
	_ = s.Start()
	defer s.Shutdown()

	port := strings.Split(s.GetListenedAddress(), ":")[1]
	client := &http.Client{}

	// 1. server_side 与 client_config 同时出现，必须拒绝且不外发个人 Key
	conflictReq := v1.TryonReq{
		PersonImage:  createTestPNG(300, 300),
		GarmentImage: createTestPNG(300, 300),
		ServerSide:   true,
		ClientConfig: &v1.ClientModelConfig{
			Provider: "gemini",
			BaseURL:  "https://example.invalid",
			APIKey:   "test-placeholder-key-not-real",
			Model:    "gemini-3.1-flash-image",
		},
	}
	conflictBody, _ := json.Marshal(conflictReq)
	req1, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+port+"/tryon", bytes.NewReader(conflictBody))
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := client.Do(req1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp1.StatusCode)
	}
	b1, _ := io.ReadAll(resp1.Body)
	var res1 struct {
		Code int `json:"code"`
		Data struct {
			ErrorCode string `json:"error_code"`
		} `json:"data"`
	}
	_ = json.Unmarshal(b1, &res1)
	if res1.Code != 40001 || res1.Data.ErrorCode != "CLIENT_CONFIG_NOT_ALLOWED" {
		t.Fatalf("unexpected conflict res: %+v", res1)
	}

	// 2. server_side 无服务端预设 Token 时明确拒绝，不允许无鉴权占用模型额度
	noTokenReq := v1.TryonReq{
		PersonImage:  createTestPNG(300, 300),
		GarmentImage: createTestPNG(300, 300),
		ServerSide:   true,
	}
	noTokenBody, _ := json.Marshal(noTokenReq)
	req2, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+port+"/tryon", bytes.NewReader(noTokenBody))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp2.StatusCode)
	}
	b2, _ := io.ReadAll(resp2.Body)
	var res2 struct {
		Code int `json:"code"`
		Data struct {
			ErrorCode string `json:"error_code"`
		} `json:"data"`
	}
	_ = json.Unmarshal(b2, &res2)
	if res2.Code != 50301 || res2.Data.ErrorCode != "SERVICE_CALL_NOT_CONFIGURED" {
		t.Fatalf("unexpected no-token res: %+v", res2)
	}
}

// 服务端 Token 已就绪、但所选模型没有任何服务端配置时，必须返回明确的 50302，
// 而不是让请求落进适配层变成不透明的 500；错误信息也不得回显任何服务端 Token。
func TestControllerTryonServerSideModelNotConfigured(t *testing.T) {
	const token = "test-server-side-token-0123456789abcdef"
	t.Setenv("MODEL_NOT_READY_KEY", token)

	secService := security.GetService()
	store, err := security.NewTokenStore(true, "MODEL_NOT_READY_KEY")
	if err != nil {
		t.Fatalf("构建 TokenStore 失败: %v", err)
	}
	originalStore := secService.TokenStore
	secService.TokenStore = store
	t.Cleanup(func() { secService.TokenStore = originalStore })
	// 赋予足够配额与并发，确保请求能被放行到服务间规则校验阶段
	originalQuota, originalConc := secService.DailyQuotaTracker, secService.ConcurrencyManager
	secService.DailyQuotaTracker = security.NewDailyQuotaTracker(100)
	secService.ConcurrencyManager = security.NewConcurrencyManager(4, 4)
	t.Cleanup(func() {
		secService.DailyQuotaTracker = originalQuota
		secService.ConcurrencyManager = originalConc
	})

	s := g.Server("test-ctrl-model-not-configured")
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.RequestID, middleware.NewAuthMiddleware(store).Handler)
		group.Bind(NewV1())
	})
	s.SetPort(0)
	_ = s.Start()
	defer s.Shutdown()

	port := strings.Split(s.GetListenedAddress(), ":")[1]
	req := v1.TryonReq{
		PersonImage:  createTestPNG(300, 300),
		GarmentImage: createTestPNG(300, 300),
		ServerSide:   true,
		Model:        "model-that-has-no-config",
	}
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+port+"/tryon", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{}).Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	var res struct {
		Code int `json:"code"`
		Data struct {
			ErrorCode string `json:"error_code"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &res)
	if res.Code != 50302 || res.Data.ErrorCode != "MODEL_NOT_CONFIGURED" {
		t.Fatalf("unexpected res: %+v", res)
	}
	if bytes.Contains(raw, []byte(token)) {
		t.Fatalf("响应体泄露了服务端 Token")
	}
}

// 浏览器兼容路径（非 server_side）同样要返回结构化 50302，
// 不能落到 HTTP 200 + 通用 code:50，更不能回显服务端配置路径。
func TestControllerTryonBrowserPathModelNotConfigured(t *testing.T) {
	secService := security.GetService()
	originalQuota, originalConc := secService.DailyQuotaTracker, secService.ConcurrencyManager
	secService.DailyQuotaTracker = security.NewDailyQuotaTracker(100)
	secService.ConcurrencyManager = security.NewConcurrencyManager(4, 4)
	t.Cleanup(func() {
		secService.DailyQuotaTracker = originalQuota
		secService.ConcurrencyManager = originalConc
	})

	s := g.Server("test-ctrl-browser-model-missing")
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.RequestID)
		group.Bind(NewV1())
	})
	s.SetPort(0)
	_ = s.Start()
	defer s.Shutdown()

	port := strings.Split(s.GetListenedAddress(), ":")[1]
	req := v1.TryonReq{
		PersonImage:  createTestPNG(300, 300),
		GarmentImage: createTestPNG(300, 300),
		Model:        "model-that-has-no-config",
	}
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+port+"/tryon", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{}).Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", resp.StatusCode, raw)
	}
	var res struct {
		Code int `json:"code"`
		Data struct {
			ErrorCode string `json:"error_code"`
			RequestID string `json:"request_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &res)
	if res.Code != 50302 || res.Data.ErrorCode != "MODEL_NOT_CONFIGURED" {
		t.Fatalf("unexpected res: %+v", res)
	}
	if res.Data.RequestID == "" {
		t.Fatal("expected request_id in error response")
	}
	// 不得回显内部配置路径
	if strings.Contains(string(raw), "llm.models.") {
		t.Fatalf("响应体泄露了内部配置路径: %s", raw)
	}
}

func init() {
	// 确保测试环境未设置任何破坏性环境变量
	os.Unsetenv("TRYON_API_KEYS")
}
