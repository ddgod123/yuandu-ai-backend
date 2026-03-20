# 字段级 SQL migration 草案

> 说明：本文件是**草案**，目标是把第一期三方工具接入所需的数据库变更一次性明确下来。  
> 范围：仅覆盖第一期最需要的 `quality settings` 扩展，以及第二期可选的 benchmark / tool run 审计表。

---

## 1. migration 命名建议

按当前仓库 migration 顺序，建议后续文件名为：

1. `070_video_quality_external_tools.sql`
2. `071_video_tool_benchmarks.sql`
3. `072_video_tool_runs_audit.sql`

其中：

- `070`：**建议第一期直接落地**
- `071` / `072`：**建议先保留草案，第一期可不执行**

---

## 2. 070：扩 `ops.video_quality_settings`

### 2.1 目标

给现有质量配置表补上：

- Gifsicle 开关与参数
- Scene detect 开关与参数
- VMAF benchmark 开关与参数

### 2.2 草案 SQL

```sql
BEGIN;

ALTER TABLE ops.video_quality_settings
  ADD COLUMN IF NOT EXISTS gif_gifsicle_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  ADD COLUMN IF NOT EXISTS gif_gifsicle_level SMALLINT NOT NULL DEFAULT 2,
  ADD COLUMN IF NOT EXISTS gif_gifsicle_skip_below_kb INTEGER NOT NULL DEFAULT 256,
  ADD COLUMN IF NOT EXISTS gif_gifsicle_min_gain_ratio DOUBLE PRECISION NOT NULL DEFAULT 0.03,
  ADD COLUMN IF NOT EXISTS scene_detect_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS scene_detect_provider VARCHAR(32) NOT NULL DEFAULT 'ffmpeg',
  ADD COLUMN IF NOT EXISTS scene_detect_detector VARCHAR(32) NOT NULL DEFAULT 'adaptive',
  ADD COLUMN IF NOT EXISTS scene_detect_threshold DOUBLE PRECISION NOT NULL DEFAULT 27,
  ADD COLUMN IF NOT EXISTS scene_detect_min_scene_len_sec DOUBLE PRECISION NOT NULL DEFAULT 0.8,
  ADD COLUMN IF NOT EXISTS vmaf_benchmark_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS vmaf_sample_rate INTEGER NOT NULL DEFAULT 10,
  ADD COLUMN IF NOT EXISTS vmaf_min_clip_sec DOUBLE PRECISION NOT NULL DEFAULT 1.2;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_video_quality_settings_gif_gifsicle_level'
  ) THEN
    ALTER TABLE ops.video_quality_settings
      ADD CONSTRAINT chk_video_quality_settings_gif_gifsicle_level
      CHECK (gif_gifsicle_level BETWEEN 1 AND 3);
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_video_quality_settings_gif_gifsicle_skip_below_kb'
  ) THEN
    ALTER TABLE ops.video_quality_settings
      ADD CONSTRAINT chk_video_quality_settings_gif_gifsicle_skip_below_kb
      CHECK (gif_gifsicle_skip_below_kb BETWEEN 0 AND 4096);
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_video_quality_settings_gif_gifsicle_min_gain_ratio'
  ) THEN
    ALTER TABLE ops.video_quality_settings
      ADD CONSTRAINT chk_video_quality_settings_gif_gifsicle_min_gain_ratio
      CHECK (gif_gifsicle_min_gain_ratio >= 0 AND gif_gifsicle_min_gain_ratio <= 0.50);
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_video_quality_settings_scene_detect_provider'
  ) THEN
    ALTER TABLE ops.video_quality_settings
      ADD CONSTRAINT chk_video_quality_settings_scene_detect_provider
      CHECK (scene_detect_provider IN ('ffmpeg', 'pyscenedetect'));
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_video_quality_settings_scene_detect_detector'
  ) THEN
    ALTER TABLE ops.video_quality_settings
      ADD CONSTRAINT chk_video_quality_settings_scene_detect_detector
      CHECK (scene_detect_detector IN ('content', 'adaptive', 'threshold'));
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_video_quality_settings_scene_detect_threshold'
  ) THEN
    ALTER TABLE ops.video_quality_settings
      ADD CONSTRAINT chk_video_quality_settings_scene_detect_threshold
      CHECK (scene_detect_threshold >= 1 AND scene_detect_threshold <= 100);
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_video_quality_settings_scene_detect_min_scene_len_sec'
  ) THEN
    ALTER TABLE ops.video_quality_settings
      ADD CONSTRAINT chk_video_quality_settings_scene_detect_min_scene_len_sec
      CHECK (scene_detect_min_scene_len_sec >= 0.2 AND scene_detect_min_scene_len_sec <= 10);
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_video_quality_settings_vmaf_sample_rate'
  ) THEN
    ALTER TABLE ops.video_quality_settings
      ADD CONSTRAINT chk_video_quality_settings_vmaf_sample_rate
      CHECK (vmaf_sample_rate BETWEEN 1 AND 30);
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_video_quality_settings_vmaf_min_clip_sec'
  ) THEN
    ALTER TABLE ops.video_quality_settings
      ADD CONSTRAINT chk_video_quality_settings_vmaf_min_clip_sec
      CHECK (vmaf_min_clip_sec >= 0.5 AND vmaf_min_clip_sec <= 10);
  END IF;
END$$;

COMMENT ON COLUMN ops.video_quality_settings.gif_gifsicle_enabled IS '是否启用 gifsicle 二次优化';
COMMENT ON COLUMN ops.video_quality_settings.gif_gifsicle_level IS 'gifsicle 优化档位，建议 1..3';
COMMENT ON COLUMN ops.video_quality_settings.gif_gifsicle_skip_below_kb IS '小于该大小的 GIF 跳过优化';
COMMENT ON COLUMN ops.video_quality_settings.gif_gifsicle_min_gain_ratio IS '最小收益比例，小于该比例可保留原文件';
COMMENT ON COLUMN ops.video_quality_settings.scene_detect_enabled IS '是否启用独立 scene detect';
COMMENT ON COLUMN ops.video_quality_settings.scene_detect_provider IS 'scene detect provider: ffmpeg | pyscenedetect';
COMMENT ON COLUMN ops.video_quality_settings.scene_detect_detector IS 'scene detect detector: content | adaptive | threshold';
COMMENT ON COLUMN ops.video_quality_settings.scene_detect_threshold IS 'scene detect 阈值';
COMMENT ON COLUMN ops.video_quality_settings.scene_detect_min_scene_len_sec IS '最小 scene 时长';
COMMENT ON COLUMN ops.video_quality_settings.vmaf_benchmark_enabled IS '是否启用 VMAF 离线 benchmark';
COMMENT ON COLUMN ops.video_quality_settings.vmaf_sample_rate IS 'VMAF 采样帧率';
COMMENT ON COLUMN ops.video_quality_settings.vmaf_min_clip_sec IS 'VMAF 参与 benchmark 的最小时长';

COMMIT;
```

