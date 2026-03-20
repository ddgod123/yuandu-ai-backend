# 03-API分组清单

> 路由来源：`internal/router/router.go`  
> 统计基准：当前工作区 176 个路由

## 1. 路由分层规则

| 层级 | 前缀 | 认证要求 | 说明 |
|---|---|---|---|
| 公共路由 | `/healthz`、`/api/*` | `AuthOptional` | 可匿名访问，若带 token 则可读取 user/role |
| 登录用户路由 | `/api/*` | `Auth(cfg)` | 必须登录 |
| 管理员路由 | `/api/admin/*` | `Auth + RequireAnyRole(super_admin)` | 仅超管 |
| 文档路由 | `/swagger/*any` | 无 | Swagger 文档 |

> 注意：当前公共组中含有若干**写接口**，这在安全文档里已单独标为高风险项。

---

## 2. 公共接口

## 2.1 健康检查

| Method | Path | Handler | 说明 |
|---|---|---|---|
| GET | `/healthz` | `Health` | 服务健康检查 |

## 2.2 鉴权与验证码

| Method | Path | Handler | 说明 |
|---|---|---|---|
| POST | `/api/auth/register-phone` | `RegisterPhone` | 手机号注册 |
| POST | `/api/auth/login-phone` | `LoginPhone` | 手机验证码登录 |
| POST | `/api/auth/login` | `Login` | 密码登录 |
| POST | `/api/auth/refresh` | `Refresh` | 刷新 token |
| POST | `/api/auth/logout` | `Logout` | 退出登录 |
| GET | `/api/auth/captcha` | `GetCaptcha` | 获取图形验证码 |
| POST | `/api/auth/send-code` | `SendCode` | 发送短信验证码 |

## 2.3 站点统计与基础公共数据

| Method | Path | Handler | 说明 |
|---|---|---|---|
| GET | `/api/stats/today` | `GetTodayStats` | 今日统计 |
| GET | `/api/stats/home` | `GetHomeStats` | 首页统计 |
| GET | `/api/categories` | `ListPublicCategories` | 公共分类树 |
| GET | `/api/ips` | `ListIPs` | 公共 IP 列表 |
| GET | `/api/ips/:id` | `GetIP` | 公共 IP 详情 |
| GET | `/api/card-themes` | `ListCardThemes` | 卡片主题 |
| GET | `/api/site-settings/footer` | `GetSiteFooterSetting` | 页脚配置 |
| POST | `/api/join-applications` | `CreateJoinApplication` | 入驻/加入申请 |

## 2.4 内容查询与公共内容操作

| Method | Path | Handler | 说明 |
|---|---|---|---|
| GET | `/api/collections` | `ListCollections` | 合集列表 |
| GET | `/api/collections/:id` | `GetCollection` | 合集详情 |
| POST | `/api/collections` | `CreateCollection` | ⚠ 当前公开可写 |
| PUT | `/api/collections/:id` | `UpdateCollection` | ⚠ 当前公开可写 |
| DELETE | `/api/collections/:id` | `DeleteCollection` | ⚠ 当前公开可写 |
| GET | `/api/emojis` | `ListEmojis` | 表情列表 |
| POST | `/api/emojis` | `BatchUploadEmoji` | ⚠ 当前公开可写 |
| PUT | `/api/emojis/:id` | `UpdateEmoji` | ⚠ 当前公开可写 |
| DELETE | `/api/emojis/:id` | `DeleteEmoji` | ⚠ 当前公开可写 |

## 2.5 存储对象接口

| Method | Path | Handler | 说明 |
|---|---|---|---|
| POST | `/api/storage/upload-token` | `GetUploadToken` | ⚠ 获取七牛上传 token |
| GET | `/api/storage/object` | `GetObject` | ⚠ 查询对象信息 |
| GET | `/api/storage/proxy` | `ProxyObject` | 代理对象内容 |
| DELETE | `/api/storage/object` | `DeleteObject` | ⚠ 删除对象 |
| GET | `/api/storage/objects` | `ListObjects` | ⚠ 列对象 |
| POST | `/api/storage/rename` | `RenameObject` | ⚠ 重命名对象 |
| GET | `/api/storage/url` | `GetObjectURL` | ⚠ 获取对象 URL |
| POST | `/api/storage/urls` | `GetObjectURLs` | ⚠ 批量获取对象 URL |
| GET | `/api/download/ticket/:token` | `DownloadByTicket` | 通过票据下载 |

## 2.6 Meme 公共 Feed

| Method | Path | Handler | 说明 |
|---|---|---|---|
| GET | `/api/v1/memes/feed` | `FeedMemes` | Meme 公共 Feed |

