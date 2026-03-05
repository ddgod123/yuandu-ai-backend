package router

import (
	"emoji/internal/config"
	"emoji/internal/handlers"
	"emoji/internal/middleware"
	"emoji/internal/storage"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func Setup(cfg config.Config, db *gorm.DB, qiniu *storage.QiniuClient) *gin.Engine {
	if cfg.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	h := handlers.New(db, cfg, qiniu)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	allowedOrigins := cfg.CORSAllowOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"http://localhost:5818", "http://localhost:5918"}
	}
	r.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type", "X-Requested-With"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/healthz", h.Health)

	api := r.Group("/api")
	{
		api.Use(middleware.AuthOptional(cfg))

		api.POST("/auth/register-phone", h.RegisterPhone)
		api.POST("/auth/login-phone", h.LoginPhone)
		api.POST("/auth/refresh", h.Refresh)
		api.POST("/auth/logout", h.Logout)
		api.POST("/auth/send-code", h.SendCode)

		api.GET("/stats/today", h.GetTodayStats)
		api.GET("/stats/home", h.GetHomeStats)
		api.GET("/categories", h.ListPublicCategories)
		api.GET("/ips", h.ListIPs)
		api.GET("/ips/:id", h.GetIP)
		api.GET("/collections", h.ListCollections)
		api.GET("/collections/:id", h.GetCollection)
		api.GET("/card-themes", h.ListCardThemes)
		api.GET("/site-settings/footer", h.GetSiteFooterSetting)
		api.POST("/join-applications", h.CreateJoinApplication)
		api.POST("/collections", h.CreateCollection)
		api.PUT("/collections/:id", h.UpdateCollection)
		api.DELETE("/collections/:id", h.DeleteCollection)

		api.GET("/emojis", h.ListEmojis)
		api.POST("/emojis", h.BatchUploadEmoji)
		api.PUT("/emojis/:id", h.UpdateEmoji)
		api.DELETE("/emojis/:id", h.DeleteEmoji)

		api.POST("/storage/upload-token", h.GetUploadToken)
		api.GET("/storage/object", h.GetObject)
		api.DELETE("/storage/object", h.DeleteObject)
		api.GET("/storage/objects", h.ListObjects)
		api.POST("/storage/rename", h.RenameObject)
		api.GET("/storage/url", h.GetObjectURL)

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
			auth.POST("/me/redeem-code", h.RedeemCodeForMe)
			auth.GET("/me/redeem-records", h.ListMyRedeemRecords)

			// 收藏相关
			auth.POST("/favorites", h.AddFavorite)
			auth.DELETE("/favorites/:emoji_id", h.RemoveFavorite)
			auth.GET("/favorites", h.ListFavorites)
			auth.GET("/favorites/collections", h.ListCollectionFavorites)
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
			admin.GET("/join-applications", h.ListJoinApplications)
			admin.GET("/tag-groups", h.ListTagGroups)
			admin.POST("/tag-groups", h.CreateTagGroup)
			admin.PUT("/tag-groups/:id", h.UpdateTagGroup)
			admin.DELETE("/tag-groups/:id", h.DeleteTagGroup)
			admin.GET("/ops/metrics/summary", h.GetOpsMetricsSummary)
			admin.GET("/ops/metrics/top-categories", h.ListOpsTopCategories)
			admin.GET("/ops/metrics/search-terms", h.ListOpsSearchTerms)
			admin.GET("/upload-tasks", h.ListUploadTasks)
			admin.GET("/users/:id/detail", h.GetAdminUserDetail)
			admin.PUT("/collections/:id", h.AdminUpdateCollection)
			admin.DELETE("/collections/:id", h.AdminDeleteCollection)
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
		}
	}

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}
