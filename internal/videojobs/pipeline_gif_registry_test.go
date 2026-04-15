package videojobs

import "testing"

func TestGIFPipelineStageRegistry_ExecuteOrder(t *testing.T) {
	registry := newGIFPipelineStageRegistry()
	got := make([]string, 0, 3)
	registry.Register("briefing", func() error {
		got = append(got, "briefing")
		return nil
	})
	registry.Register("planning", func() error {
		got = append(got, "planning")
		return nil
	})
	registry.Register("scoring", func() error {
		got = append(got, "scoring")
		return nil
	})

	if err := registry.Execute(); err != nil {
		t.Fatalf("expected execute success, got err=%v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 stages executed, got %d", len(got))
	}
	if got[0] != "briefing" || got[1] != "planning" || got[2] != "scoring" {
		t.Fatalf("unexpected execution order: %+v", got)
	}
}

func TestGIFPipelineStageRegistry_StopOnHalt(t *testing.T) {
	registry := newGIFPipelineStageRegistry()
	got := make([]string, 0, 3)
	registry.Register("briefing", func() error {
		got = append(got, "briefing")
		return nil
	})
	registry.Register("halt", func() error {
		got = append(got, "halt")
		return errGIFPipelineHalt
	})
	registry.Register("never", func() error {
		got = append(got, "never")
		return nil
	})

	err := registry.Execute()
	if err == nil {
		t.Fatalf("expected halt error")
	}
	if err != errGIFPipelineHalt {
		t.Fatalf("expected errGIFPipelineHalt, got %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected execution to stop at halt stage, got %+v", got)
	}
}
