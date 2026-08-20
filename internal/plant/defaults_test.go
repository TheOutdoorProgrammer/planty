package plant

import "testing"

func TestApplyDefaultsFillsOnlyMissingCreateFields(t *testing.T) {
	p := Plant{CommonName: "Fern", Domain: DomainEdibleIndoor, Steward: "Maya"}
	ApplyDefaults(&p)

	if p.Domain != DomainEdibleIndoor {
		t.Errorf("domain = %q, want preserved edible_indoor", p.Domain)
	}
	if p.Steward != "Maya" {
		t.Errorf("steward = %q, want preserved Maya", p.Steward)
	}
	if p.Status != StatusAlive {
		t.Errorf("status = %q, want alive", p.Status)
	}
	if p.Accessibility != AccessEasy {
		t.Errorf("accessibility = %q, want easy", p.Accessibility)
	}
	if p.WateringMethod != WateringHand {
		t.Errorf("watering = %q, want hand", p.WateringMethod)
	}
}

func TestApplyDefaultsMatchesSparseCreateContract(t *testing.T) {
	p := Plant{CommonName: "Fern"}
	ApplyDefaults(&p)

	if p.Domain != DomainHouseplant || p.Status != StatusAlive || p.Steward != StewardSelf ||
		p.Accessibility != AccessEasy || p.WateringMethod != WateringHand {
		t.Fatalf("unexpected defaults: %#v", p)
	}
}
