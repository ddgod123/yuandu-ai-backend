package models

import "gorm.io/gorm"

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&User{},
		&AdminRole{},
		&RefreshToken{},
		&CreatorProfile{},
		&Collection{},
		&CollectionZip{},
		&CollectionDownload{},
		&CollectionFavorite{},
		&CollectionLike{},
		&Emoji{},
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
		&RedeemCode{},
		&RedeemCodeRedemption{},
		&CardTheme{},
		&UploadTask{},
	)
}
