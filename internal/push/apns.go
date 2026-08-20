// Package push sends Planty's own iOS notifications through APNs. Home
// Assistant remains the sensor/weather bus; this package replaces only its
// role as the operator's notification transport.
package push

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

const (
	productionEndpoint = "https://api.push.apple.com"
	sandboxEndpoint    = "https://api.sandbox.push.apple.com"
)

// Sender implements the notification transport consumed by Home Assistant's
// compatibility adapter. It owns APNs authentication and fans one message out
// to every registered Planty device in the configured environment.
type Sender struct {
	store       *store.Store
	log         *slog.Logger
	keyID       string
	teamID      string
	bundleID    string
	environment string
	privateKey  *ecdsa.PrivateKey
	http        *http.Client

	mu        sync.Mutex
	jwt       string
	jwtIssued time.Time
}

// NewFromEnv returns nil until the four APNs credentials are configured. That
// deliberate nil lets the existing HA notify service remain a rollout fallback
// instead of silently dropping an alert before the first signed build lands.
func NewFromEnv(s *store.Store, log *slog.Logger) *Sender {
	keyID := strings.TrimSpace(os.Getenv("PLANTY_APNS_KEY_ID"))
	teamID := strings.TrimSpace(os.Getenv("PLANTY_APNS_TEAM_ID"))
	private := os.Getenv("PLANTY_APNS_PRIVATE_KEY")
	if keyID == "" || teamID == "" || strings.TrimSpace(private) == "" {
		return nil
	}
	key, err := parsePrivateKey([]byte(private))
	if err != nil {
		log.Error("APNs disabled: private key is invalid", "error", err)
		return nil
	}
	bundleID := strings.TrimSpace(os.Getenv("PLANTY_APNS_BUNDLE_ID"))
	if bundleID == "" {
		bundleID = "zone.stout.Planty"
	}
	environment := strings.ToLower(strings.TrimSpace(os.Getenv("PLANTY_APNS_ENVIRONMENT")))
	if environment == "" {
		environment = "production"
	}
	if environment != "production" && environment != "sandbox" {
		log.Error("APNs disabled: PLANTY_APNS_ENVIRONMENT must be production or sandbox")
		return nil
	}
	return &Sender{
		store: s, log: log, keyID: keyID, teamID: teamID, bundleID: bundleID,
		environment: environment, privateKey: key,
		http: &http.Client{Timeout: 20 * time.Second},
	}
}

// Send matches ha.NotificationTransport. The extra map is carried forward from
// the old HA notifier so existing jobs keep their collapse tags and urgency.
func (s *Sender) Send(ctx context.Context, title, body string, extra map[string]any) error {
	tokens, err := s.store.PushDeviceTokens(ctx, s.environment)
	if err != nil {
		return fmt.Errorf("list push devices: %w", err)
	}
	if len(tokens) == 0 {
		return errors.New("no Planty push devices are registered")
	}

	payload := map[string]any{
		"aps": map[string]any{
			"alert":     map[string]string{"title": title, "body": body},
			"sound":     "default",
			"thread-id": "planty",
		},
	}
	for key, value := range extra {
		if key != "data" {
			payload[key] = value
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var failures []error
	for _, token := range tokens {
		if err := s.sendOne(ctx, token, raw, collapseID(extra)); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) == len(tokens) {
		return errors.Join(failures...)
	}
	return nil
}

func (s *Sender) sendOne(ctx context.Context, token string, payload []byte, collapse string) error {
	auth, err := s.authorization()
	if err != nil {
		return err
	}
	endpoint := productionEndpoint
	if s.environment == "sandbox" {
		endpoint = sandboxEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		endpoint+"/3/device/"+token, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("authorization", "bearer "+auth)
	req.Header.Set("apns-topic", s.bundleID)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", "10")
	if collapse != "" {
		req.Header.Set("apns-collapse-id", collapse)
	}

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("APNs %s: %w", tokenSuffix(token), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	var reply struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(body, &reply)
	if resp.StatusCode == http.StatusGone || reply.Reason == "BadDeviceToken" || reply.Reason == "Unregistered" {
		if err := s.store.DeletePushDevice(ctx, s.environment, token); err != nil {
			s.log.Warn("could not prune APNs device token", "error", err)
		}
	}
	return fmt.Errorf("APNs %s: %s (%s)", tokenSuffix(token), resp.Status, reply.Reason)
}

func (s *Sender) authorization() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jwt != "" && time.Since(s.jwtIssued) < 50*time.Minute {
		return s.jwt, nil
	}
	now := time.Now().UTC()
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": s.keyID})
	claims, _ := json.Marshal(map[string]any{"iss": s.teamID, "iat": now.Unix()})
	unsigned := encode(header) + "." + encode(claims)
	digest := sha256.Sum256([]byte(unsigned))
	r, ss, err := ecdsa.Sign(rand.Reader, s.privateKey, digest[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	ss.FillBytes(sig[32:])
	s.jwt = unsigned + "." + encode(sig)
	s.jwtIssued = now
	return s.jwt, nil
}

func parsePrivateKey(raw []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("PEM block not found")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("APNs key is not ECDSA")
	}
	return key, nil
}

func encode(raw []byte) string { return base64.RawURLEncoding.EncodeToString(raw) }

func collapseID(extra map[string]any) string {
	data, ok := extra["data"].(map[string]any)
	if !ok {
		return ""
	}
	tag, _ := data["tag"].(string)
	return tag
}

func tokenSuffix(token string) string {
	if len(token) <= 8 {
		return token
	}
	return "…" + token[len(token)-8:]
}
