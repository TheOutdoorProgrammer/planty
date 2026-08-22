package judge

import (
	"strings"
	"testing"
)

// The whole point of the capability table: a model that cannot see is refused
// for the jobs that show it a photograph, rather than failing in a garden
// centre when somebody points a phone at a plant.
func TestABlindModelIsRefusedTheJobsThatShowPhotographs(t *testing.T) {
	blind, ok := Lookup("opencode-go", "gpt-5.6-luna")
	if !ok {
		t.Fatal("gpt-5.6-luna is missing from the catalogue")
	}

	for _, job := range []Job{JobIdentify, JobConsult, JobAsk} {
		if err := blind.CanDo(job); err == nil {
			t.Errorf("%s was allowed on %s despite not reading images", blind.Ref(), job)
		} else if !strings.Contains(err.Error(), "images") {
			t.Errorf("the refusal for %s does not say why: %v", job, err)
		}
	}
	for _, job := range []Job{JobAssess, JobPostmortem, JobOwnerUpdate} {
		if err := blind.CanDo(job); err != nil {
			t.Errorf("%s was refused text-only job %s: %v", blind.Ref(), job, err)
		}
	}
}

func TestASeeingModelTakesEveryJob(t *testing.T) {
	seeing, ok := Lookup("opencode-go", "qwen3.8-max")
	if !ok {
		t.Fatal("qwen3.8-max is missing from the catalogue")
	}
	for _, job := range Jobs {
		if err := seeing.CanDo(job); err != nil {
			t.Errorf("%s was refused %s: %v", seeing.Ref(), job, err)
		}
	}
}

func TestAnUnknownJobIsRefusedRatherThanAllowed(t *testing.T) {
	m, _ := Lookup("claude", "claude-opus-5")
	if err := m.CanDo(Job("invented")); err == nil {
		t.Error("an unknown job was allowed through")
	}
}

// The app lists by capability, not alphabetically, so the order the catalogue
// returns is part of the contract.
func TestTheCatalogueIsSmartestFirstAndOnlyWhatIsReachable(t *testing.T) {
	both := Catalog(map[string]Provider{
		"claude":      {ID: "claude", Kind: KindClaude},
		"opencode-go": {ID: "opencode-go", Kind: KindOpenAI},
	})
	if len(both) < 2 {
		t.Fatalf("the catalogue came back nearly empty: %d entries", len(both))
	}
	for i := 1; i < len(both); i++ {
		if both[i-1].Rank > both[i].Rank {
			t.Fatalf("%s (rank %d) was listed after %s (rank %d)",
				both[i-1].Ref(), both[i-1].Rank, both[i].Ref(), both[i].Rank)
		}
	}
	if both[0].Provider != "claude" {
		t.Errorf("the smartest model is not first; got %s", both[0].Ref())
	}

	only := Catalog(map[string]Provider{"claude": {ID: "claude", Kind: KindClaude}})
	for _, m := range only {
		if m.Provider != "claude" {
			t.Errorf("%s was offered though its provider is not configured", m.Ref())
		}
	}
}

// A key-less OpenAI provider is dropped, because a model in the picker that
// 401s is worse than one that was never listed.
func TestAProviderWithNoKeyIsNotOffered(t *testing.T) {
	t.Setenv("PLANTY_PROVIDERS", "")
	t.Setenv("OPENCODE_API_KEY", "")

	without, err := Providers()
	if err != nil {
		t.Fatalf("Providers: %v", err)
	}
	if _, ok := without["opencode-go"]; ok {
		t.Error("opencode-go was offered with no key set")
	}
	if _, ok := without["claude"]; !ok {
		t.Error("claude was dropped; it needs no key")
	}

	t.Setenv("OPENCODE_API_KEY", "sk-test")
	with, err := Providers()
	if err != nil {
		t.Fatalf("Providers: %v", err)
	}
	if _, ok := with["opencode-go"]; !ok {
		t.Error("opencode-go was still absent with a key set")
	}
}

func TestAMalformedProviderIsRefusedLoudly(t *testing.T) {
	t.Setenv("PLANTY_PROVIDERS", `[{"id":"x","kind":"openai"}]`)
	if _, err := Providers(); err == nil {
		t.Error("an openai provider with no base_url was accepted")
	}

	t.Setenv("PLANTY_PROVIDERS", `not json`)
	if _, err := Providers(); err == nil {
		t.Error("malformed PLANTY_PROVIDERS was accepted")
	}
}
