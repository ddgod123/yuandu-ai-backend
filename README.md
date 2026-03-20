# emoji backend

一个以 **表情包内容库 + 视频转表情包生产流水线 + AI 运营后台** 为核心的 Go 后端项目。

> 当前 README 基于 **2026-03-19 的工作区代码** 更新，已纳入未提交改动的分析结果。

## 1. 项目定位

这个仓库不是单纯的“表情包后台”，而是一个综合系统：

- 用户与鉴权：短信验证码、密码登录、JWT、验证码、风控
- 内容库：合集、表情、分类、标签、主题、IP
- 对象存储：七牛上传、签名 URL、下载票据、对象管理
- 视频任务：视频转 GIF / WebP / JPG / PNG / live
- AI 能力：GIF Director / Planner / Judge、Meme 文案匹配与合成
- 运营后台：质量看板、反馈完整性、基线分析、导出、算力账户、风险后台

一句话：

> **Gin API + Asynq Worker + Postgres + Redis + 七牛 + ffmpeg/ffprobe + LLM 的单体平台。**

---

## 2. 关键目录

```text
cmd/                  # 程序入口：api、worker、各种回填/回归工具
internal/config/      # 配置加载
internal/db/          # 数据库连接
internal/router/      # Gin 路由
internal/middleware/  # JWT 等中间件
internal/models/      # GORM 模型
internal/handlers/    # HTTP 业务层
internal/videojobs/   # 视频任务核心引擎
internal/service/     # AIService / ComposeService
internal/storage/     # 七牛封装
pkg/oss/              # OSS 封装
migrations/           # SQL migration
scripts/              # 校验、回填、回归脚本
分析文档/             # 本次补充的后端分析文档
```

---

## 3. 本地启动

## 3.1 环境准备

你至少需要：

- Go 1.25.x
- PostgreSQL
- Redis
- 七牛配置
- `ffmpeg`
- `ffprobe`

可选：

- 阿里 OSS（meme 模板/成图）

先准备：

```bash
cp .env.example .env
```

再按你的本地环境填写：

- `DB_*`
- `REDIS_*`
- `ASYNQ_*`
- `QINIU_*`
- `JWT_*`
- `AI_*`
- `OSS_*`（可选）

> ⚠️ 注意：`.env.example` 当前仍建议你自行做一次安全清理，详见分析文档中的安全报告。

---

## 3.2 启动 API

```bash
go run ./cmd/api
```

默认：

- 端口：`5050`
- 健康检查：`GET /healthz`
- Swagger：`/swagger/index.html`

---

## 3.3 启动 Worker

```bash
go run ./cmd/worker
```

Worker 负责消费视频任务队列；如果只启动 API，不启动 Worker，视频任务会停留在排队状态。

---

## 3.4 运行测试

```bash
go test ./...
```

---

## 4. 主要入口

### API 入口

- `cmd/api/main.go`

负责：

- 加载配置
- 建立数据库连接
- 初始化七牛 / OSS / AIService / ComposeService
- 注册路由并启动 Gin

### Worker 入口

- `cmd/worker/main.go`

负责：

- 初始化 Asynq worker
- 注册 `video_jobs:process`
- 执行视频处理流水线

---

## 5. 最重要的业务主线

## 5.1 内容主线

- `collections.go`
- `emojis.go`
- `categories.go`
- `downloads.go`

## 5.2 用户与安全主线

- `auth.go`
- `auth_security.go`
- `security_risk.go`

## 5.3 视频任务主线

- `handlers/video_jobs.go`
- `videojobs/processor.go`
- `videojobs/ai_gif_pipeline.go`
- `videojobs/public_sync.go`
- `videojobs/points.go`
- `videojobs/costing.go`

---

## 6. 当前代码规模快照

- Go 文件：146
- 测试文件：34
- Migration：68
- 路由：176
- 关键大文件：
  - `internal/videojobs/processor.go`：9816 行
  - `internal/handlers/admin_video_jobs.go`：9149 行

---

## 7. 分析文档

本次已补充完整分析文档，位于：

- [`分析文档/00-索引.md`](./分析文档/00-索引.md)
- [`分析文档/01-后端架构图.md`](./分析文档/01-后端架构图.md)
- [`分析文档/02-数据库表关系图.md`](./分析文档/02-数据库表关系图.md)
- [`分析文档/03-API分组清单.md`](./分析文档/03-API分组清单.md)
- [`分析文档/04-Processor重构方案.md`](./分析文档/04-Processor重构方案.md)
- [`分析文档/05-安全审查报告.md`](./分析文档/05-安全审查报告.md)
- [`分析文档/06-新同事上手文档.md`](./分析文档/06-新同事上手文档.md)

建议阅读顺序：

1. 架构图
2. 数据库关系
3. API 清单
4. Processor 重构方案
5. 安全审查
6. 新同事上手文档

---

## 8. 当前最需要关注的两件事

### 1）安全整改

根据本次审查，当前存在若干高风险问题，尤其是：

- 公共组中存在不该公开的写接口
- 公共组中存在对象存储管理接口
- 示例环境文件中不应保留真实密钥

建议优先阅读：

- [`分析文档/05-安全审查报告.md`](./分析文档/05-安全审查报告.md)

### 2）视频引擎重构

`internal/videojobs/processor.go` 已经成为主要维护瓶颈。

建议优先阅读：

- [`分析文档/04-Processor重构方案.md`](./分析文档/04-Processor重构方案.md)

---

## 9. 适合新同事的阅读顺序

1. `cmd/api/main.go`
2. `internal/config/config.go`
3. `internal/router/router.go`
4. `internal/models/*.go`
5. `internal/handlers/auth.go`
6. `internal/handlers/collections.go`
7. `internal/handlers/video_jobs.go`
8. `internal/videojobs/processor.go`
9. `internal/handlers/admin_video_jobs.go`
10. `分析文档/06-新同事上手文档.md`

---

## 10. 说明

本 README 与分析文档的目标不是替代代码，而是降低首次接手和后续维护成本。  
如果你准备继续治理这个仓库，推荐先做：

1. 安全收口
2. Processor 模块化
3. Admin video jobs 模块化
4. README / 运维文档继续补全
