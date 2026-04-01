# Yuandu AI Backend

[中文](#中文简介) | [English](#english)

---

## 中文简介

元都AI（Yuandu AI）是一个**工业级 AI 视觉资产生产平台**。  
本仓库是平台后端与调度中枢，负责 API、任务编排、Worker 调度、资产交付与可观测能力。

### 核心能力

- 用户与权限体系（鉴权、下载票据、风控）
- 视觉资产管理（合集/单图/衍生结果）
- 视频转视觉资产任务（GIF/PNG/JPG/WebP/MP4/Live）
- 异步任务编排（Redis + Asynq）
- 管理中台 API 与质量巡检接口

### 生产架构（简化）

AI1（需求理解） → AI2（候选评分/筛选） → Worker（执行生成） → AI3（复审交付）

---

## English

Yuandu AI is an **industrial AI visual asset production platform**.  
This repository contains the backend control plane: API, workflow orchestration, worker scheduling, asset delivery, and observability.

### Core Capabilities

- Auth, permission, download ticketing, and risk controls
- Visual asset management (collections / single assets / derivatives)
- Video-to-asset production (GIF/PNG/JPG/WebP/MP4/Live)
- Async orchestration with Redis + Asynq
- Admin-facing APIs for operations and quality governance

---

## Tech Stack

- Go 1.25+
- Gin + GORM
- PostgreSQL
- Redis + Asynq
- ffmpeg / ffprobe
- Qiniu Object Storage

---

## Quick Start

```bash
cp .env.example .env
go run ./cmd/api
# new terminal
go run ./cmd/worker
```

- API default: `:5050`
- Health: `GET /healthz`

---

## Database Migration

```bash
for f in migrations/*.sql; do
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$f"
done
```

---

## Deployment

See: [`docs/DEPLOYMENT.md`](./docs/DEPLOYMENT.md)

---

## Open-source Safety

- Do **not** commit real secrets (`.env` is ignored)
- Do **not** commit private model weights / prompt policies / private datasets

---

## License

See `LICENSE`.
