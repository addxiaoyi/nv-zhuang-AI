# 与 Studio（Helstera）集成指南

本文档说明如何把「拍片哥女装」提供的 AI 换装能力（`POST /tryon`）接入另一个项目 **Studio（Helstera）**，
作为其电商图 / 虚拟试穿（`16-try-on-virtual`）场景的生成后端。

> **授权前提**：Studio 采用 **Helstera 专有商业许可证**（`Loomic/LICENSE`），禁止未经授权的复制、修改与商用。
> 本指南只描述**服务间调用**方式——本仓库不改动 Studio 任何源码，接入改由 Studio 的授权方自行完成。
> 在实施前请先取得 Helstera 的书面商业许可（`legal@helstera.com`）。

---

## 1. 为什么不能「代码合并」

| 维度 | 本仓库 | Studio (Helstera) |
|---|---|---|
| 前端 | 纯静态 HTML + `data.js` | Next.js 16 + React 19（静态导出） |
| 后端 | GoFrame v2（Go） | Fastify 5 + LangGraph（TypeScript） |
| 数据库 | 无 | PostgreSQL 17 + Redis 8.6（Redis Stream） |
| 计费 | 令牌桶限流 + 每日额度 | 积分余额 / 交易流水 / 阶梯计费 |
| 图像生成 | 直连 OpenAI / 火山方舟 / Gemini / ComfyUI | 服务端渠道配置 + 适配器 |
| 许可证 | MIT | 专有商业许可 |

二者在语言、运行时、数据模型与许可证上都不同，**合并代码**既无收益也有法律风险。
正确做法是让 Studio 把本服务的 `/tryon` 当作**一个受信任的上游能力**调用。

---

## 2. 集成方式：服务间调用（`server_side`）

新增一个仅供**服务端之间**使用的调用标记：请求体带 `"server_side": true`。

```
Studio (Fastify)  ──HTTPS──►  nvzhuang 后端 (/tryon)  ──►  大模型 / 本地 ComfyUI
```

### 2.1 安全约束（服务端强制执行）

| 约束 | 说明 | 触发时的错误 |
|---|---|---|
| 必须携带有效后端 Token | `Authorization: Bearer <token>`，Token 来自环境变量 `TRYON_API_KEYS`（32~512 字符，逗号分隔）。仍走恒定时间比对与 SHA-256 摘要。 | `401 AUTH_REQUIRED` / `401 AUTH_INVALID` |
| 禁止个人模型配置 | 请求体若同时出现 `client_config`，直接拒绝，**绝不转发**其中的 Key / Base URL。 | `400 CLIENT_CONFIG_NOT_ALLOWED` |
| 必须已配置服务端模型 | 环境变量中没有任何 Token 时，服务端没有可用的预设模型，直接拒绝而不是回落为匿名调用。 | `503 SERVICE_CALL_NOT_CONFIGURED` |
| 沿用全部原有防护 | 图片真实性探测、字节/像素上限、限流、每日额度、全局与单主体并发、CORS 白名单。 | `413 / 422 / 429 / 403` |

> 说明：`server_side` 保证的是「**只会使用服务端预设的模型，永远不会转发你的个人 Key**」；
> 真正的身份隔离来自 `security.authRequired: true` + 为 Studio 单独签发一个 Token。
> 每个 Token 的 SHA-256 前 16 位即独立的额度键，默认与其它 Token 互不共享。
> 生产环境请在 `security.trustedOrigins` 中把 Studio 的域名加入白名单。本仓库已在
> `backend/manifest/config/config.example.yaml` 与 `DefaultSecurityConfig()` 的默认值中加入 `https://studio.0st.top`；
> 若 Studio 后续更换域名，两处都要同步修改（白名单为精确匹配，端口与协议必须一致）。

### 2.1.1 额度命名空间（可选）

默认情况下**每个 Token 各自独立计数**，互不挤占：`principal_id` 是该 Token 的 SHA-256 前 16 位，
每 Token 每天独立享有 `dailyRequestsPerPrincipal` 次额度。这对 Studio 已经够用——给它单独签发
一个 Token 即可，不需要任何额外配置。

只有当**同一合作方需要轮换 Token，且希望额度池在换 Token 时不清零**时，才需要开启命名空间：

```yaml
security:
  authRequired: true
  namespaces:
    - name: "studio"                    # 命名空间名 = 额度键
      tokenHash: "0123456789abcdef"    # Studio 旧 Token 的 SHA-256 前 16 位
    - name: "studio"
      tokenHash: "fedcba9876543210"    # Studio 新 Token，与旧 Token 共用同一份额度
```

`tokenHash` 的算法是 `sha256(token)` 取前 16 位十六进制，与服务端日志中打印的 `principal_id` 完全一致：
在日志里搜到 Studio 请求的 `principal_id`，把它填进 `tokenHash` 即可，**不需要在配置里写明文 Token**。

需要注意的边界：

- 命名空间是**共享**预算，不是叠加预算。两个 Token 指同一个 `name`，它们合起来仍只有一份每日额度，
  不会变成两倍。