---

## 3. 登录用户接口

## 3.1 合集下载与互动

| Method | Path | Handler | 说明 |
|---|---|---|---|
| GET | `/api/collections/:id/download-zip` | `GetCollectionZipDownload` | 下载合集 ZIP |
| GET | `/api/collections/:id/download-zip-all` | `GetCollectionZipDownloadAll` | 下载全部 ZIP |
| GET | `/api/collections/:id/zips` | `GetCollectionZipList` | ZIP 列表 |
| GET | `/api/collections/:id/download-list` | `GetCollectionDownloadList` | 下载列表 |
| POST | `/api/collections/:id/like` | `AddCollectionLike` | 点赞合集 |
| DELETE | `/api/collections/:id/like` | `RemoveCollectionLike` | 取消点赞合集 |
| POST | `/api/collections/:id/favorite` | `AddCollectionFavorite` | 收藏合集 |
| DELETE | `/api/collections/:id/favorite` | `RemoveCollectionFavorite` | 取消收藏合集 |
| GET | `/api/emojis/:id/download` | `GetEmojiDownload` | 获取表情下载链接 |
| GET | `/api/emojis/:id/download-file` | `DownloadEmojiFile` | 直接下载表情文件 |

## 3.2 个人中心

| Method | Path | Handler | 说明 |
|---|---|---|---|
| GET | `/api/me` | `Me` | 当前用户信息 |
| PUT | `/api/me` | `UpdateMe` | 更新个人资料 |
| GET | `/api/me/compute-account` | `GetMyComputeAccount` | 我的算力账户 |
| POST | `/api/me/redeem-code/validate` | `ValidateRedeemCodeForMe` | 校验兑换码 |
| POST | `/api/me/redeem-code` | `RedeemCodeForMe` | 提交兑换码 |
| GET | `/api/me/redeem-records` | `ListMyRedeemRecords` | 我的兑换记录 |

## 3.3 视频任务

| Method | Path | Handler | 说明 |
|---|---|---|---|
| POST | `/api/video-jobs` | `CreateVideoJob` | 创建视频转图任务 |
| GET | `/api/video-jobs` | `ListMyVideoJobs` | 我的任务列表 |
| GET | `/api/video-jobs/capabilities` | `GetVideoJobCapabilities` | 运行时能力检查 |
| POST | `/api/video-jobs/source-probe` | `ProbeVideoSource` | 预探测源视频 |
| GET | `/api/video-jobs/:id` | `GetVideoJob` | 任务详情 |
| GET | `/api/video-jobs/:id/result` | `GetVideoJobResult` | 任务结果 |
| GET | `/api/video-jobs/:id/download-zip` | `GetVideoJobZipDownload` | 下载任务结果 ZIP |
| POST | `/api/video-jobs/:id/feedback` | `SubmitVideoJobFeedback` | 提交结果反馈 |
| POST | `/api/video-jobs/:id/cancel` | `CancelVideoJob` | 取消任务 |
| POST | `/api/video-jobs/:id/delete-collection` | `DeleteVideoJobCollection` | 删除结果合集 |
| POST | `/api/video-jobs/:id/delete-output` | `DeleteVideoJobOutput` | 删除单个输出 |

## 3.4 收藏夹

| Method | Path | Handler | 说明 |
|---|---|---|---|
| POST | `/api/favorites` | `AddFavorite` | 收藏表情 |
| DELETE | `/api/favorites/:emoji_id` | `RemoveFavorite` | 取消收藏表情 |
| GET | `/api/favorites` | `ListFavorites` | 我的表情收藏 |
| GET | `/api/favorites/collections` | `ListCollectionFavorites` | 我的合集收藏 |

## 3.5 Meme 个人接口

| Method | Path | Handler | 说明 |
|---|---|---|---|
| POST | `/api/v1/memes/generate` | `GenerateMeme` | 生成 meme |
| POST | `/api/v1/memes/:id/like` | `ToggleMemeLike` | 点赞/取消点赞 meme |
| POST | `/api/v1/memes/:id/collect` | `ToggleMemeCollect` | 收藏/取消收藏 meme |
| GET | `/api/v1/users/me/memes` | `MyMemes` | 我的 meme |
| GET | `/api/v1/users/me/collections` | `MyMemeCollections` | 我收藏的 meme |

---

## 4. 管理员接口

## 4.1 用户与系统

