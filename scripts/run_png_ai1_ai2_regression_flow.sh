#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "[PNG Regression] Step 1/4: AI1 策略与场景测试（default/xiaohongshu）"
go test "$ROOT_DIR/internal/videojobs" -run \
  "Test(NormalizeVideoJobAdvancedOptions_VisualFocusMaxTwo|ResolveVideoJobAI1StrategyProfile_DefaultPNG|ResolveVideoJobAI1StrategyProfile_Xiaohongshu|ApplyImageAI1StrategyHardOverrides|ClampPNGMainlineAdvancedOptions_FallbackToDefault|ClampPNGMainlineAdvancedOptions_AllowXiaohongshu)"

echo "[PNG Regression] Step 2/4: AI2 对齐与门槛测试（命中/拒绝/水印）"
go test "$ROOT_DIR/internal/videojobs" -run \
  "Test(PNGMainline_AI1EditedDirectiveFlowsToWorkerStrategy|ComputeFrameQualityScoreWithAI2Weights_DirectiveHitBoostAndAvoidPenalty|ComputeFrameQualityScoreWithAI2Weights_XiaohongshuSceneBoost|ShouldRejectForWatermarkRisk)"

echo "[PNG Regression] Step 3/4: Debug 可观测性测试（公式/拒绝分布/候选解释）"
go test "$ROOT_DIR/internal/handlers" -run \
  "Test(BuildAI2ExecutionObservability_CollectsGuidedSignals|BuildAI1DebugTimelineV1_IncludesAI2ExecutionStep|BuildAI1OutputContractReport_RequiresAI2QualityWeights|BuildPipelineAlignmentReportV1_ProducesConsistencyChecks)"

echo "[PNG Regression] Step 4/4: 主线套件 smoke（videojobs + handlers）"
go test "$ROOT_DIR/internal/videojobs" "$ROOT_DIR/internal/handlers" "$ROOT_DIR/internal/router" "$ROOT_DIR/cmd/backfill-video-image-split"

echo "✅ PNG AI1/AI2 主线回归通过"
