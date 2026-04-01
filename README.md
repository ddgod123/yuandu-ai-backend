# Emoji Backend (Go)

表情包平台后端服务，提供：

- 用户登录与权限体系
- 合集/表情内容管理
- 视频转图片任务（GIF/PNG/JPG/WebP/MP4/Live）
- 下载票据、收藏、互动统计
- 管理后台 API

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

## 6. 开源安全与资产策略（重点）

### 6.1 敏感信息

- 严禁提交真实密钥（AK/SK、Token、密码、私钥）
- `.env` 已被忽略，提交前仅保留 `.env.example`

### 6.2 模型参数/权重/长期训练资产

你说的很对：长期积累的**参数、权重、Prompt 策略、训练/标注数据**通常才是核心资产。  
这些内容**不应放入公开仓库**。

建议做法：

1. 放到私有仓库或私有对象存储（建议独立权限）
2. 在本仓只保留接口与示例模板（脱敏版）
3. 使用环境变量或远端配置中心注入运行参数
4. 将下列路径/文件类型长期加入 ignore（已处理）：
   - `weights/`、`checkpoints/`、`models/private/`、`datasets/private/`、`prompts/private/`
   - `*.pt`、`*.pth`、`*.ckpt`、`*.safetensors`、`*.onnx`、`*.gguf`、`*.pkl`

---

## 7. License

本项目采用仓库内 `LICENSE`。

