package nativesymbols

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

const (
	Audience     = "planty-native-symbols"
	githubIssuer = "https://token.actions.githubusercontent.com"
)

var fullCommit = regexp.MustCompile(`^[a-f0-9]{40}$`)

type Authorizer interface {
	Authorize(context.Context, string) error
}

type GitHubAuthorizer struct {
	verifier     *oidc.IDTokenVerifier
	repository   string
	repositoryID string
}

func NewGitHubAuthorizer(ctx context.Context, repository, repositoryID string) *GitHubAuthorizer {
	client := &http.Client{Timeout: 15 * time.Second}
	ctx = oidc.ClientContext(ctx, client)
	keys := oidc.NewRemoteKeySet(ctx, githubIssuer+"/.well-known/jwks")
	return &GitHubAuthorizer{
		verifier: oidc.NewVerifier(githubIssuer, keys, &oidc.Config{
			ClientID:             Audience,
			SupportedSigningAlgs: []string{oidc.RS256},
		}),
		repository:   repository,
		repositoryID: repositoryID,
	}
}

func (a *GitHubAuthorizer) Authorize(ctx context.Context, raw string) error {
	if a.repository == "" || a.repositoryID == "" || len(raw) > 16384 {
		return errors.New("symbol publication is not authorized")
	}
	token, err := a.verifier.Verify(ctx, raw)
	if err != nil {
		return errors.New("invalid workload identity")
	}
	var claims struct {
		Repository   string `json:"repository"`
		RepositoryID string `json:"repository_id"`
		Ref          string `json:"ref"`
		WorkflowRef  string `json:"workflow_ref"`
		EventName    string `json:"event_name"`
		SHA          string `json:"sha"`
		NotBefore    int64  `json:"nbf"`
		IssuedAt     int64  `json:"iat"`
	}
	if token.Claims(&claims) != nil || claims.Repository != a.repository ||
		claims.RepositoryID != a.repositoryID || claims.Ref != "refs/heads/main" ||
		claims.WorkflowRef != a.repository+"/.github/workflows/release.yml@refs/heads/main" ||
		claims.EventName != "workflow_dispatch" || !fullCommit.MatchString(claims.SHA) {
		return errors.New("workload cannot publish symbols")
	}
	now := time.Now().Unix()
	if claims.IssuedAt <= 0 || claims.IssuedAt > now+60 || claims.NotBefore > now+60 || now-claims.IssuedAt > 600 {
		return errors.New("workload identity is not current")
	}
	return nil
}
