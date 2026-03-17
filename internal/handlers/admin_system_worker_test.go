package handlers

import "testing"

func TestElevateAdminWorkerHealth(t *testing.T) {
	if got := elevateAdminWorkerHealth("green", "yellow"); got != "yellow" {
		t.Fatalf("expected yellow, got %s", got)
	}
	if got := elevateAdminWorkerHealth("yellow", "green"); got != "yellow" {
		t.Fatalf("expected yellow, got %s", got)
	}
	if got := elevateAdminWorkerHealth("yellow", "red"); got != "red" {
		t.Fatalf("expected red, got %s", got)
	}
}

func TestFinalizeAdminWorkerHealth_NoWorkerWithBacklog(t *testing.T) {
	out := AdminWorkerHealthResponse{
		Health:         "green",
		RedisReachable: true,
		ServersActive:  0,
		Queue: AdminWorkerQueueStatus{
			Name:    "media",
			Pending: 5,
		},
	}
	health, alerts := finalizeAdminWorkerHealth(out)
	if health != "red" {
		t.Fatalf("expected red, got %s", health)
	}
	if len(alerts) == 0 {
		t.Fatalf("expected alerts, got none")
	}
}

func TestFinalizeAdminWorkerHealth_Healthy(t *testing.T) {
	out := AdminWorkerHealthResponse{
		Health:         "green",
		RedisReachable: true,
		ServersActive:  1,
		Queue: AdminWorkerQueueStatus{
			Name:    "media",
			Pending: 0,
		},
	}
	health, alerts := finalizeAdminWorkerHealth(out)
	if health != "green" {
		t.Fatalf("expected green, got %s", health)
	}
	if len(alerts) != 0 {
		t.Fatalf("expected no alert, got %+v", alerts)
	}
}
