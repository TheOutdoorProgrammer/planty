package judge

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Kind is how a provider is spoken to, and so which backend answers for it.
type Kind string

const (
	// KindClaude runs `claude -p`, spending the subscription rather than a
	// metered key. Choosing any Claude model means this.
	KindClaude Kind = "claude"

	// KindOpenAI is any endpoint speaking OpenAI chat completions.
	KindOpenAI Kind = "openai"
)

// Provider is one place models can be reached.
type Provider struct {
	ID      string `json:"id"`
	Kind    Kind   `json:"kind"`
	BaseURL string `json:"base_url,omitempty"`

	// APIKeyEnv names the variable holding the key, so a provider can be
	// configured in a ConfigMap while its credential stays in a Secret.
	APIKeyEnv string `json:"api_key_env,omitempty"`
}

// Key reads the provider's credential.
func (p Provider) Key() string {
	if p.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(p.APIKeyEnv)
}

func (p Provider) validate() error {
	if p.ID == "" {
		return fmt.Errorf("a provider needs an id")
	}
	switch p.Kind {
	case KindClaude:
	case KindOpenAI:
		if p.BaseURL == "" {
			return fmt.Errorf("provider %q needs a base_url", p.ID)
		}
		if p.APIKeyEnv == "" {
			return fmt.Errorf("provider %q needs an api_key_env", p.ID)
		}
	default:
		return fmt.Errorf("provider %q has unknown kind %q", p.ID, p.Kind)
	}
	return nil
}

// defaultProviders is what Planty knows about with no configuration: the
// Claude subscription it has always used, and the OpenCode Go subscription,
// which is dormant until its key is present.
var defaultProviders = []Provider{
	{ID: "claude", Kind: KindClaude},
	{ID: "opencode-go", Kind: KindOpenAI,
		BaseURL: "https://opencode.ai/zen/go/v1", APIKeyEnv: "OPENCODE_API_KEY"},
}

// Providers is every provider that can actually be reached right now. An
// OpenAI provider whose key is absent is dropped rather than offered, because
// a model in the picker that 401s is worse than one that was never listed.
func Providers() (map[string]Provider, error) {
	declared := defaultProviders
	if raw := strings.TrimSpace(os.Getenv("PLANTY_PROVIDERS")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &declared); err != nil {
			return nil, fmt.Errorf("PLANTY_PROVIDERS is not valid JSON: %w", err)
		}
	}

	out := map[string]Provider{}
	for _, p := range declared {
		if err := p.validate(); err != nil {
			return nil, err
		}
		if p.Kind == KindOpenAI && p.Key() == "" {
			continue
		}
		out[p.ID] = p
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no provider is reachable")
	}
	return out, nil
}
