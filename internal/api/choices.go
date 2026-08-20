package api

import (
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

const recentChoiceLimit = 5

type managedChoice struct {
	Value      string     `json:"value"`
	Sources    []string   `json:"sources"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type managedChoiceList struct {
	Recent []managedChoice `json:"recent"`
	All    []managedChoice `json:"all"`
}

type managedChoicesResponse struct {
	Places       managedChoiceList `json:"places"`
	Owners       managedChoiceList `json:"owners"`
	PotMaterials managedChoiceList `json:"pot_materials"`
}

func (s *Server) listManagedChoices(w http.ResponseWriter, r *http.Request) {
	candidates, err := s.store.ChoiceCandidates(r.Context())
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}

	entities := []ha.Entity{}
	if s.homeAssistant != nil {
		discovered, err := s.homeAssistant.Entities(r.Context())
		if err != nil {
			// The catalog is still useful from Planty's own data. Home Assistant
			// enriches it; it must not make owner or pot-material choices vanish.
			s.log.Warn("managed choices omitted Home Assistant areas", "error", err)
		} else {
			entities = discovered
		}
	}

	w.Header().Set("Cache-Control", "no-store")
	s.ok(w, http.StatusOK, mergeManagedChoices(candidates, entities))
}

type choiceAccumulator struct {
	value   string
	sources map[string]struct{}
	usedAt  time.Time
}

func mergeManagedChoices(candidates []store.ChoiceCandidate, entities []ha.Entity) managedChoicesResponse {
	places := map[string]*choiceAccumulator{}
	owners := map[string]*choiceAccumulator{}
	materials := map[string]*choiceAccumulator{}

	for _, candidate := range candidates {
		target := places
		placeIdentity := true
		switch candidate.Kind {
		case store.ChoiceOwner:
			target, placeIdentity = owners, false
		case store.ChoicePotMaterial:
			target, placeIdentity = materials, false
		case store.ChoicePlace:
		default:
			continue
		}
		addManagedChoice(target, candidate.Value, candidate.Source, candidate.UsedAt, placeIdentity)
	}

	// `self` is a protocol default but also a valid household-owned label, so a
	// brand-new garden can choose it before any plant row exists.
	addManagedChoice(owners, "self", "default", time.Time{}, false)
	for _, entity := range entities {
		if strings.TrimSpace(entity.Area) != "" {
			addManagedChoice(places, entity.Area, "home_assistant", time.Time{}, true)
		}
	}

	return managedChoicesResponse{
		Places:       finalizeManagedChoices(places),
		Owners:       finalizeManagedChoices(owners),
		PotMaterials: finalizeManagedChoices(materials),
	}
}

func addManagedChoice(target map[string]*choiceAccumulator, value, source string, usedAt time.Time, placeIdentity bool) {
	value = strings.TrimSpace(value)
	key := textChoiceKey(value)
	if placeIdentity {
		key = managedPlaceKey(value)
	}
	if key == "" {
		return
	}
	entry, ok := target[key]
	if !ok {
		entry = &choiceAccumulator{value: value, sources: map[string]struct{}{}}
		target[key] = entry
	}
	if source != "" {
		entry.sources[source] = struct{}{}
	}
	// Prefer the spelling the user touched most recently over an older variant
	// or a Home Assistant-only spelling with no Planty timestamp.
	if usedAt.After(entry.usedAt) {
		entry.usedAt = usedAt
		entry.value = value
	}
}

func finalizeManagedChoices(values map[string]*choiceAccumulator) managedChoiceList {
	all := make([]managedChoice, 0, len(values))
	for _, entry := range values {
		sources := make([]string, 0, len(entry.sources))
		for source := range entry.sources {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		choice := managedChoice{Value: entry.value, Sources: sources}
		if !entry.usedAt.IsZero() {
			used := entry.usedAt
			choice.LastUsedAt = &used
		}
		all = append(all, choice)
	}
	sort.Slice(all, func(i, j int) bool {
		return strings.ToLower(all[i].Value) < strings.ToLower(all[j].Value)
	})

	recent := make([]managedChoice, 0, recentChoiceLimit)
	for _, choice := range all {
		if choice.LastUsedAt != nil {
			recent = append(recent, choice)
		}
	}
	sort.SliceStable(recent, func(i, j int) bool {
		return recent[i].LastUsedAt.After(*recent[j].LastUsedAt)
	})
	if len(recent) > recentChoiceLimit {
		recent = recent[:recentChoiceLimit]
	}
	return managedChoiceList{Recent: recent, All: all}
}

func textChoiceKey(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

// Place punctuation is presentation, not identity. This collapses Living Room,
// living-room and living_room without pretending unrelated names such as
// Lounge are synonyms. Other vocabularies keep punctuation because it can be
// meaningful in a person's name or a material label.
func managedPlaceKey(value string) string {
	fields := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.Join(fields, " ")
}
