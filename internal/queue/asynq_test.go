package queue

import (
	"testing"

	"emoji/internal/videojobs"
)

func TestResolveVideoWorkerRole(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "all"},
		{name: "all", in: "all", want: "all"},
		{name: "default", in: "default", want: "all"},
		{name: "gif", in: "gif", want: "gif"},
		{name: "png", in: "png", want: "png"},
		{name: "jpg", in: "jpg", want: "jpg"},
		{name: "webp", in: "webp", want: "webp"},
		{name: "live", in: "live", want: "live"},
		{name: "mp4", in: "mp4", want: "mp4"},
		{name: "image alias", in: "image", want: "image"},
		{name: "media", in: "media", want: "media"},
		{name: "unknown", in: "abc", want: "all"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveVideoWorkerRole(tc.in)
			if got != tc.want {
				t.Fatalf("resolveVideoWorkerRole(%q)=%q want=%q", tc.in, got, tc.want)
			}
		})
	}
}

func TestApplyVideoWorkerRoleFilter(t *testing.T) {
	base := map[string]int{
		"default":                    4,
		videojobs.QueueVideoJobGIF:   7,
		videojobs.QueueVideoJobPNG:   9,
		videojobs.QueueVideoJobJPG:   8,
		videojobs.QueueVideoJobWEBP:  6,
		videojobs.QueueVideoJobLIVE:  5,
		videojobs.QueueVideoJobMP4:   4,
		videojobs.QueueVideoJobMedia: 2,
	}

	cases := []struct {
		name    string
		role    string
		wantKey string
		wantVal int
	}{
		{name: "gif", role: "gif", wantKey: videojobs.QueueVideoJobGIF, wantVal: 7},
		{name: "png", role: "png", wantKey: videojobs.QueueVideoJobPNG, wantVal: 9},
		{name: "jpg", role: "jpg", wantKey: videojobs.QueueVideoJobJPG, wantVal: 8},
		{name: "webp", role: "webp", wantKey: videojobs.QueueVideoJobWEBP, wantVal: 6},
		{name: "live", role: "live", wantKey: videojobs.QueueVideoJobLIVE, wantVal: 5},
		{name: "mp4", role: "mp4", wantKey: videojobs.QueueVideoJobMP4, wantVal: 4},
		{name: "media", role: "media", wantKey: videojobs.QueueVideoJobMedia, wantVal: 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := applyVideoWorkerRoleFilter(base, tc.role)
			if len(got) != 1 {
				t.Fatalf("expected exactly 1 queue, got=%v", got)
			}
			if got[tc.wantKey] != tc.wantVal {
				t.Fatalf("queue %s weight=%d want=%d", tc.wantKey, got[tc.wantKey], tc.wantVal)
			}
		})
	}

	t.Run("all keeps full map", func(t *testing.T) {
		got := applyVideoWorkerRoleFilter(base, "all")
		if len(got) != len(base) {
			t.Fatalf("expected full map size=%d, got=%d", len(base), len(got))
		}
	})

	t.Run("image selects still lanes", func(t *testing.T) {
		got := applyVideoWorkerRoleFilter(base, "image")
		want := map[string]int{
			videojobs.QueueVideoJobPNG:  9,
			videojobs.QueueVideoJobJPG:  8,
			videojobs.QueueVideoJobWEBP: 6,
			videojobs.QueueVideoJobLIVE: 5,
			videojobs.QueueVideoJobMP4:  4,
		}
		if len(got) != len(want) {
			t.Fatalf("expected %d queues, got=%v", len(want), got)
		}
		for key, val := range want {
			if got[key] != val {
				t.Fatalf("queue %s weight=%d want=%d", key, got[key], val)
			}
		}
	})
}

func TestResolveAsynqQueueWeightsFromEnvWithRole(t *testing.T) {
	t.Setenv("ASYNQ_QUEUE_WEIGHTS", "video_gif=5,video_png=3,video_jpg=2")
	t.Setenv("VIDEO_WORKER_ROLE", "gif")
	got := ResolveAsynqQueueWeightsFromEnv()
	if len(got) != 1 {
		t.Fatalf("expected 1 queue, got=%v", got)
	}
	if got[videojobs.QueueVideoJobGIF] != 5 {
		t.Fatalf("gif queue weight=%d want=5", got[videojobs.QueueVideoJobGIF])
	}
}
