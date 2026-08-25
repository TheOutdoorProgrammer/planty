package photos

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

const internalEndpoint = "minio.example.svc.cluster.local:9000"

// presign signs one link as the deployment would, against the public host.
func presign(t *testing.T, public string) *url.URL {
	t.Helper()

	cfg := Config{
		Endpoint:       internalEndpoint,
		PublicEndpoint: public,
		AccessKey:      "planty",
		SecretKey:      "secret",
		Bucket:         "planty",
		PublicSSL:      true,
	}
	signer, err := signingClient(cfg, nil)
	if err != nil {
		t.Fatalf("build signer: %v", err)
	}
	// Region is fixed in advance precisely so this needs no bucket to talk to.
	link, err := signer.PresignedGetObject(context.Background(), cfg.Bucket,
		"golden-pothos/2026/08/18/photo.jpg", 30*time.Minute, nil)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}
	return link
}

// The bug this exists to stop: links signed against the cluster DNS name reach
// the pod fine and are unresolvable to the phone that has to render them.
func TestLinksAreSignedForWhoeverFollowsThem(t *testing.T) {
	link := presign(t, "s3.example.com")

	if link.Host != "s3.example.com" {
		t.Errorf("signed for %q, want the public host", link.Host)
	}
	if link.Scheme != "https" {
		t.Errorf("scheme is %q, want https", link.Scheme)
	}
	if !link.Query().Has("X-Amz-Signature") {
		t.Error("link is not signed")
	}
}

// A signature covers the host, so a link signed for one name and served under
// another is rejected. Swapping the host afterwards is not an available fix.
func TestTheSignatureCoversTheHost(t *testing.T) {
	internal := presign(t, "minio.other.svc.cluster.local:9000")
	public := presign(t, "s3.example.com")

	if !strings.Contains(internal.Query().Get("X-Amz-SignedHeaders"), "host") {
		t.Fatal("host is not in the signed headers, so this test proves nothing")
	}
	if internal.Query().Get("X-Amz-Signature") == public.Query().Get("X-Amz-Signature") {
		t.Error("the two hosts signed identically, so the host is not covered")
	}
}

// Without a public endpoint nothing changes, so a deployment that never needed
// this keeps working untouched.
func TestNoPublicEndpointSignsWithTheReachingClient(t *testing.T) {
	reaching, err := minio.New(internalEndpoint, &minio.Options{})
	if err != nil {
		t.Fatalf("build reaching client: %v", err)
	}
	cfg := Config{Endpoint: internalEndpoint, Bucket: "planty"}

	for _, public := range []string{"", internalEndpoint} {
		cfg.PublicEndpoint = public

		signer, err := signingClient(cfg, reaching)
		if err != nil {
			t.Fatalf("public %q: build signer: %v", public, err)
		}
		if signer != reaching {
			t.Errorf("public %q: built a second client instead of reusing the reaching one", public)
		}
	}
}

func TestKeysDoNotCollideWithinASecond(t *testing.T) {
	takenAt := time.Date(2026, 8, 18, 23, 26, 9, 0, time.UTC)

	first := Key("golden-pothos", takenAt, ".jpg")
	second := Key("golden-pothos", takenAt, ".jpg")

	if first == second {
		t.Errorf("two photos of one plant in the same second share a key: %s", first)
	}
	if !strings.HasPrefix(first, "golden-pothos/2026/08/18/") {
		t.Errorf("key %q is not laid out as a timeline", first)
	}
}

// This is the shared-node restart in miniature: Planty starts first, the
// object store refuses the first connection, and the same Manager becomes
// ready later without being replaced.
func TestManagerRecoversWhenStorageStartsAfterPlanty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	states := make(chan State, 2)
	var attempts atomic.Int32
	m := &Manager{
		state: StateStarting,
		open: func(context.Context, Config) (*Store, error) {
			if attempts.Add(1) == 1 {
				return nil, errors.New("connection refused")
			}
			return &Store{}, nil
		},
		retryMin: time.Millisecond,
		retryMax: 2 * time.Millisecond,
		changed:  func(state State, _ error) { states <- state },
	}
	go m.run(ctx)

	for _, want := range []State{StateUnavailable, StateReady} {
		select {
		case got := <-states:
			if got != want {
				t.Fatalf("state = %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("never reached %q", want)
		}
	}
	if store, err := m.store(); err != nil || store == nil {
		t.Fatalf("manager did not expose the recovered store: %v", err)
	}
}