### 2.3 为什么 070 建议第一期就上

因为你们第一期真正会改的主配置入口就是：

- `internal/videojobs/quality_settings.go`
- `internal/handlers/video_quality_settings.go`
- `internal/models/video_jobs.go` 里的 `VideoQualitySetting`

如果 DB 不先扩，后面的后台配置和 runtime 开关都接不稳。

---

## 3. 071：建 `ops.video_tool_benchmarks`

> 说明：这张表第一期**可选**。如果你们决定先把 benchmark 结果只落 JSON，这张表可以先不建。

### 3.1 目标

用于长期存储：

- VMAF benchmark 结果
- 后续可能接入的其他 benchmark 结果
- profile 对比、baseline 对比的长期沉淀

### 3.2 草案 SQL

```sql
BEGIN;

CREATE TABLE IF NOT EXISTS ops.video_tool_benchmarks (
  id BIGSERIAL PRIMARY KEY,
  job_id BIGINT NOT NULL,
  output_id BIGINT NULL,
  baseline_output_id BIGINT NULL,
  tool_name VARCHAR(32) NOT NULL,
  tool_version VARCHAR(64) NOT NULL DEFAULT '',
  run_reason VARCHAR(64) NOT NULL DEFAULT '',
  result_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_video_tool_benchmarks_tool_name
    CHECK (tool_name IN ('vmaf', 'gifsicle', 'pyscenedetect', 'real_esrgan'))
);

CREATE INDEX IF NOT EXISTS idx_video_tool_benchmarks_job_created
  ON ops.video_tool_benchmarks (job_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_video_tool_benchmarks_output_created
  ON ops.video_tool_benchmarks (output_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_video_tool_benchmarks_tool_created
  ON ops.video_tool_benchmarks (tool_name, created_at DESC);

COMMENT ON TABLE ops.video_tool_benchmarks IS '视频任务第三方工具 benchmark 结果';
COMMENT ON COLUMN ops.video_tool_benchmarks.result_json IS 'benchmark 原始结果';

COMMIT;
```

