# PNG 主线上线前检查清单（P0）

> 目标：确保「视频转图片（PNG）」主链路在目标环境可稳定上线。

## 1) 数据库迁移就绪（必须）

在目标环境执行：

```bash
./scripts/check_png_mainline_schema.sh
```

等价命令：

```bash
go run ./cmd/check-schema-readiness --profile png-mainline --strict
```

若失败，请确认已执行以下 SQL：

- `migrations/085_video_image_split_tables_and_work_cards.sql`
- `migrations/086_compute_redeem_codes.sql`
- `migrations/087_compute_redeem_expire_clear.sql`

---

## 2) 计费闭环校验（必须）

```bash
go run ./cmd/check-billing-closure --format png --limit 100 --strict
```

如果是预发环境且需要修复历史测试数据，可先执行：

```bash
go run ./cmd/check-billing-closure --format png --limit 100 --repair --show-all
```

然后再次执行 `--strict`，要求 `mismatched=0`。

---

## 3) 后端回归（必须）

```bash
go test ./... -count=1
```

---

## 4) 前端构建（必须）

### 官网前端（frontweb）

```bash
npm run lint
npm run build
```

### 管理后台（lookfront）

```bash
npm run lint
npm run build
```

---

## 5) 发布冻结（必须）

上线前确认 3 个仓库都处于可追溯发布状态：

- backend
- frontweb
- lookfront

要求：

- `git status` 干净（无临时改动）
- 有明确发布 commit / tag
- 回滚版本已标记

