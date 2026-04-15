package models

import "gorm.io/gorm"

func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&User{},
		&AdminRole{},
		&RefreshToken{},
		&CreatorProfile{},
		&IP{},
		&IPCollectionBinding{},
		&Collection{},
		&VideoAssetCollection{},
		&CollectionZip{},
		&VideoAssetCollectionZip{},
		&CollectionDownload{},
		&CollectionFavorite{},
		&CollectionLike{},
		&Emoji{},
		&VideoAssetEmoji{},
		&Tag{},
		&Category{},
		&EmojiTag{},
		&CollectionTag{},
		&Favorite{},
		&Like{},
		&Download{},
		&Report{},
		&AuditLog{},
		&JoinApplication{},
		&HomeDailyStats{},
		&SiteFooterSetting{},
		&UploadRuleSetting{},
		&UGCCollectionReviewState{},
		&UGCCollectionReviewLog{},
		&RedeemCode{},
		&RedeemCodeRedemption{},
		&ComputeRedeemCode{},
		&ComputeRedeemRedemption{},
		&CollectionDownloadCode{},
		&CollectionDownloadEntitlement{},
		&CollectionDownloadRedemption{},
		&CollectionDownloadConsumption{},
		&CardTheme{},
		&UploadTask{},
		// New meme social models
		&Meme{},
		&MemeTemplate{},
		&MemePhrase{},
		&UserLike{},
		&UserCollect{},
		&SmsCode{},
		&VideoJob{},
		&VideoJobArtifact{},
		&VideoJobEvent{},
		&VideoJobGIFCandidate{},
		&VideoJobGIFEvaluation{},
		&VideoJobCost{},
		&VideoJobAIUsage{},
		&VideoJobGIFAIProposal{},
		&VideoJobGIFAIReview{},
		&VideoJobGIFAIDirective{},
		&VideoJobAI1Plan{},
		&VideoJobImageAIReview{},
		&VideoJobAIReading{},
		&VideoAIPromptTemplate{},
		&VideoAIPromptTemplateAudit{},
		&VideoJobGIFBaseline{},
		&VideoJobGIFRerankLog{},
		&VideoJobGIFManualScore{},
		&ComputeAccount{},
		&ComputeLedger{},
		&ComputePointHold{},
		&VideoQualitySetting{},
		&VideoQualitySettingScoped{},
		&VideoQualityRolloutAudit{},
		&CollectionCopyrightTask{},
		&CollectionCopyrightTaskImage{},
		&ImageCopyrightResult{},
		&CollectionCopyrightResult{},
		&CopyrightReviewRecord{},
		&TagDimension{},
		&TagDefinition{},
		&EmojiAutoTag{},
		&CollectionAutoTag{},
		&CopyrightEvidence{},
		&CopyrightTaskLog{},
		&VideoImageJobPublic{},
		&VideoImageOutputPublic{},
		&VideoImagePackagePublic{},
		&VideoImageEventPublic{},
		&VideoImageFeedbackPublic{},
		&VideoWorkCardPublic{},
		&VideoImageQualitySettingPublic{},
		&VideoImageRolloutAuditPublic{},
		&VideoImageSplitBackfillRun{},
		&GPUImageEnhanceJob{},
		&GPUImageEnhanceAsset{},
		&ExternalAccount{},
		&VideoIngressJob{},
		&FeishuEventLog{},
		&FeishuMessageJob{},
		&FeishuBindCode{},
	); err != nil {
		return err
	}
	return autoMigrateVideoImageFormatSplitTables(db)
}

func autoMigrateVideoImageFormatSplitTables(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	formats := []string{"gif", "png", "jpg", "webp", "live", "mp4"}
	for _, format := range formats {
		suffix := "_" + format
		if err := db.Table("public.video_image_jobs" + suffix).AutoMigrate(&VideoImageJobPublic{}); err != nil {
			return err
		}
		if err := db.Table("public.video_image_outputs" + suffix).AutoMigrate(&VideoImageOutputPublic{}); err != nil {
			return err
		}
		if err := db.Table("public.video_image_packages" + suffix).AutoMigrate(&VideoImagePackagePublic{}); err != nil {
			return err
		}
		if err := db.Table("public.video_image_events" + suffix).AutoMigrate(&VideoImageEventPublic{}); err != nil {
			return err
		}
		if err := db.Table("public.video_image_feedback" + suffix).AutoMigrate(&VideoImageFeedbackPublic{}); err != nil {
			return err
		}
	}
	return nil
}
