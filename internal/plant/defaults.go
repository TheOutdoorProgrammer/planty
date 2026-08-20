package plant

// ApplyDefaults fills the values every sparse plant create means when the
// caller leaves them out. HTTP, agent, imports and future clients all call this
// one function so adding a new default cannot make two creation paths disagree.
func ApplyDefaults(p *Plant) {
	if p.Domain == "" {
		p.Domain = DomainHouseplant
	}
	if p.Status == "" {
		p.Status = StatusAlive
	}
	if p.Steward == "" {
		p.Steward = StewardSelf
	}
	if p.Accessibility == "" {
		p.Accessibility = AccessEasy
	}
	if p.WateringMethod == "" {
		p.WateringMethod = WateringHand
	}
}
