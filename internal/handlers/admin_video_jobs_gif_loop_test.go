package handlers

import (
	"errors"
	"testing"
)

func TestIsMissingPublicGIFLoopColumnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "missing table",
			err:  errors.New(`ERROR: relation "public.video_image_outputs" does not exist`),
			want: true,
		},
		{
			name: "missing column",
			err:  errors.New(`ERROR: column "gif_loop_tune_applied" does not exist`),
			want: true,
		},
		{
			name: "other sql error",
			err:  errors.New("ERROR: connection reset by peer"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isMissingPublicGIFLoopColumnsError(tc.err)
			if got != tc.want {
				t.Fatalf("unexpected result: got=%v want=%v", got, tc.want)
			}
		})
	}
}
