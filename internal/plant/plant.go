// Package plant holds the domain types shared by the store, the HTTP API, the
// Dusk plugin and the iOS app. The reasoning behind the shape is in
// docs/DATA-MODEL.md.
package plant

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Domain decides which questions are worth asking: a houseplant is judged on
// staying alive, a tomato on bearing fruit.
type Domain string

const (
	DomainHouseplant    Domain = "houseplant"
	DomainEdibleIndoor  Domain = "edible_indoor"
	DomainEdibleOutdoor Domain = "edible_outdoor"
)

// Status marks dead as a state rather than a deletion, because what was done to
// a plant that died is the most useful record for not repeating it.
type Status string

const (
	StatusAlive      Status = "alive"
	StatusStruggling Status = "struggling"
	StatusDormant    Status = "dormant"
	StatusDead       Status = "dead"
	StatusGone       Status = "gone"
)

// ValidateArchive rejects states that contradict an archived record.
func (s Status) ValidateArchive() error {
	if s != StatusDead && s != StatusGone {
		return invalid("archiving records dead or gone, not %q", s)
	}
	return nil
}

// WateringMethod decides whether Planty can act, or only report.
type WateringMethod string

const (
	WateringLetPot WateringMethod = "letpot"
	WateringHand   WateringMethod = "hand"
)

// Accessibility drives sensor priority; unreachable plants stop being checked.
type Accessibility string

const (
	AccessEasy    Accessibility = "easy"
	AccessAwkward Accessibility = "awkward"
	AccessHard    Accessibility = "hard"
)

// LightExposure is what the plant receives. What it wants lives in CareProfile.
type LightExposure string

const (
	LightDirect         LightExposure = "direct"
	LightBrightIndirect LightExposure = "bright_indirect"
	LightMedium         LightExposure = "medium"
	LightLow            LightExposure = "low"
)

// StewardSelf marks a plant as the operator's own; anything else is a name.
const StewardSelf = "self"

// ErrInvalid marks a failure the caller caused, so a handler can answer 400
// without every call site guessing. A server fault must never report as one.
var ErrInvalid = errors.New("invalid")

// invalid wraps a rejection so it carries whose fault it is.
func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Plant is one growing thing in one place. Fields automations query are typed;
// descriptive care knowledge goes in CareProfile.
type Plant struct {
	ID   uuid.UUID `json:"id"`
	Slug string    `json:"slug"`

	CommonName    string `json:"common_name"`
	BotanicalName string `json:"botanical_name,omitempty"`
	Variety       string `json:"variety,omitempty"`

	Domain  Domain `json:"domain"`
	Steward string `json:"steward"`
	Status  Status `json:"status"`

	Location string `json:"location"`
	HAArea   string `json:"ha_area,omitempty"`

	Accessibility  Accessibility  `json:"accessibility"`
	WateringMethod WateringMethod `json:"watering_method"`
	LetPotDripper  *int           `json:"letpot_dripper,omitempty"`

	PotSizeIn     *float64      `json:"pot_size_in,omitempty"`
	PotMaterial   string        `json:"pot_material,omitempty"`
	HasDrainage   *bool         `json:"has_drainage,omitempty"`
	SoilMix       string        `json:"soil_mix,omitempty"`
	LightExposure LightExposure `json:"light_exposure,omitempty"`

	// Typed and indexed rather than left in CareProfile because the cold-snap
	// automation queries it nightly, where a JSON key typo would fail silently.
	MinTempF *float64 `json:"min_temp_f,omitempty"`

	CareProfile CareProfile `json:"care_profile"`
	Toxicity    Toxicity    `json:"toxicity"`

	AcquiredAt *time.Time `json:"acquired_at,omitempty"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`

	// Set when carried indoors ahead of a cold night, so the system can also
	// say when to put it back out.
	ShelteredAt *time.Time `json:"sheltered_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsFriends reports whether somebody else owns this plant.
func (p Plant) IsFriends() bool { return p.Steward != "" && p.Steward != StewardSelf }

// NeedsChasing reports whether a due action depends on a human remembering.
func (p Plant) NeedsChasing() bool { return p.WateringMethod == WateringHand }

// Risk ranks likelihood of neglect, not current thirst.
func (p Plant) Risk() int {
	risk := 0
	if p.NeedsChasing() {
		risk += 2
	}
	switch p.Accessibility {
	case AccessHard:
		risk += 2
	case AccessAwkward:
		risk++
	}
	if p.IsFriends() {
		risk += 2
	}
	if p.Status == StatusStruggling {
		risk++
	}
	return risk
}

// CareProfile is domain-shaped descriptive knowledge. Every field is optional,
// since most are unknown when a plant is first written down.
type CareProfile struct {
	Notes      string        `json:"notes,omitempty"`
	OwnerSays  string        `json:"owner_says,omitempty"`
	Quirks     string        `json:"quirks,omitempty"`
	WantsLight LightExposure `json:"wants_light,omitempty"`

	MoistureLow  *float64 `json:"moisture_low,omitempty"`
	MoistureHigh *float64 `json:"moisture_high,omitempty"`
	HumidityPref string   `json:"humidity_pref,omitempty"`
	Dormancy     string   `json:"dormancy,omitempty"`

	DaysToMaturity  *int       `json:"days_to_maturity,omitempty"`
	SownAt          *time.Time `json:"sown_at,omitempty"`
	TransplantedAt  *time.Time `json:"transplanted_at,omitempty"`
	FeedSchedule    string     `json:"feed_schedule,omitempty"`
	ExpectedHarvest *time.Time `json:"expected_harvest,omitempty"`

	// Indoors a tomato flowers and sets no fruit without something shaking the
	// flowers, and the plant looks healthy the whole time it yields nothing.
	NeedsPollination *bool `json:"needs_pollination,omitempty"`

	HardinessZone      string     `json:"hardiness_zone,omitempty"`
	FrostSensitive     *bool      `json:"frost_sensitive,omitempty"`
	SuccessionDays     *int       `json:"succession_days,omitempty"`
	LastFrostExpected  *time.Time `json:"last_frost_expected,omitempty"`
	FirstFrostExpected *time.Time `json:"first_frost_expected,omitempty"`
}

// Valid reports whether the enum fields hold values the store accepts, so a
// rejection reads the same whether the API or the Dusk plugin asked.
func (p Plant) Valid() error {
	switch p.Domain {
	case DomainHouseplant, DomainEdibleIndoor, DomainEdibleOutdoor:
	default:
		return invalid("unknown domain %q", p.Domain)
	}
	switch p.Status {
	case StatusAlive, StatusStruggling, StatusDormant, StatusDead, StatusGone:
	default:
		return invalid("unknown status %q", p.Status)
	}
	switch p.WateringMethod {
	case WateringLetPot, WateringHand:
	default:
		return invalid("unknown watering method %q", p.WateringMethod)
	}
	switch p.Accessibility {
	case AccessEasy, AccessAwkward, AccessHard:
	default:
		return invalid("unknown accessibility %q", p.Accessibility)
	}
	switch p.LightExposure {
	case "", LightDirect, LightBrightIndirect, LightMedium, LightLow:
	default:
		return invalid("unknown light exposure %q", p.LightExposure)
	}
	if p.WateringMethod == WateringHand && p.LetPotDripper != nil {
		return invalid("hand-watered plant cannot hold a LetPot dripper number")
	}
	if strings.TrimSpace(p.CommonName) == "" {
		return invalid("common_name is required")
	}
	return nil
}
