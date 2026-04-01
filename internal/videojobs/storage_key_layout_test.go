package videojobs

import "testing"

func TestVideoImageStorageLayoutKeys(t *testing.T) {
	layout := NewVideoImageStorageLayout("staging")

	prefix := layout.JobPrefix(12345, 678)
	if prefix != "emoji/video-image/staging/u/45/12345/j/678/" {
		t.Fatalf("unexpected prefix: %s", prefix)
	}

	source := layout.SourceKey(12345, 678, "input.MP4")
	if source != "emoji/video-image/staging/u/45/12345/j/678/source/input.mp4" {
		t.Fatalf("unexpected source key: %s", source)
	}

	gif := layout.OutputKey(12345, 678, "gif", 3, "gif")
	if gif != "emoji/video-image/staging/u/45/12345/j/678/outputs/gif/003.gif" {
		t.Fatalf("unexpected gif key: %s", gif)
	}

	thumb := layout.ThumbnailKey(12345, 678, "gif", 3)
	if thumb != "emoji/video-image/staging/u/45/12345/j/678/outputs/gif/thumb_003.jpg" {
		t.Fatalf("unexpected thumb key: %s", thumb)
	}

	pkg := layout.PackageKey(12345, 678, "gif", 2)
	if pkg != "emoji/video-image/staging/u/45/12345/j/678/package/678_gif_v2.zip" {
		t.Fatalf("unexpected package key: %s", pkg)
	}

	manifest := layout.ManifestKey(12345, 678)
	if manifest != "emoji/video-image/staging/u/45/12345/j/678/manifest/result_manifest_v1.json" {
		t.Fatalf("unexpected manifest key: %s", manifest)
	}
}

func TestVideoImageStorageLayoutDefaults(t *testing.T) {
	layout := NewVideoImageStorageLayout("")
	if layout.Env != "prod" {
		t.Fatalf("expected default env prod, got %s", layout.Env)
	}
	if layout.UserShard(1) != "01" {
		t.Fatalf("unexpected shard for user 1: %s", layout.UserShard(1))
	}
	if layout.UserShard(100) != "00" {
		t.Fatalf("unexpected shard for user 100: %s", layout.UserShard(100))
	}
}

func TestVideoImageStorageLayoutJobPrefixByFormat(t *testing.T) {
	layout := NewVideoImageStorageLayout("prod")
	prefix := layout.JobPrefixByFormat(77, 9001, "png")
	if prefix != "emoji/video-image/prod/f/png/u/77/77/j/9001/" {
		t.Fatalf("unexpected format prefix: %s", prefix)
	}
}

func TestVideoImageStorageLayoutWithCustomRootPrefix(t *testing.T) {
	layout := NewVideoImageStorageLayoutWithRoot("prod", "emoji-prod")
	prefix := layout.JobPrefix(9, 100)
	if prefix != "emoji-prod/video-image/prod/u/09/9/j/100/" {
		t.Fatalf("unexpected custom root prefix path: %s", prefix)
	}
}