| Method | Path | Handler | 说明 |
|---|---|---|---|
| GET | `/api/admin/users` | `ListUsers` | 用户列表 |
| PUT | `/api/admin/users/:id/role` | `UpdateUserRole` | 修改用户角色 |
| PUT | `/api/admin/users/:id/status` | `UpdateUserStatus` | 修改用户状态 |
| GET | `/api/admin/users/:id/detail` | `GetAdminUserDetail` | 用户详情 |
| GET | `/api/admin/system/worker-health` | `GetAdminWorkerHealth` | Worker 健康检查 |
| POST | `/api/admin/system/worker-start` | `StartAdminWorker` | 一键拉起 Worker |
| POST | `/api/admin/telegram/download` | `DownloadTelegram` | Telegram 下载接入 |

## 4.2 分类、IP、标签、主题、合集内容管理

| Method | Path | Handler | 说明 |
|---|---|---|---|
| GET | `/api/admin/categories` | `ListCategories` | 分类列表 |
| POST | `/api/admin/categories` | `CreateCategory` | 新建分类 |
| PUT | `/api/admin/categories/:id` | `UpdateCategory` | 修改分类 |
| DELETE | `/api/admin/categories/:id` | `DeleteCategory` | 删除分类 |
| GET | `/api/admin/categories/stats` | `ListCategoryStats` | 分类统计 |
| GET | `/api/admin/categories/:id/objects` | `ListCategoryObjects` | 分类下对象 |
| GET | `/api/admin/ips` | `ListAdminIPs` | IP 列表 |
| POST | `/api/admin/ips` | `CreateIP` | 新建 IP |
| PUT | `/api/admin/ips/:id` | `UpdateIP` | 修改 IP |
| DELETE | `/api/admin/ips/:id` | `DeleteIP` | 删除 IP |
| GET | `/api/admin/tags` | `ListTags` | 标签列表 |
| POST | `/api/admin/tags` | `CreateTag` | 新建标签 |
| PUT | `/api/admin/tags/:id` | `UpdateTag` | 修改标签 |
| DELETE | `/api/admin/tags/:id` | `DeleteTag` | 删除标签 |
| GET | `/api/admin/tag-groups` | `ListTagGroups` | 标签组列表 |
| POST | `/api/admin/tag-groups` | `CreateTagGroup` | 新建标签组 |
| PUT | `/api/admin/tag-groups/:id` | `UpdateTagGroup` | 修改标签组 |
| DELETE | `/api/admin/tag-groups/:id` | `DeleteTagGroup` | 删除标签组 |
| GET | `/api/admin/themes` | `ListThemes` | 主题列表 |
| POST | `/api/admin/themes` | `CreateTheme` | 新建主题 |
| PUT | `/api/admin/themes/:id` | `UpdateTheme` | 修改主题 |
| DELETE | `/api/admin/themes/:id` | `DeleteTheme` | 删除主题 |
| PUT | `/api/admin/collections/:id` | `AdminUpdateCollection` | 管理员修改合集 |
| DELETE | `/api/admin/collections/:id` | `AdminDeleteCollection` | 管理员硬删合集 |
| POST | `/api/admin/collections/batch-sample` | `AdminBatchUpdateCollectionSample` | 批量样本标记 |
| GET | `/api/admin/collections/samples/export.csv` | `AdminExportSampleCollectionsCSV` | 样本合集导出 |
| POST | `/api/admin/collections/import-zip` | `ImportCollectionZip` | ZIP 导入新合集 |
| POST | `/api/admin/collections/:id/import-zip` | `AppendCollectionZip` | ZIP 追加到合集 |
| POST | `/api/admin/collections/:id/emojis/upload` | `UploadCollectionEmojis` | 向合集上传表情 |

## 4.3 存储对象运维

| Method | Path | Handler | 说明 |
|---|---|---|---|
| DELETE | `/api/admin/storage/object` | `AdminDeleteObject` | 删除对象 |
| POST | `/api/admin/storage/batch-delete` | `BatchDeleteObjects` | 批量删除 |
| GET | `/api/admin/storage/trash` | `ListTrashObjects` | 垃圾箱列表 |
| DELETE | `/api/admin/storage/trash` | `EmptyTrash` | 清空垃圾箱 |
| POST | `/api/admin/storage/trash/restore` | `RestoreTrashObject` | 恢复对象 |
| POST | `/api/admin/storage/trash/batch-restore` | `BatchRestoreTrashObjects` | 批量恢复 |
| GET | `/api/admin/storage/search` | `AdminSearchObjects` | 搜索对象 |

## 4.4 运维、风控、上传任务

