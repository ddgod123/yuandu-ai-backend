package router

import (
	"emoji/internal/config"
	"emoji/internal/handlers"
	"emoji/internal/middleware"
	"emoji/internal/service"
	"emoji/internal/storage"
	"emoji/pkg/oss"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func Setup(cfg config.Config, db *gorm.DB, qiniu *storage.QiniuClient, ossClient *oss.Client, ai *service.AIService, compose *service.ComposeService) *gin.Engine {
	if cfg.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	h := handlers.New(db, cfg, qiniu, ossClient, ai, compose)

	r := gin.New()
	r.Use(gin.Logger(), middleware.RecoveryWithStack())

	allowedOrigins := cfg.CORSAllowOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"http://localhost:5818", "http://localhost:5918"}
	}
	r.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type", "X-Requested-With", "X-Device-ID"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/healthz", h.Health)

	api := r.Group("/api")
	{
		api.Use(middleware.AuthOptional(cfg))

		api.POST("/auth/register-phone", h.RegisterPhone)
		api.POST("/auth/login-phone", h.LoginPhone)
		api.POST("/auth/login", h.Login)
		api.POST("/auth/refresh", h.Refresh)
		api.POST("/auth/logout", h.Logout)
		api.GET("/auth/captcha", h.GetCaptcha)
		api.POST("/auth/send-code", h.SendCode)

		api.GET("/stats/today", h.GetTodayStats)
		api.GET("/stats/home", h.GetHomeStats)
		api.POST("/behavior/events", h.TrackUserBehaviorEvent)
		api.GET("/categories", h.ListPublicCategories)
		api.GET("/ips", h.ListIPs)
		api.GET("/ips/:id", h.GetIP)
		api.GET("/ips/:id/collections", h.GetIPCollections)
		api.GET("/collections", h.ListCollections)
		api.GET("/collections/:id", h.GetCollection)
		api.GET("/card-themes", h.ListCardThemes)
		api.GET("/site-settings/footer", h.GetSiteFooterSetting)
		api.POST("/join-applications", h.CreateJoinApplication)

		api.GET("/emojis", h.ListEmojis)

		api.GET("/storage/proxy", h.ProxyObject)
		api.GET("/storage/url", h.GetObjectURL)
		api.POST("/storage/urls", h.GetObjectURLs)
		api.GET("/download/ticket/:token", h.DownloadByTicket)

		// Meme social — public feed
		api.GET("/v1/memes/feed", h.FeedMemes)

		auth := api.Group("", middleware.Auth(cfg))
		{
			auth.GET("/collections/:id/download-zip", h.GetCollectionZipDownload)
			auth.GET("/collections/:id/download-zip-all", h.GetCollectionZipDownloadAll)
			auth.GET("/collections/:id/zips", h.GetCollectionZipList)
			auth.GET("/collections/:id/download-list", h.GetCollectionDownloadList)
			auth.POST("/collections/:id/like", h.AddCollectionLike)
			auth.DELETE("/collections/:id/like", h.RemoveCollectionLike)
			auth.POST("/collections/:id/favorite", h.AddCollectionFavorite)
			auth.DELETE("/collections/:id/favorite", h.RemoveCollectionFavorite)
			auth.GET("/emojis/:id/download", h.GetEmojiDownload)
			auth.GET("/emojis/:id/download-file", h.DownloadEmojiFile)
			auth.GET("/me", h.Me)
			auth.PUT("/me", h.UpdateMe)
			auth.GET("/me/compute-account", h.GetMyComputeAccount)
			auth.POST("/me/redeem-code/validate", h.ValidateRedeemCodeForMe)
			auth.POST("/me/redeem-code", h.RedeemCodeForMe)
			auth.GET("/me/redeem-records", h.ListMyRedeemRecords)
			auth.POST("/me/compute-redeem-code/validate", h.ValidateComputeRedeemCodeForMe)
			auth.POST("/me/compute-redeem-code/redeem", h.RedeemComputeCodeForMe)
			auth.GET("/me/compute-redeem-records", h.ListMyComputeRedeemRecords)
			auth.POST("/me/collection-download-code/validate", h.ValidateCollectionDownloadCodeForMe)
			auth.POST("/me/collection-download-code/redeem", h.RedeemCollectionDownloadCodeForMe)
			auth.GET("/me/collection-download-entitlements", h.ListMyCollectionDownloadEntitlements)
			auth.GET("/my/works", h.ListMyWorks)
			auth.POST("/video-jobs", h.CreateVideoJob)
			auth.GET("/video-jobs", h.ListMyVideoJobs)
			auth.GET("/video-jobs/capabilities", h.GetVideoJobCapabilities)
			auth.GET("/video-jobs/advanced-scene-options", h.GetVideoJobAdvancedSceneOptions)
			auth.POST("/video-jobs/upload-token", h.GetVideoJobUploadToken)
			auth.POST("/video-jobs/source-probe", h.ProbeVideoSource)
			auth.POST("/video-jobs/source-url-probe", h.ProbeSourceVideoURLMock)
			auth.GET("/video-jobs/:id", h.GetVideoJob)
			auth.GET("/video-jobs/:id/events", h.ListVideoJobEvents)
			auth.GET("/video-jobs/:id/stream", h.StreamVideoJobEvents)
			auth.GET("/video-jobs/:id/ai1-plan", h.GetVideoJobAI1Plan)
			auth.PATCH("/video-jobs/:id/ai1-plan", h.PatchVideoJobAI1Plan)
			auth.GET("/video-jobs/:id/ai1-debug", h.GetVideoJobAI1Debug)
			auth.GET("/video-jobs/:id/result", h.GetVideoJobResult)
			auth.GET("/video-jobs/:id/download-zip", h.GetVideoJobZipDownload)
			auth.POST("/video-jobs/:id/feedback", h.SubmitVideoJobFeedback)
			auth.POST("/video-jobs/:id/confirm-ai1", h.ConfirmVideoJobAI1)
			auth.POST("/video-jobs/:id/cancel", h.CancelVideoJob)
			auth.POST("/video-jobs/:id/delete-collection", h.DeleteVideoJobCollection)
			auth.POST("/video-jobs/:id/delete-output", h.DeleteVideoJobOutput)

			// 收藏相关
			auth.POST("/favorites", h.AddFavorite)
			auth.DELETE("/favorites/:emoji_id", h.RemoveFavorite)
			auth.GET("/favorites", h.ListFavorites)
			auth.GET("/favorites/collections", h.ListCollectionFavorites)

			// Meme social — authenticated
			auth.POST("/v1/memes/generate", h.GenerateMeme)
			auth.POST("/v1/memes/:id/like", h.ToggleMemeLike)
			auth.POST("/v1/memes/:id/collect", h.ToggleMemeCollect)
			auth.GET("/v1/users/me/memes", h.MyMemes)
			auth.GET("/v1/users/me/collections", h.MyMemeCollections)
		}

		adminCompat := api.Group("", middleware.Auth(cfg), middleware.RequireAnyRole("super_admin"))
		{
			// Lock down content write APIs behind super-admin auth while keeping legacy paths
			// for existing admin clients.
			adminCompat.POST("/collections", h.CreateCollection)
			adminCompat.PUT("/collections/:id", h.UpdateCollection)
			adminCompat.DELETE("/collections/:id", h.DeleteCollection)
			adminCompat.POST("/emojis", h.BatchUploadEmoji)
			adminCompat.PUT("/emojis/:id", h.UpdateEmoji)
			adminCompat.DELETE("/emojis/:id", h.DeleteEmoji)

			// Lock down storage management APIs; signed URL issuing is enforced by handler role checks.
			adminCompat.POST("/storage/upload-token", h.GetUploadToken)
			adminCompat.GET("/storage/object", h.GetObject)
			adminCompat.DELETE("/storage/object", h.DeleteObject)
			adminCompat.GET("/storage/objects", h.ListObjects)
			adminCompat.POST("/storage/rename", h.RenameObject)
		}

		admin := api.Group("/admin", middleware.Auth(cfg), middleware.RequireAnyRole("super_admin"))
		{
			admin.GET("/users", h.ListUsers)
			admin.PUT("/users/:id/role", h.UpdateUserRole)
			admin.PUT("/users/:id/status", h.UpdateUserStatus)
			admin.POST("/telegram/download", h.DownloadTelegram)
			admin.GET("/categories", h.ListCategories)
			admin.POST("/categories", h.CreateCategory)
			admin.PUT("/categories/:id", h.UpdateCategory)
			admin.DELETE("/categories/:id", h.DeleteCategory)
			admin.GET("/ips", h.ListAdminIPs)
			admin.GET("/ips/:id", h.GetAdminIP)
			admin.POST("/ips", h.CreateIP)
			admin.PUT("/ips/:id", h.UpdateIP)
			admin.DELETE("/ips/:id", h.DeleteIP)
			admin.GET("/categories/stats", h.ListCategoryStats)
			admin.GET("/categories/:id/objects", h.ListCategoryObjects)
			admin.DELETE("/storage/object", h.AdminDeleteObject)
			admin.POST("/storage/batch-delete", h.BatchDeleteObjects)
			admin.GET("/storage/trash", h.ListTrashObjects)
			admin.DELETE("/storage/trash", h.EmptyTrash)
			admin.POST("/storage/trash/restore", h.RestoreTrashObject)
			admin.POST("/storage/trash/batch-restore", h.BatchRestoreTrashObjects)
			admin.GET("/storage/search", h.AdminSearchObjects)
			admin.GET("/tags", h.ListTags)
			admin.POST("/tags", h.CreateTag)
			admin.PUT("/tags/:id", h.UpdateTag)
			admin.DELETE("/tags/:id", h.DeleteTag)
			admin.GET("/copyright/collections", h.ListAdminCopyrightCollections)
			admin.GET("/copyright/collections/:id", h.GetAdminCopyrightCollection)
			admin.GET("/copyright/collections/:id/images", h.ListAdminCopyrightCollectionImages)
			admin.GET("/copyright/images/:id", h.GetAdminCopyrightImageDetail)
			admin.POST("/copyright/images/:id/tags", h.UpdateAdminCopyrightImageTags)
			admin.POST("/copyright/tasks", h.CreateAdminCopyrightTask)
			admin.GET("/copyright/tasks", h.ListAdminCopyrightTasks)
			admin.GET("/copyright/tasks/:id", h.GetAdminCopyrightTask)
			admin.GET("/copyright/tasks/:id/logs", h.ListAdminCopyrightTaskLogs)
			admin.GET("/copyright/reviews/pending", h.ListAdminCopyrightPendingReviews)
			admin.POST("/copyright/reviews", h.SubmitAdminCopyrightReview)
			admin.GET("/copyright/tag-dimensions", h.ListAdminTagDimensions)
			admin.GET("/copyright/tag-definitions", h.ListAdminTagDefinitions)
			admin.POST("/copyright/tag-definitions", h.CreateAdminTagDefinition)
			admin.PUT("/copyright/tag-definitions/:id", h.UpdateAdminTagDefinition)
			admin.GET("/join-applications", h.ListJoinApplications)
			admin.GET("/tag-groups", h.ListTagGroups)
			admin.POST("/tag-groups", h.CreateTagGroup)
			admin.PUT("/tag-groups/:id", h.UpdateTagGroup)
			admin.DELETE("/tag-groups/:id", h.DeleteTagGroup)
			admin.GET("/ops/metrics/summary", h.GetOpsMetricsSummary)
			admin.GET("/ops/metrics/top-categories", h.ListOpsTopCategories)
			admin.GET("/ops/metrics/search-terms", h.ListOpsSearchTerms)
			admin.GET("/security/overview", h.GetSecurityOverview)
			admin.GET("/security/blacklists", h.ListRiskBlacklists)
			admin.POST("/security/blacklists", h.CreateRiskBlacklist)
			admin.PUT("/security/blacklists/:id/status", h.UpdateRiskBlacklistStatus)
			admin.DELETE("/security/blacklists/:id", h.DeleteRiskBlacklist)
			admin.GET("/security/events", h.ListRiskEvents)
			admin.GET("/upload-tasks", h.ListUploadTasks)
			admin.GET("/dashboard/trends", h.GetAdminDashboardTrends)
			admin.GET("/system/worker-health", h.GetAdminWorkerHealth)
			admin.POST("/system/worker-start", h.StartAdminWorker)
			admin.POST("/system/worker-stop", h.StopAdminWorker)
			admin.POST("/system/worker-guard/run", h.RunAdminWorkerGuard)
			admin.GET("/system/data-audit/overview", h.GetAdminDataAuditOverview)
			admin.GET("/users/:id/detail", h.GetAdminUserDetail)
			admin.PUT("/collections/:id", h.AdminUpdateCollection)
			admin.DELETE("/collections/:id", h.AdminDeleteCollection)
			admin.POST("/collections/batch-sample", h.AdminBatchUpdateCollectionSample)
			admin.POST("/collections/batch-assign-ip", h.AdminBatchAssignCollectionIP)
			admin.POST("/collections/batch-visibility", h.AdminBatchUpdateCollectionVisibility)
			admin.GET("/collections/ip-stats", h.GetAdminCollectionIPStats)
			admin.GET("/collections/ip-audit-logs", h.GetAdminCollectionIPAuditLogs)
			admin.GET("/collections/samples/export.csv", h.AdminExportSampleCollectionsCSV)
			admin.POST("/collections/:id/import-zip", h.AppendCollectionZip)
			admin.POST("/collections/:id/emojis/upload", h.UploadCollectionEmojis)
			admin.GET("/themes", h.ListThemes)
			admin.POST("/themes", h.CreateTheme)
			admin.PUT("/themes/:id", h.UpdateTheme)
			admin.DELETE("/themes/:id", h.DeleteTheme)
			admin.POST("/collections/import-zip", h.ImportCollectionZip)
			admin.GET("/site-settings/footer", h.GetAdminSiteFooterSetting)
			admin.PUT("/site-settings/footer", h.UpdateAdminSiteFooterSetting)
			admin.GET("/redeem-codes", h.ListRedeemCodes)
			admin.POST("/redeem-codes/generate", h.GenerateRedeemCodes)
			admin.PUT("/redeem-codes/:id/status", h.UpdateRedeemCodeStatus)
			admin.GET("/redeem-codes/:id/redemptions", h.ListRedeemCodeRedemptions)
			admin.GET("/compute-redeem-codes", h.ListComputeRedeemCodes)
			admin.POST("/compute-redeem-codes/generate", h.GenerateComputeRedeemCodes)
			admin.PUT("/compute-redeem-codes/:id/status", h.UpdateComputeRedeemCodeStatus)
			admin.GET("/compute-redeem-codes/:id/redemptions", h.ListComputeRedeemCodeRedemptions)
			admin.GET("/collection-download-codes", h.ListCollectionDownloadCodes)
			admin.POST("/collection-download-codes/generate", h.GenerateCollectionDownloadCodes)
			admin.PUT("/collection-download-codes/:id/status", h.UpdateCollectionDownloadCodeStatus)
			admin.GET("/collection-download-codes/:id/redemptions", h.ListCollectionDownloadCodeRedemptions)
			admin.GET("/collection-download-entitlements", h.ListAdminCollectionDownloadEntitlements)
			admin.PUT("/collection-download-entitlements/:id", h.UpdateAdminCollectionDownloadEntitlement)
			admin.GET("/video-jobs/overview", h.GetAdminVideoJobsOverview)
			admin.GET("/video-jobs/feedback-integrity/overview", h.GetAdminVideoJobsFeedbackIntegrityOverview)
			admin.GET("/video-jobs/feedback-report.csv", h.ExportAdminVideoJobsFeedbackCSV)
			admin.GET("/video-jobs/feedback-integrity.csv", h.ExportAdminVideoJobsFeedbackIntegrityCSV)
			admin.GET("/video-jobs/feedback-integrity-trend.csv", h.ExportAdminVideoJobsFeedbackIntegrityTrendCSV)
			admin.GET("/video-jobs/feedback-integrity/drilldown", h.GetAdminVideoJobsFeedbackIntegrityDrilldown)
			admin.GET("/video-jobs/feedback-integrity-anomalies.csv", h.ExportAdminVideoJobsFeedbackIntegrityAnomaliesCSV)
			admin.GET("/video-jobs/gif-evaluations.csv", h.ExportAdminVideoJobsGIFEvaluationsCSV)
			admin.GET("/video-jobs/gif-baselines.csv", h.ExportAdminVideoJobsGIFBaselinesCSV)
			admin.GET("/video-jobs/gif-rerank-logs.csv", h.ExportAdminVideoJobsGIFRerankLogsCSV)
			admin.GET("/video-jobs/gif-quality-report.csv", h.ExportAdminVideoJobsGIFQualityReportCSV)
			admin.GET("/video-jobs/gif-manual-compare.csv", h.ExportAdminVideoJobsGIFManualCompareCSV)
			admin.GET("/video-jobs/gif-sub-stage-anomalies.csv", h.ExportAdminVideoJobsGIFSubStageAnomaliesCSV)
			admin.GET("/video-jobs/samples/baseline.csv", h.ExportAdminSampleVideoJobsBaselineCSV)
			admin.GET("/video-jobs/samples/baseline-diff.csv", h.ExportAdminSampleVideoJobsBaselineDiffCSV)
			admin.GET("/video-jobs/samples/baseline-diff", h.GetAdminSampleVideoJobsBaselineDiff)
			admin.GET("/video-jobs", h.ListAdminVideoJobs)
			admin.GET("/video-jobs/read-route", h.GetAdminVideoImageReadRoute)
			admin.POST("/video-jobs/read-route/debug", h.SetAdminVideoImageReadRouteDebug)
			admin.GET("/video-jobs/split-backfill", h.GetAdminVideoImageSplitBackfillStatus)
			admin.POST("/video-jobs/split-backfill/start", h.StartAdminVideoImageSplitBackfill)
			admin.POST("/video-jobs/split-backfill/stop", h.StopAdminVideoImageSplitBackfill)
			admin.GET("/video-jobs/quality-settings", h.GetAdminVideoQualitySetting)
			admin.GET("/video-jobs/quality-settings/audits", h.ListAdminVideoQualitySettingAudits)
			admin.PUT("/video-jobs/quality-settings", h.UpdateAdminVideoQualitySetting)
			admin.PATCH("/video-jobs/quality-settings", h.PatchAdminVideoQualitySetting)
			admin.POST("/video-jobs/quality-settings/apply-rollout-suggestion", h.ApplyAdminVideoQualityRolloutSuggestion)
			admin.GET("/video-jobs/quality-settings/rollout-effects", h.ListAdminVideoQualityRolloutEffects)
			admin.GET("/video-jobs/ai-prompt-templates", h.GetAdminVideoAIPromptTemplates)
			admin.PATCH("/video-jobs/ai-prompt-templates", h.PatchAdminVideoAIPromptTemplates)
			admin.GET("/video-jobs/ai-prompt-templates/audits", h.ListAdminVideoAIPromptTemplateAudits)
			admin.GET("/video-jobs/ai-prompt-templates/versions", h.ListAdminVideoAIPromptTemplateVersions)
			admin.POST("/video-jobs/ai-prompt-templates/activate", h.ActivateAdminVideoAIPromptTemplateVersion)
			admin.GET("/video-jobs/gif-health", h.GetAdminVideoJobsGIFHealth)
			admin.GET("/video-jobs/gif-health/trend", h.GetAdminVideoJobsGIFHealthTrend)
			admin.GET("/video-jobs/gif-health/trend.csv", h.ExportAdminVideoJobsGIFHealthTrendCSV)
			admin.GET("/video-jobs/:id/download-zip", h.GetAdminVideoJobZipDownload)
			admin.GET("/video-jobs/:id/health", h.GetAdminVideoJobHealth)
			admin.GET("/video-jobs/:id/gif-audit-chain", h.GetAdminVideoJobGIFAuditChain)
			admin.GET("/video-jobs/:id", h.GetAdminVideoJob)
			admin.POST("/video-jobs/:id/rerender-gif", h.AdminRerenderVideoJobGIF)
			admin.POST("/video-jobs/:id/rerender-gif/batch", h.AdminBatchRerenderVideoJobGIF)
			admin.POST("/video-jobs/:id/gif-review-decisions", h.SubmitAdminVideoJobGIFReviewDecisions)
			admin.POST("/video-jobs/:id/delete-collection", h.AdminDeleteVideoJobCollection)
			admin.POST("/video-jobs/:id/delete-output", h.AdminDeleteVideoJobOutput)
			admin.GET("/compute/accounts", h.ListAdminComputeAccounts)
			admin.GET("/compute/accounts/:id", h.GetAdminComputeAccount)
			admin.POST("/compute/accounts/:id/adjust", h.AdminAdjustComputeAccount)

			// Meme admin
			admin.POST("/templates", h.UploadTemplate)
			admin.GET("/templates", h.ListMemeTemplates)
			admin.POST("/phrases", h.AddPhrase)
			admin.GET("/phrases", h.ListPhrases)
		}
	}

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}
