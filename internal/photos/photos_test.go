package photos

import (
	"context"
	"net/url"
	"strings"
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