| Method | Path | Handler | 说明 |
|---|---|---|---|
| GET | `/api/admin/ops/metrics/summary` | `GetOpsMetricsSummary` | 运营汇总 |
| GET | `/api/admin/ops/metrics/top-categories` | `ListOpsTopCategories` | 热门分类 |
| GET | `/api/admin/ops/metrics/search-terms` | `ListOpsSearchTerms` | 搜索词统计 |
| GET | `/api/admin/security/overview` | `GetSecurityOverview` | 安全总览 |
| GET | `/api/admin/security/blacklists` | `ListRiskBlacklists` | 黑名单列表 |
| POST | `/api/admin/security/blacklists` | `CreateRiskBlacklist` | 新建黑名单 |
| PUT | `/api/admin/security/blacklists/:id/status` | `UpdateRiskBlacklistStatus` | 修改黑名单状态 |
| DELETE | `/api/admin/security/blacklists/:id` | `DeleteRiskBlacklist` | 删除黑名单 |
| GET | `/api/admin/security/events` | `ListRiskEvents` | 风险事件列表 |
| GET | `/api/admin/upload-tasks` | `ListUploadTasks` | 上传任务列表 |
| GET | `/api/admin/dashboard/trends` | `GetAdminDashboardTrends` | 后台趋势 |

## 4.5 站点设置与兑换码

| Method | Path | Handler | 说明 |
|---|---|---|---|
| GET | `/api/admin/site-settings/footer` | `GetAdminSiteFooterSetting` | 页脚配置详情 |
| PUT | `/api/admin/site-settings/footer` | `UpdateAdminSiteFooterSetting` | 更新页脚配置 |
| GET | `/api/admin/redeem-codes` | `ListRedeemCodes` | 兑换码列表 |
| POST | `/api/admin/redeem-codes/generate` | `GenerateRedeemCodes` | 批量生成兑换码 |
| PUT | `/api/admin/redeem-codes/:id/status` | `UpdateRedeemCodeStatus` | 更新兑换码状态 |
| GET | `/api/admin/redeem-codes/:id/redemptions` | `ListRedeemCodeRedemptions` | 查看兑换记录 |

## 4.6 视频任务后台运营

| Method | Path | Handler | 说明 |
|---|---|---|---|
| GET | `/api/admin/video-jobs/overview` | `GetAdminVideoJobsOverview` | 视频任务总览 |
| GET | `/api/admin/video-jobs` | `ListAdminVideoJobs` | 后台任务列表 |
| GET | `/api/admin/video-jobs/:id` | `GetAdminVideoJob` | 后台任务详情 |
| GET | `/api/admin/video-jobs/:id/health` | `GetAdminVideoJobHealth` | 单任务健康详情 |
| GET | `/api/admin/video-jobs/:id/download-zip` | `GetAdminVideoJobZipDownload` | 后台下载任务 ZIP |
| POST | `/api/admin/video-jobs/:id/delete-collection` | `AdminDeleteVideoJobCollection` | 后台删结果合集 |
| POST | `/api/admin/video-jobs/:id/delete-output` | `AdminDeleteVideoJobOutput` | 后台删结果输出 |
| POST | `/api/admin/video-jobs/:id/rerender-gif` | `AdminRerenderVideoJobGIF` | 按 proposal 重渲 GIF |

## 4.7 视频任务质量、反馈、导出

| Method | Path | Handler | 说明 |
|---|---|---|---|
| GET | `/api/admin/video-jobs/feedback-integrity/overview` | `GetAdminVideoJobsFeedbackIntegrityOverview` | 反馈完整性总览 |
| GET | `/api/admin/video-jobs/feedback-integrity/drilldown` | `GetAdminVideoJobsFeedbackIntegrityDrilldown` | 反馈完整性下钻 |
| GET | `/api/admin/video-jobs/feedback-report.csv` | `ExportAdminVideoJobsFeedbackCSV` | 导出反馈明细 |
| GET | `/api/admin/video-jobs/feedback-integrity.csv` | `ExportAdminVideoJobsFeedbackIntegrityCSV` | 导出反馈完整性 |
| GET | `/api/admin/video-jobs/feedback-integrity-trend.csv` | `ExportAdminVideoJobsFeedbackIntegrityTrendCSV` | 导出完整性趋势 |
| GET | `/api/admin/video-jobs/feedback-integrity-anomalies.csv` | `ExportAdminVideoJobsFeedbackIntegrityAnomaliesCSV` | 导出异常 |
| GET | `/api/admin/video-jobs/gif-evaluations.csv` | `ExportAdminVideoJobsGIFEvaluationsCSV` | 导出 GIF 评分 |
| GET | `/api/admin/video-jobs/gif-baselines.csv` | `ExportAdminVideoJobsGIFBaselinesCSV` | 导出 GIF 基线 |
| GET | `/api/admin/video-jobs/gif-rerank-logs.csv` | `ExportAdminVideoJobsGIFRerankLogsCSV` | 导出 rerank 日志 |
| GET | `/api/admin/video-jobs/gif-quality-report.csv` | `ExportAdminVideoJobsGIFQualityReportCSV` | 导出质量报告 |
| GET | `/api/admin/video-jobs/gif-manual-compare.csv` | `ExportAdminVideoJobsGIFManualCompareCSV` | 导出人工对比 |
| GET | `/api/admin/video-jobs/gif-sub-stage-anomalies.csv` | `ExportAdminVideoJobsGIFSubStageAnomaliesCSV` | 导出 GIF 子阶段异常 |
| GET | `/api/admin/video-jobs/samples/baseline.csv` | `ExportAdminSampleVideoJobsBaselineCSV` | 导出样本基线 |
| GET | `/api/admin/video-jobs/samples/baseline-diff.csv` | `ExportAdminSampleVideoJobsBaselineDiffCSV` | 导出基线差异 CSV |
| GET | `/api/admin/video-jobs/samples/baseline-diff` | `GetAdminSampleVideoJobsBaselineDiff` | 查看基线差异 |

