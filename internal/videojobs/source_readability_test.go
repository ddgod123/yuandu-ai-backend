package videojobs

import (
	"errors"
	"testing"
)

func TestSourceHTTPStatusFromErr(t *testing.T) {
	status, ok := sourceHTTPStatusFromErr(errors.New("unexpected status 404"))
	if !ok {
		t.Fatalf("expected status parse ok")
	}
	if status != 404 {
		t.Fatalf("expected 404, got %d", status)
	}
}

func TestClassifySourceReadabilityFailureNotFound(t *testing.T) {
	reason, hint, permanent := classifySourceReadabilityFailure(
		errors.New("download failed"),
		[]map[string]interface{}{
			{
				"step":        "qiniu_signed_url",
				"http_status": int64(404),
				"error":       "unexpected status 404",
			},
		},
	)
	if reason != "source_video_not_found" {
		t.Fatalf("unexpected reason: %s", reason)
	}
	if !permanent {
		t.Fatalf("expected permanent=true")
	}
	if hint == "" {
		t.Fatalf("expected hint")
	}
}

func TestClassifySourceReadabilityFailureNetwork(t *testing.T) {
	reason, _, permanent := classifySourceReadabilityFailure(
		errors.New("download failed"),
		[]map[string]interface{}{
			{
				"step":  "qiniu_signed_url",
				"error": "dial tcp: no such host",
			},
		},
	)
	if reason != "source_video_network_unstable" {
		t.Fatalf("unexpected reason: %s", reason)
	}
	if permanent {
		t.Fatalf("expected permanent=false")
	}
}