- 只认 `tokenHash` 精确匹配。`Origin` 请求头由客户端单方声明、可随意伪造，**不参与身份归并**，
  否则任何人加一个 `Origin: https://studio.0st.top` 就能蹭走 Studio 的额度。
- 未配置命名空间的 Token 行为完全不变，仍是各自独立计数。

### 2.2 建议的部署形态

```
public:  https://tryon.your-domain.com   （nvzhuang 后端，authRequired: true，只对 Studio 服务端开放）
Studio:  在服务端配置 TRYON_BASE_URL + TRYON_API_TOKEN，仅供后端进程读取
```

- **不要**把 Token 放到 Studio 的 `NEXT_PUBLIC_*` 变量里，也不要让浏览器直连本服务。
- 本服务的 CORS 白名单用于拦截浏览器跨域；服务间调用由 `Authorization` 头鉴权，二者独立生效。
- 若只为 Studio 服务，可把 `trustedOrigins` 收紧到 Studio 自己的域名（即 `https://studio.0st.top`）。

---

## 3. 接口契约

### `POST /tryon`

**请求头**

```
Content-Type: application/json
Authorization: Bearer <TRYON_API_TOKEN>   # authRequired=true 时必填
```

**请求体**

```json
{
  "server_side": true,
  "person_image": "<人像 base64 或 data URL>",
  "garment_image": "<服装 base64 或 data URL>",
  "model": "gemini",
  "size": "1024x1536",
  "lang": "zh"
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| `server_side` | 服务间调用时必填 | `true` 表示由服务端预设模型生成；不允许同时传 `client_config` |
| `person_image` | 是 | 人像，base64 或 `data:image/png;base64,...`；JPEG/PNG/WebP，见下方限制 |
| `garment_image` | 是 | 服装图，同上 |
| `model` | 否 | 模型别名，默认 `gemini`：`gemini` / `gpt-image` / `doubao` / `comfyui` |
| `size` | 否 | `1024x1536`（默认）/ `1024x1024` / `1536x1024` |
| `lang` | 否 | `zh` / `en`，默认 `zh` |
| `client_config` | 否 | **仅浏览器场景可用**；`server_side: true` 时传了会被拒绝 |

**成功响应 `200`**

```json
{
  "image": "<结果图 base64>",
  "model": "gemini",
  "prompt": "<实际使用的提示词>",
  "server_side": true
}
```

> 响应为 base64 原始数据（不含 `data:` 前缀）。若该模型返回的是 URL，后端会先下载再转 base64。

**错误响应**（GoFrame `MiddlewareHandlerResponse` 标准结构）

| HTTP | `code` | `data.error_code` | 含义 |
|---|---|---|---|
| 400 | 40001 | `CLIENT_CONFIG_NOT_ALLOWED` | 服务间调用却携带了 `client_config` |
| 401 | 40101 | `AUTH_REQUIRED` | 未提供后端 Token |
| 401 | 40102 | `AUTH_INVALID` | Token 无效或长度不合规 |
| 403 | 40301 | `ORIGIN_NOT_ALLOWED` | 浏览器来源不在白名单 |
| 413 | 41301 | `REQUEST_TOO_LARGE` | 请求体超限（默认 24 MiB） |
| 422 | 42201 | `INVALID_IMAGE_BASE64` | 图片 base64 无法解码 |
| 422 | 42202 | `UNSUPPORTED_IMAGE_TYPE` | 非 JPEG/PNG/WebP，或声明的 MIME 与真实内容不一致 |
| 422 | 42203 | `IMAGE_FILE_TOO_LARGE` | 单图 > 8 MiB 或两图合计 > 16 MiB |
| 422 | 42204 | `IMAGE_DIMENSIONS_INVALID` | 长宽不在 256~8192 px |
| 422 | 42205 | `IMAGE_PIXEL_LIMIT_EXCEEDED` | 单图像素 > 2000 万 |
| 429 | 42901 | `RATE_LIMITED` | 触发速率限制，参考 `Retry-After` 头 |
| 429 | 42902 | `DAILY_QUOTA_EXCEEDED` | 当日额度用完 |
| 429 | 42903 | `CONCURRENCY_LIMITED` | 全局或单主体并发已满 |
| 503 | 50301 | `SERVICE_CALL_NOT_CONFIGURED` | 服务端未配置任何模型 Token |
| 503 | 50302 | `MODEL_NOT_CONFIGURED` | 服务端已有 Token，但 `model` 指定的别名缺密钥/地址（如未部署 ComfyUI） |

### `GET /comfyui/health`

检查本机 ComfyUI 与固定工作流是否可用，**不执行推理、不上传图片**。

```json
{ "available": true, "model": "qwen-image-edit-2511", "message": "本地 ComfyUI 固定工作流可用" }
```

Studio privater 部署时建议作为 readiness 探针之一，失败时不要路由到 `model: "comfyui"`。

---

## 4. Studio 侧接入参考（示意，需 Studio 授权后落地）

Studio 已有 `apps/server/src/http/local-ecom-image.ts` 的 `POST /api/ecom/jobs` 管线，
其 `generateConfiguredImage(db, model, params)` 接受 `prompt` + `inputImages`。
可在该管线上新增一个 `tryon` 渠道适配器，把两张参考图转发到本服务：

```ts
// 示意代码：不属于本仓库，不可直接提交到 Studio
async function generateTryonViaNvzhuang(params: {
  personImage: string;   // data:image/...;base64,... 或公网 URL
  garmentImage: string;
  size?: "1024x1536" | "1024x1024" | "1536x1024";
}) {
  const base = process.env.TRYON_BASE_URL;      // 仅服务端
  const token = process.env.TRYON_API_TOKEN;    // 仅服务端
  controlledFetchGuard(base);                    // 见 §5：SSRF 校验

  const res = await fetch(`${base}/tryon`, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      authorization: `Bearer ${token}`,
      "x-request-id": randomUUID(),
    },
    body: JSON.stringify({
      server_side: true,
      person_image: await toBase64(params.personImage),
      garment_image: await toBase64(params.garmentImage),
      size: params.size ?? "1024x1536",
    }),
    signal: AbortSignal.timeout(120_000),
  });

  const body = await res.json();
  if (!res.ok) {
  throw new TryonUpstreamError(body?.data?.error_code ?? "UNKNOWN", body?.message);
  }
  return Buffer.from(body.image, "base64");
}
```

对应关系：

| Studio `ecom` 概念 | 本服务 `/tryon` |
|---|---|
| 参考图 1 = 模特/人像 | `person_image` |
| 参考图 2 = 服装商品图 | `garment_image` |
| 场景 `16-try-on-virtual` | 整条链路（该场景即"把服装穿到模特身上"） |
| 生成结果写入 `asset_objects` | 把响应 `image` 解码后落盘，再建资产记录 |
| 积分扣费 / 部分退款 | 由 Studio 统一记账；本服务另计限流与每日额度 |

> Studio 现有 `16-try-on-virtual` 走的是「文生图 + 参考图」提示词路径（`buildPrompt`），
> 结果是**重绘**而非**保人换装**；走本接口则是真实的两图条件生成，人脸与服装细节保真度更高，
> 两者可作为 Studio 侧的两个可选"引擎"，由管理员在渠道配置里切换。

---

## 5. 接入方自查清单

Studio 侧实施时（**须先获得商业授权**）至少覆盖：

- [ ] `TRYON_BASE_URL` / `TRYON_API_TOKEN` 只存服务端环境变量，**不得**出现在 `NEXT_PUBLIC_*`、前端 bundle 或日志中。
- [ ] Base URL 走白名单/配置，不做用户输入直连；出站请求做 DNS 解析后私有地址拦截（防 SSRF），与 `local-ecom-image.ts` 现有 `resolveEcomReferenceImage` 同一套思路。
- [ ] 参考图取回后限制大小（建议 ≤ 8 MiB）与真实 MIME，再转 base64；不把第三方 URL 原样透传。
- [ ] 超时（建议 120 s）与失败重试上限（建议 1 次且仅对 5xx）显式设置。
- [ ] 上游错误码映射到 Studio 自己的 `error.code`，**不要**把本服务的 `message` 原样回显给终端用户。
- [ ] 计费：只有拿到有效图片才扣积分；失败或部分成功按 Studio 现有 `calculateCompletedCredits` 逻辑退款。
- [ ] 上游返回 base64 时，先校验真实图片类型与像素上限再落盘（与本服务 `imagevalidate` 同标准）。

---

## 6. 本地联调

```bash
cd backend
cp manifest/config/config.example.yaml manifest/config/config.yaml   # 若不存在
export TRYON_API_KEYS="$(python -c 'import secrets;print(secrets.token_urlsafe(48))')"
go run main.go            # 监听 :8000
```

```bash
curl -s -X POST http://127.0.0.1:8000/tryon \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TRYON_API_KEYS" \
  -d '{"server_side":true,"person_image":"'"$(base64 -w0 person.png)"'","garment_image":"'"$(base64 -w0 garment.png)"'"}'
```

验证点：

1. 不带 `Authorization` → `401 AUTH_REQUIRED`；
2. 带 `server_side: true` + `client_config` → `400 CLIENT_CONFIG_NOT_ALLOWED`，且**上游请求中不出现**该 Key；
3. 未设置 `TRYON_API_KEYS` → 进程启动即退出（fail-closed），不会以无鉴权状态对外提供服务；
4. 正常请求 → `200`，响应含 `"server_side": true` 与 base64 图片。

> 已实测（本仓库回归）：1、2 通过；未配置任何模型密钥时，`server_side` 请求返回 `503 50301`，
> 配置了 Token 但目标模型缺密钥时返回 `503 50302`（二者分别由
> `TestControllerTryonServerSideRejectsClientConfig`、`TestControllerTryonServerSideModelNotConfigured` 覆盖），
> 均不会发出外部模型调用。4 需要真实密钥，未在本仓库验证。

---

## 7. 许可证

- 本仓库：**MIT**，可自由商用、修改与分发。
- Studio：**Helstera Proprietary License**，商用与二次开发须取得权利人书面许可。
- 本指南仅描述协议对接，**不包含也不授予** Studio 的任何源码或再分发权利。