## 4.8 视频质量配置与 AI Prompt 模板

| Method | Path | Handler | 说明 |
|---|---|---|---|
| GET | `/api/admin/video-jobs/quality-settings` | `GetAdminVideoQualitySetting` | 获取质量配置 |
| PUT | `/api/admin/video-jobs/quality-settings` | `UpdateAdminVideoQualitySetting` | 全量更新质量配置 |
| PATCH | `/api/admin/video-jobs/quality-settings` | `PatchAdminVideoQualitySetting` | 局部更新质量配置 |
| POST | `/api/admin/video-jobs/quality-settings/apply-rollout-suggestion` | `ApplyAdminVideoQualityRolloutSuggestion` | 应用 rollout 建议 |
| GET | `/api/admin/video-jobs/quality-settings/rollout-effects` | `ListAdminVideoQualityRolloutEffects` | 查看 rollout 效果 |
| GET | `/api/admin/video-jobs/ai-prompt-templates` | `GetAdminVideoAIPromptTemplates` | 读取 AI Prompt 模板 |
| PATCH | `/api/admin/video-jobs/ai-prompt-templates` | `PatchAdminVideoAIPromptTemplates` | 修改模板 |
| GET | `/api/admin/video-jobs/ai-prompt-templates/audits` | `ListAdminVideoAIPromptTemplateAudits` | 模板审计 |
| GET | `/api/admin/video-jobs/ai-prompt-templates/versions` | `ListAdminVideoAIPromptTemplateVersions` | 模板版本列表 |
| POST | `/api/admin/video-jobs/ai-prompt-templates/activate` | `ActivateAdminVideoAIPromptTemplateVersion` | 激活模板版本 |
| GET | `/api/admin/video-jobs/gif-health` | `GetAdminVideoJobsGIFHealth` | GIF 健康看板 |
| GET | `/api/admin/video-jobs/gif-health/trend` | `GetAdminVideoJobsGIFHealthTrend` | GIF 健康趋势 |
| GET | `/api/admin/video-jobs/gif-health/trend.csv` | `ExportAdminVideoJobsGIFHealthTrendCSV` | GIF 健康趋势导出 |

## 4.9 算力账户后台

| Method | Path | Handler | 说明 |
|---|---|---|---|
| GET | `/api/admin/compute/accounts` | `ListAdminComputeAccounts` | 算力账户列表 |
| GET | `/api/admin/compute/accounts/:id` | `GetAdminComputeAccount` | 单个算力账户详情 |
| POST | `/api/admin/compute/accounts/:id/adjust` | `AdminAdjustComputeAccount` | 手动调账 |

## 4.10 Meme 管理后台

| Method | Path | Handler | 说明 |
|---|---|---|---|
| POST | `/api/admin/templates` | `UploadTemplate` | 上传 meme 模板 |
| GET | `/api/admin/templates` | `ListMemeTemplates` | 模板列表 |
| POST | `/api/admin/phrases` | `AddPhrase` | 新增梗文案 |
| GET | `/api/admin/phrases` | `ListPhrases` | 文案列表 |

---

## 5. Swagger

- 路径：`/swagger/*any`
- 入口：通常是 `/swagger/index.html`

---

## 6. API 结构观察结论

1. **公共读接口很多，后台接口也非常多**
2. **视频后台接口已经是独立产品级子系统**
3. **AI Prompt Template 管理已纳入后台**
4. **公共组里存在若干高风险写接口**，应优先整改到用户/管理员组
