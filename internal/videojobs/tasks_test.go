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
			wantQueue:   QueueVideoJobPNG,
			wantTask:    TaskTypeProcessVideoJobPNG,
			wantPrimary: "jpg",
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
