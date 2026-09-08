package nativesymbols

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func TestWorkloadIdentityRequiresSignedReleaseClaims(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
	if err != nil {
		t.Fatal(err)
	}
	auth := &GitHubAuthorizer{
		verifier:   oidc.NewVerifier(githubIssuer, &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, &oidc.Config{ClientID: Audience}),
		repository: "owner/app", repositoryID: "123",
	}
	for _, tc := range []struct {
		name, claim string
		value       any
		allowed     bool
	}{
		{name: "release", allowed: true},
		{name: "wrong audience", claim: "aud", value: "fledge"},
		{name: "wrong issuer", claim: "iss", value: "https://attacker.invalid"},
		{name: "expired", claim: "exp", value: time.Now().Add(-time.Minute).Unix()},
		{name: "recreated repository", claim: "repository_id", value: "456"},
		{name: "other repository", claim: "repository", value: "attacker/app"},
		{name: "branch", claim: "ref", value: "refs/heads/feature"},
		{name: "different workflow", claim: "workflow_ref", value: "owner/app/.github/workflows/ci.yml@refs/heads/main"},
		{name: "pull request", claim: "event_name", value: "pull_request_target"},
		{name: "missing commit", claim: "sha", value: ""},
		{name: "not yet valid", claim: "nbf", value: time.Now().Add(time.Hour).Unix()},
		{name: "stale issuance", claim: "iat", value: time.Now().Add(-time.Hour).Unix()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := map[string]any{
				"iss": githubIssuer, "aud": Audience, "sub": "repo:owner/app:ref:refs/heads/main",
				"iat": time.Now().Unix(), "nbf": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(),
				"repository": "owner/app", "repository_id": "123", "ref": "refs/heads/main",
				"workflow_ref": "owner/app/.github/workflows/release.yml@refs/heads/main", "event_name": "workflow_dispatch",
				"sha": "0123456789012345678901234567890123456789",
			}
			if tc.claim != "" {
				claims[tc.claim] = tc.value
			}
			raw, err := jwt.Signed(signer).Claims(claims).Serialize()
			if err != nil {
				t.Fatal(err)
			}
			err = auth.Authorize(context.Background(), raw)
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v, error=%v", tc.allowed, err)
			}
		})
	}
	if auth.Authorize(context.Background(), "a mobile API token") == nil {
		t.Fatal("accepted unsigned mobile token")
	}
}
