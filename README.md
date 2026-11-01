# 拍片哥女装 · AI 服装参考图集（COSTUME-DB-01）

一个免费、开源、可商用的 AI 服装参考图集网站，收录 **1407 张** 人物不入镜的纯服装图像（2:3 比例），支持中英文双语浏览与多维检索，并附带可选的 AI 一键换装能力。

- **纯前端**：浏览、分类、搜索、大图查看、黑白主题、本机生成记录全部离线可用，不需要后端。
- **免安装**：直接打开 `index.html`，或静态托管在任意 CDN / GitHub Pages。
- **中英双语**：中文模式自动把服装属性翻译为中文展示。
- **AI 换装可选**：需要时再拉起 `backend/`（GoFrame v2），支持多模型可插拔与本地 ComfyUI 离线生图。

## 目录

- [快速开始](#快速开始)
- [URL 参数](#url-参数)
- [接入 AI 换装后端](#接入-ai-换装后端)
- [模型配置](#模型配置)
- [前端对接后端](#前端对接后端)
- [接口安全与资源限制](#接口安全与资源限制)
- [项目结构](#项目结构)
- [数据字段](#数据字段)
- [常见问题](#常见问题)
- [社区友链](#社区友链)
- [许可证](#许可证)

## 快速开始

前端是纯静态站点，无需构建。

```bash
# 方式一：Python
python3 -m http.server 3000

# 方式二：Node
npx serve -l 3000 .

# 方式三：直接用浏览器打开 index.html
```

打开 `http://localhost:3000` 即可浏览图库。若同时已启动后端（默认 `:8000`），本地访问时 AI 换装会自动连上。

## URL 参数

支持通过 URL 参数直接分享特定状态：

| 参数 | 说明 | 示例 |
|---|---|---|
| `lang` | 初始语言，`zh`（默认）/ `en` | `?lang=en` |
| `q` | 初始搜索词 | `?q=lace` |
| `tryon` | 指定换装后端地址（**仅允许同源或本机回环**） | `?tryon=http://localhost:8000` |

可组合使用：`?lang=en&q=white`。

> `?tryon=` 只接受同源，或本机回环主机名 `localhost` / `127.0.0.1` / `[::1]`（IPv6 回环须**带方括号**，裸写 `::1` 会被判定为非法 URL 并静默忽略），用于防止钓鱼链接窃取 API Key 与用户照片。指向外部公网后端时，须在设置中心手动保存（且必须为 HTTPS）。

## 接入 AI 换装后端

「AI 换装」需要一个把服装真实「穿」到人像上的后端，位于 `backend/`。不部署后端时，图库的全部功能照常可用。

后端支持**多模型可插拔**，同一套接口可切换云端或本地生成：

| 模型别名 | 模型 | 协议 | 说明 |
|---|---|---|---|
| `comfyui` | **Qwen-Image-Edit-2511**（优先）/ MiniMax H3 | ComfyUI 本地 API | 本地零成本离线方案，4 步极速换装，无审查限制；Qwen 未就绪时自动回退 H3 |
| `gemini` | **Nano Banana 2**（`gemini-3.1-flash-image`） | Google generateContent | 云端推荐默认，对真人换装场景无 moderation 阻断 |
| `doubao` | 豆包 Seedream（火山方舟） | 火山方舟 | 国内可用，无需代理网络 |
| `gpt-image` | **GPT Image 2**（`gpt-image-2`） | OpenAI Images | 兼容 OpenAI 协议，支持第三方中转与代理 |
| `qwen` / `zhipu` / `siliconflow` … | 通义万相 / 智谱 CogView / 硅基流动 | OpenAI Images 兼容 | 按需在配置中开启 |

> 三个关键点都可配置：**API Key、Base URL、模型名**。新增一个国内模型只需在配置里加一段，无需改代码。

### 部署方式

后端推荐 **Docker**，无需在宿主机安装 Go 编译环境。

#### 方式一：Docker Compose（推荐）

```bash
# 1. 复制环境变量模板，填入你的 Key（如国内免翻墙的豆包 ARK_API_KEY）
cp backend/.env.example .env

# 2. 一键启动（默认监听 :8000）
docker compose up -d

# 服务启动后监听 http://localhost:8000
```

根目录的 `docker-compose.yml` 会自动读取 `.env` 或 `backend/.env`，未复制配置文件时后端也会自动回退加载模板，稳定监听 `:8000`。

#### 方式二：Docker Run 单命令

```bash
cd backend

# 构建镜像
docker build -f manifest/docker/Dockerfile -t costume-tryon .

# 运行（按需注入模型 Key，例如豆包或 OpenAI）
docker run -d --name costume-tryon \
  -p 8000:8000 \
  -e ARK_API_KEY="你的豆包ArkKey" \
  -e OPENAI_API_KEY="sk-你的OpenAIKey" \
  -e TRYON_API_KEYS="你的后端访问Token，至少32个字符" \
  costume-tryon
```

#### 方式三：本地直接运行

- **Windows 用户**：双击 `backend/restart.bat` 一键编译并启动。
- **命令行运行**：

```bash
cd backend
# 可选：复制配置文件，不复制时会自动安全回退加载模板并监听 :8000
# cp manifest/config/config.example.yaml manifest/config/config.yaml
export ARK_API_KEY="你的Key"
export TRYON_API_KEYS="你的后端访问Token，至少32个字符"
go run main.go
```

两个后端专用入口（`go run main.go` / `docker compose up`）的完整部署与安全加固说明见 [`backend/DEPLOY.md`](backend/DEPLOY.md)。

### ComfyUI 本地离线模式（零成本、免 Key、最快）

电脑有独立显卡（或局域网里有运行 ComfyUI 的主机）时，可以完全不用 API Key：

1. 启动本地 ComfyUI（默认 `http://127.0.0.1:8188`）；
2. 放好 `Qwen-Image-Edit-2511` 的模型与 4 步 Lightning LoRA，详见 [`backend/README.MD`](backend/README.MD)；
3. 打开网页右上角 **设置中心** → 选择「ComfyUI 本地」→ 点「测试连接」，成功后保存。

系统内置双工作流自适应：`qwen-image-edit-2511` 优先，未就绪时自动回退 `h3-ref2va-still`；也可用 `comfyui-qwen` / `comfyui-h3` 显式指定。对应工作流文件位于 `backend/workflows/`。

## 模型配置

编辑 `backend/manifest/config/config.yaml`（模板见同目录 [`config.example.yaml`](backend/manifest/config/config.example.yaml)）：

```yaml
llm:
  models:
    gpt-image:                    # 前端请求用的别名
      provider: openai             # 协议：openai / doubao / gemini / comfyui
      apiKey: ""                   # 直接填 Key，或留空用环境变量
      envKey: "OPENAI_API_KEY"     # 环境变量名（优先级高于 apiKey）
      baseURL: "https://api.openai.com/v1"
      model: "gpt-image-2"         # 真实模型名

    doubao:                       # 豆包（国内可用）
      provider: doubao
      envKey: "ARK_API_KEY"
      baseURL: "https://ark.cn-beijing.volces.com/api/v3"
      model: "doubao-seedream-3-0-t2i-250415"

    gemini:
      provider: gemini
      envKey: "GEMINI_API_KEY"
      baseURL: "https://generativelanguage.googleapis.com/v1beta"
      model: "gemini-3.1-flash-image"

    # 接入任意 OpenAI 兼容生图模型（通义万相 / 智谱 / 硅基流动等）
    qwen:
      provider: openai
      envKey: "DASHSCOPE_API_KEY"
      baseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"
      model: "wanx2.1-t2i-turbo"

    # 本地 ComfyUI（零成本离线换装）
    comfyui:
      provider: comfyui
      baseURL: "http://127.0.0.1:8188"
      model: "qwen-image-edit-2511"
      workflowPath: "workflows/comfyui_qwen_image_edit_2511_api.json"
```

### 两种使用方式

前端 AI 试穿页右上角「设置中心」支持：

1. **服务器预设**：使用后端 `config.yaml` / 环境变量中的模型配置。
2. **个人模型配置**：在当前浏览器保存协议、Base URL、API Key 与模型名，生成时随本次请求临时发送给后端。后端不把这些字段写入数据库、配置文件或业务日志。

> 浏览器本地保存适合个人设备。共享设备使用完毕后，请在设置中心点「清除本地配置」。个人 Base URL 默认禁止指向 localhost、内网和保留地址，避免公开后端被用作内网请求代理。

## 前端对接后端

前端按以下顺序识别换装后端地址（优先级从高到低）：

1. **设置中心**：用户在当前浏览器保存的「后端 API 地址」。跨源外部公网后端须使用 `HTTPS`。
2. **全局变量**：页面加载前设置 `window.AI_API_URL = "http://localhost:8000"`。
3. **URL 参数**：`?tryon=http://localhost:8000`。出于防钓鱼考虑，**仅允许同源或本机回环**；外部后端须在设置中心显式保存。
4. **本地回退**：以 `localhost` / `127.0.0.1` 访问前端时，默认自动连接 `http://127.0.0.1:8000`。

## 接口安全与资源限制

后端原生提供调用鉴权与资源限制（公网部署时建议全部保持开启）：

1. **接口访问鉴权**：Bearer Token 校验。服务端只保存 SHA-256 摘要，并做恒定时间比较以防侧信道；Token 由环境变量 `TRYON_API_KEYS` 提供，逗号分隔，每个 32~512 字符。前端设置中心可单独保存「后端访问 Token」，与大模型 API Key 互不相干。
2. **精确 CORS 白名单**：严格域名匹配，杜绝跨域滥刷与盗链。
3. **频率与每日额度**：令牌桶限流（默认 **10 次/分**，突发 3）+ UTC 自然日额度（默认 **100 次/天**），超限返回标准 `429`。
4. **并发控制**：全局默认 **2 并发**，单调用主体默认 **1 并发**，模型生图不排队堆积。
5. **图片校验**：
   - 仅接受 JPEG / PNG / WebP 真实内容探测，拒绝伪造扩展名与代码注入；
   - 单图上限 8 MiB、两图合计 16 MiB，长宽 256~8192 px，总像素上限 2000 万（防像素炸弹 OOM）。
6. **前端自适应压缩**：任一边小于 256 px 时自动等比放大；高清模式最长边 2048 px；阶梯压缩（WebP 优先，质量 0.90~0.75），避免触发体积上限。

> 完整的 Nginx 反代与生产加固说明见 [`backend/DEPLOY.md`](backend/DEPLOY.md)。

## 项目结构

```
nvzhuang/
├── index.html          # 页面结构 + 样式 + 交互（含双语字典、AI 试穿搭配台）
├── data.js             # 1407 条服装数据（id / 分类 / 关键词 / 图片路径）
├── docker-compose.yml  # 一键启动后端容器
├── assets/
│   ├── thumbs/         # 缩略图（webp，网格卡片用）
│   └── img/            # 展示图（webp，弹窗放大用）
├── docs/               # 截图、换装效果图与合规/集成文档
└── backend/            # AI 换装后端（GoFrame v2，可选）
    ├── docker-compose.yml             # 后端独立编排
    ├── .env.example                   # 环境变量模板
    ├── restart.bat / build.bat        # Windows 一键启动 / 编译
    ├── manifest/config/config.example.yaml
    ├── internal/logic/tryon/          # 模型适配器（openai / doubao / gemini / comfyui）
    └── DEPLOY.md                      # 部署与安全加固指南
```

> 注：原尺寸 PNG（约 1GB+）未纳入仓库，「下载原尺寸」按钮指向原站资源。

## 数据字段

`data.js` 每条记录包含以下字段：

| 字段 | 说明 |
|---|---|
| `id` | 编号，如 `SPR-0001` |
| `カテゴリ` | 分类，如「春」 |
| `キーワード` | 关键词，英文，空格分隔 |
| `サムネ` | 缩略图路径 |
| `画像` | 展示图路径 |
| `原寸` | 原尺寸 PNG 路径 |

> `カテゴリ`、`キーワード`、`サムネ`、`画像`、`原寸` 是数据原始的**日文字段名**，为与原站数据兼容而保留，未做重命名。`META.cats` 中的分类值同样为日文（`ランジェリー`、`パーティ`、`水着`、`セクシー` 等），网站会在前端把它们本地化为中文或英文展示。二次开发时读取这五个字段请严格按上面的日文字面量，不要按中文推测键名。

## 常见问题

**为什么 AI 换装报「所选模型尚未配置服务端密钥或地址」？**

返回 `HTTP 503` + `50302` / `MODEL_NOT_CONFIGURED`，表示请求的模型别名没有可用的 Key 或地址。在设置中心填入自己的模型配置，或在 `config.yaml` 中补全对应模型段。响应里带 `request_id`，可据此查日志。

**启动后端报没有后端访问 Token？**

`security.authRequired` 为 true 时，必须通过 `TRYON_API_KEYS` 环境变量提供至少一个 32~512 字符的 Token。

**本地访问前端，换装却连到了别处？**

检查优先级：设置中心保存的地址 > `window.AI_API_URL` > `?tryon=` 参数 > 本地回退 `127.0.0.1:8000`（后者仅当页面本身从 `localhost` / `127.0.0.1` / `file:` 打开时生效）。

注意 `window.AI_API_URL` 是在脚本加载时就读取的，必须写在 `index.html` 的脚本之前才有效，事后在控制台里赋值不会覆盖已确定的值。设置中心里点「清除本地配置」可恢复默认。

## 社区友链

- [LINUX DO](https://linux.do/) —— 真诚、友善、团结、专业的开源与技术极客社区。
- 相关文档：[开源合规审计报告](docs/LINUXDO_RELEASE_AUDIT.md)、[内容合规审查报告](docs/CONTENT_REVIEW.md)、[发布前核验清单](docs/LINUXDO_PUBLISH_CHECKLIST.md)、[Studio 集成说明](docs/INTEGRATION-STUDIO.md)

## 许可证

代码以 [MIT License](LICENSE) 开放。图集素材的来源与使用边界另见 [`docs/LICENSE-DECISION.md`](docs/LICENSE-DECISION.md)，两者适用范围不同，请分别阅读。
