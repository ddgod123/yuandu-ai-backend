package handlers

import (
	"testing"

	"emoji/internal/models"
)

func TestValidateVideoJobFeedbackTarget(t *testing.T) {
	output := &models.VideoImageOutputPublic{ID: 101, ObjectKey: "emoji/video-image/dev/u/1/101/j/22/outputs/gif/main.gif"}

	t.Run("output required", func(t *testing.T) {
		if err := validateVideoJobFeedbackTarget(nil, nil); err == nil {
			t.Fatalf("expected error when output is nil")
		}
	})

	t.Run("output only is valid", func(t *testing.T) {
		if err := validateVideoJobFeedbackTarget(nil, output); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("emoji and output key must match", func(t *testing.T) {
		emoji := &models.Emoji{ID: 8, FileURL: "emoji/video-image/dev/u/1/101/j/22/outputs/gif/another.gif"}
		if err := validateVideoJobFeedbackTarget(emoji, output); err == nil {
			t.Fatalf("expected mismatch error")
		}
	})

	t.Run("emoji and output key matched", func(t *testing.T) {
		emoji := &models.Emoji{ID: 9, FileURL: output.ObjectKey}
		if err := validateVideoJobFeedbackTarget(emoji, output); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestParseVideoImageFeedbackActionAllowlist(t *testing.T) {
	tests := []struct {
		raw        string
		wantAction string
		wantOK     bool
	}{
		{raw: "download", wantAction: videoImageFeedbackActionDownload, wantOK: true},
		{raw: "favorite", wantAction: videoImageFeedbackActionFavorite, wantOK: true},
		{raw: "top_pick", wantAction: videoImageFeedbackActionTopPick, wantOK: true},
		{raw: "thumb_up", wantAction: videoImageFeedbackActionLike, wantOK: true},
		{raw: "invalid_action", wantAction: "", wantOK: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.raw, func(t *testing.T) {
			action, _, ok := parseVideoImageFeedbackAction(tc.raw)
			if ok != tc.wantOK {
				t.Fatalf("unexpected ok: got=%v want=%v", ok, tc.wantOK)
			}
			if action != tc.wantAction {
				t.Fatalf("unexpected action: got=%s want=%s", action, tc.wantAction)
			}
		})
	}
}
