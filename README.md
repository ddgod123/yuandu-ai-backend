# 元都AI Backend（Yuandu AI Backend）

元都AI（Yuandu AI）是一个**工业级 AI 视觉资产生产平台**。  
本仓库是平台后端与调度中枢，负责：

- 用户登录与权限体系
- 视觉资产（合集/单图/衍生结果）管理
- 视频转视觉资产任务（GIF/PNG/JPG/WebP/MP4/Live）
- 下载票据、收藏、互动统计
- 管理后台 API 与任务可观测能力

其中核心生产链路采用「**三脑一引擎**」思路：AI1 理解策划 → AI2 决策筛选 → Worker 执行生成 → AI3 复审交付。

---

## 1. 技术栈

- Go 1.25+
- Gin
- GORM
- PostgreSQL
- Redis + Asynq
- ffmpeg/ffprobe
- 七牛云对象存储

---

## 2. 目录结构

```text
cmd/                  # API、Worker、运维/回填工具入口
internal/             # 业务代码
migrations/           # SQL 迁移脚本（按编号顺序执行）
scripts/              # 运维与核查脚本
docs/                 # 文档
```

---

## 3. 快速开始（本地）

### 3.1 准备环境变量

```bash
cp .env.example .env
```

按你的本地环境填写 `.env`（数据库、Redis、对象存储等）。

### 3.2 启动 API

```bash
go run ./cmd/api
```

- 默认端口：`5050`
- 健康检查：`GET /healthz`

### 3.3 启动 Worker

```bash
go run ./cmd/worker
```

> 不启动 Worker 时，异步视频任务不会被消费。

### 3.4 测试

```bash
go test ./...
```

---

## 4. 数据库迁移

按文件名顺序执行 `migrations/*.sql`：

```bash
for f in migrations/*.sql; do
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$f"
done
```

---

## 5. 生产部署

详细见：[`docs/DEPLOYMENT.md`](./docs/DEPLOYMENT.md)

---

## 6. 开源安全与资产治理建议

### 6.1 敏感信息

- 严禁提交真实密钥（AK/SK、Token、密码、私钥）
- `.env` 已被忽略，提交前仅保留 `.env.example`

### 6.2 AI 资产管理

生产环境建议将以下资产独立治理（私有仓/对象存储/权限分层）：

- 模型参数与权重
- 私有 Prompt 策略
- 训练/标注数据
- 评测样本与基线结果

当前仓库已默认忽略典型私有资产路径与权重文件扩展名。

---

## 7. License

本项目采用仓库内 `LICENSE`。
