package videojobs

import "testing"

func TestValidateProbeMeta(t *testing.T) {
	tests := []struct {
		name    string
		meta    videoProbeMeta
		wantErr bool
	}{
		{
			name: "ok",
			meta: videoProbeMeta{
				DurationSec: 12.5,
				Width:       1920,
				Height:      1080,
				FPS:         25,
			},
			wantErr: false,
		},
		{
			name: "missing stream",
			meta: videoProbeMeta{
				DurationSec: 5,
				Width:       0,
				Height:      1080,
			},
			wantErr: true,
		},
		{
			name: "missing duration",
			meta: videoProbeMeta{
				DurationSec: 0,
				Width:       1920,
				Height:      1080,
			},
			wantErr: true,
		},
		{
			name: "duration too long",
			meta: videoProbeMeta{
				DurationSec: 5 * 60 * 60,
				Width:       1920,
				Height:      1080,
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := validateProbeMeta(tc.meta)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
