package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"backend/internal/security"
)

const CtxRequestID = "request_id"
const CtxPrincipalID = "principal_id"

func GetRequestID(r *ghttp.Request) string {
	if val := r.GetCtxVar(CtxRequestID); !val.IsEmpty() {
		return val.String()
	}
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	return ""
}

func GenerateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x%s", time.Now().UnixNano(), hex.EncodeToString(b))
}

func WriteAPIError(r *ghttp.Request, err *security.APIError) {
	reqID := GetRequestID(r)
	r.Response.Header().Set("Content-Type", "application/json; charset=utf-8")
	if reqID != "" {
		r.Response.Header().Set("X-Request-ID", reqID)
	}
	r.Response.WriteHeader(err.HTTPStatus)
	r.Response.WriteJson(g.Map{
		"code":    err.BizCode,
		"message": err.Message,
		"data": g.Map{
			"error_code": err.ErrorCode,
			"request_id": reqID,
		},
	})
	r.ExitAll()
}

// Recover 兜住内层中间件/handler 的 panic，返回统一的 JSON 错误结构。
// 日志只记录错误摘要与 request_id，不记录请求体（可能含图片与个人 Key）。
// 注意：GoFrame 会在 middleware.Next 中补栈重新抛出 panic，因此本中间件
// 必须注册在中间件链最前（紧跟 RequestID），才能最先拿到内层 panic 并收口。
func Recover(r *ghttp.Request) {
	defer func() {
		if exception := recover(); exception != nil {
			reqID := GetRequestID(r)
			g.Log().Errorf(r.Context(), "panic recovered: %v, request_id=%s", exception, reqID)
			r.Response.ClearBuffer()
			r.Response.Header().Set("Content-Type", "application/json; charset=utf-8")
			if reqID != "" {
				r.Response.Header().Set("X-Request-ID", reqID)
			}
			r.Response.WriteHeader(http.StatusInternalServerError)
			r.Response.WriteJson(g.Map{
				"code":    security.ErrServiceInternal.BizCode,
				"message": security.ErrServiceInternal.Message,
				"data": g.Map{
					"error_code": security.ErrServiceInternal.ErrorCode,
					"request_id": reqID,
				},
			})
			r.ExitAll()
		}
	}()
	r.Middleware.Next()
}
