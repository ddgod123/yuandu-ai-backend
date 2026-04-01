# 元都AI Backend 部署说明（生产）

> 适用于 Linux + systemd + Nginx 反向代理。

## 1. 依赖

- Go 1.25+
- PostgreSQL 14+
- Redis 7+
- ffmpeg / ffprobe
- Nginx（可选，用于对外统一入口）

---

## 2. 构建

```bash
cd backend
go mod download
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/api ./cmd/api
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/worker ./cmd/worker
```

---

## 3. 环境变量

```bash
cp .env.example /etc/emoji/emoji.env
chmod 600 /etc/emoji/emoji.env
```

最少需要确认：

- `APP_ENV=prod`
- `DB_*`
- `REDIS_*` / `ASYNQ_*`
- `QINIU_*`
- `JWT_*`

---

## 4. 数据库迁移

```bash
cd backend
for f in migrations/*.sql; do
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$f"
done
```

---

## 5. systemd

### 5.1 API 服务

`/etc/systemd/system/emoji-api.service`

```ini
[Unit]
Description=Yuandu AI Backend API
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/emoji/backend
EnvironmentFile=/etc/emoji/emoji.env
ExecStart=/opt/emoji/backend/bin/api
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

### 5.2 Worker 服务

`/etc/systemd/system/emoji-worker.service`

```ini
[Unit]
Description=Yuandu AI Backend Worker
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/emoji/backend
EnvironmentFile=/etc/emoji/emoji.env
ExecStart=/opt/emoji/backend/bin/worker
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

启用：

```bash
systemctl daemon-reload
systemctl enable --now emoji-api emoji-worker
```

---

## 6. 健康检查

```bash
curl -fsS http://127.0.0.1:5050/healthz
systemctl status emoji-api emoji-worker --no-pager
```

---

## 7. Nginx（可选）

将 `/api/` 反代到 `127.0.0.1:5050`：

```nginx
location /api/ {
  proxy_pass http://127.0.0.1:5050;
  proxy_http_version 1.1;
  proxy_set_header Host $host;
  proxy_set_header X-Real-IP $remote_addr;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  proxy_set_header X-Forwarded-Proto $scheme;
}
```

---

## 8. 发布前检查（建议）

```bash
go test ./...
bash scripts/pre-open-source-check.sh
```