### 3.3 第一阶段是否建议执行

我的建议：

- **如果你们本周就要开始做 VMAF 回归沉淀**，执行
- **如果第一期只想先把 VMAF 跑通**，可以先不执行，先写到 JSON

---

## 4. 072：建 `audit.video_tool_runs`

> 说明：这张表也是第一期**可选**。更偏平台审计用途。

### 4.1 目标

记录工具执行日志，便于查：

- 调用了哪个工具
- 在哪个 stage 调的
- 成功/失败/降级情况
- 耗时与错误消息

### 4.2 草案 SQL

```sql
BEGIN;

CREATE TABLE IF NOT EXISTS audit.video_tool_runs (
  id BIGSERIAL PRIMARY KEY,
  job_id BIGINT NOT NULL,
  stage VARCHAR(32) NOT NULL,
  tool_name VARCHAR(32) NOT NULL,
  status VARCHAR(16) NOT NULL,
  duration_ms BIGINT NOT NULL DEFAULT 0,
  error_message TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_video_tool_runs_stage
    CHECK (stage IN ('analyzing', 'candidate_selection', 'rendering', 'evaluation', 'rerender', 'benchmark')),
  CONSTRAINT chk_video_tool_runs_tool_name
    CHECK (tool_name IN ('ffmpeg', 'ffprobe', 'gifsicle', 'pyscenedetect', 'vmaf', 'real_esrgan')),
  CONSTRAINT chk_video_tool_runs_status
    CHECK (status IN ('started', 'done', 'fallback', 'failed', 'skipped'))
);

CREATE INDEX IF NOT EXISTS idx_video_tool_runs_job_created
  ON audit.video_tool_runs (job_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_video_tool_runs_tool_created
  ON audit.video_tool_runs (tool_name, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_video_tool_runs_stage_created
  ON audit.video_tool_runs (stage, created_at DESC);

COMMENT ON TABLE audit.video_tool_runs IS '第三方工具执行审计';
COMMENT ON COLUMN audit.video_tool_runs.metadata IS '输入参数、输出摘要、fallback 信息';

COMMIT;
```

---

## 5. 推荐执行顺序

### 第一阶段

只执行：

- `070_video_quality_external_tools.sql`

### 第二阶段

根据需求执行：

- `071_video_tool_benchmarks.sql`
- `072_video_tool_runs_audit.sql`

---

## 6. 与现有表的关系建议

### `ops.video_quality_settings`

继续作为：

- 主配置真源
- 后台可修改的 rollout / tool 开关中心

### `archive.video_jobs.metrics`

继续作为：

- 单任务级汇总
- 适合写：`scene_detect_v1` / `vmaf_benchmark_summary_v1`

### `public.video_image_outputs.metadata`

继续作为：

- 单 output 工具执行详情
- 适合写：`gif_optimization_v1` / `vmaf_benchmark_v1`

### `archive.video_job_gif_evaluations.feature_json`

继续作为：

- 评价时的派生特征
- 适合补：`gif_optimization_v1`

---

## 7. 我给你的落地建议

如果你准备开始真正改代码，我建议这样执行：

1. 先把 `070` 落地
2. 第二阶段先不建 `071/072`
3. 先用 JSON 落结果，把链路跑顺
4. 等你们确认 benchmark / audit 查询真的常用，再把 `071/072` 建出来

这样最稳。
