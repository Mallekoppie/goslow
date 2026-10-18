package platform

import (
	"context"
	"embed"
	"testing"
)

// Pointing the servers at an invalid address makes the blocking start helpers
// return their listen error immediately, which is what this test asserts.
func TestStartServerHelpersReportListenErrors(t *testing.T) {
	cfg := setupTestPlatformState(t)
	cfg.HTTP.Server.ListeningAddress = "not-a-valid-address"
	cfg.Grpc.Server.ListeningAddress = "not-a-valid-address"
	SetPlatformConfiguration(*cfg)

	if err := StartHttpServer(Routes{}); err == nil {
		t.Fatal("expected StartHttpServer to report the listen error")
	}

	if err := StartHttpServerWithContext(context.Background(), Routes{}); err == nil {
		t.Fatal("expected StartHttpServerWithContext to report the listen error")
	}

	var assets embed.FS
	if err := StartHttpServerWithWeb(Routes{}, assets); err == nil {
		t.Fatal("expected StartHttpServerWithWeb to report the listen error")
	}
	if err := StartHttpServerWithWeb(Routes{}, assets); err == nil {
		t.Fatal("expected StartHttpServerWithWeb to report the listen error")
	}

	if err := StartConnectServerWithContext(context.Background(), ConnectServerOptions{}); err == nil {
		t.Fatal("expected StartConnectServerWithContext to report the listen error")
	}
}
