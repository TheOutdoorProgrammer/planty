package store

import (
	"testing"

	"github.com/google/uuid"
)

func TestPushTokenRefreshReplacesOneInstallationRegistration(t *testing.T) {
	s, ctx := testStore(t)
	installation := uuid.New()

	first, err := s.UpsertPushDevice(ctx, PushDevice{
		Token: "aaaa", Environment: "production", InstallationID: installation,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.UpsertPushDevice(ctx, PushDevice{
		Token: "bbbb", Environment: "production", InstallationID: installation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.AcceptedAt.Before(first.AcceptedAt) {
		t.Fatalf("refresh acceptance moved backward: %v then %v", first.AcceptedAt, second.AcceptedAt)
	}
	registered, err := s.PushDeviceForInstallation(ctx, "production", installation)
	if err != nil {
		t.Fatal(err)
	}
	if registered.Token != "bbbb" {
		t.Fatalf("stale token survived refresh: %q", registered.Token)
	}
	tokens, err := s.PushDeviceTokens(ctx, "production")
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 || tokens[0] != "bbbb" {
		t.Fatalf("tokens after refresh = %v", tokens)
	}
}
