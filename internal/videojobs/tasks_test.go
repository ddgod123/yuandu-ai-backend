package videojobs

import "testing"

func TestResolveVideoJobExecutionTarget(t *testing.T) {
	cases := []struct {
		name        string
		output      string
		wantQueue   string
		wantTask    string
		wantPrimary string
	}{
		{
			name:        "gif",
			output:      "gif",
			wantQueue:   QueueVideoJobGIF,
			wantTask:    TaskTypeProcessVideoJobGIF,
			wantPrimary: "gif",
		},
		{
			name:        "png",
			output:      "png",
			wantQueue:   QueueVideoJobPNG,
			wantTask:    TaskTypeProcessVideoJobPNG,
			wantPrimary: "png",
		},
		{
			name:        "jpg alias",
			output:      "jpeg",
			wantQueue:   QueueVideoJobJPG,
			wantTask:    TaskTypeProcessVideoJobJPG,
			wantPrimary: "jpg",
		},
		{
			name:        "webp",
			output:      "webp",
			wantQueue:   QueueVideoJobWEBP,
			wantTask:    TaskTypeProcessVideoJobWEBP,
			wantPrimary: "webp",
		},
		{
			name:        "live",
			output:      "live",
			wantQueue:   QueueVideoJobLIVE,
			wantTask:    TaskTypeProcessVideoJobLIVE,
			wantPrimary: "live",
		},
		{
			name:        "mp4",
			output:      "mp4",
			wantQueue:   QueueVideoJobMP4,
			wantTask:    TaskTypeProcessVideoJobMP4,
			wantPrimary: "mp4",
		},
		{
			name:        "fallback",
			output:      "",
			wantQueue:   QueueVideoJobMedia,
			wantTask:    TaskTypeProcessVideoJob,
			wantPrimary: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			queue, taskType, primary := ResolveVideoJobExecutionTarget(tc.output)
			if queue != tc.wantQueue || taskType != tc.wantTask || primary != tc.wantPrimary {
				t.Fatalf("got queue=%s task=%s primary=%s, want queue=%s task=%s primary=%s",
					queue, taskType, primary, tc.wantQueue, tc.wantTask, tc.wantPrimary)
			}
		})
	}
}
