package push

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendOneUsesTheConfiguredEnvironmentTopicAndToken(t *testing.T) {
	var gotPath, gotTopic, gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotTopic = r.Header.Get("apns-topic")
		gotAuth = r.Header.Get("authorization")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sender := &Sender{
		log: slog.Default(), keyID: "KEY", teamID: "TEAM", bundleID: "zone.stout.Planty",
		environment: "sandbox", privateKey: key, http: server.Client(), endpoint: server.URL,
	}
	if err := sender.sendOne(context.Background(), "aabbcc", []byte(`{"aps":{}}`), "test"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/3/device/aabbcc" || gotTopic != "zone.stout.Planty" {
		t.Fatalf("path/topic = %q / %q", gotPath, gotTopic)
	}
	if !strings.HasPrefix(gotAuth, "bearer ") || gotBody != `{"aps":{}}` {
		t.Fatalf("authorization or payload missing: %q / %q", gotAuth, gotBody)
	}
}
