package job

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const CommonCareWindow = 30 * time.Minute

type IncidentRadar struct {
	Store *store.Store
}

func (r IncidentRadar) Run(ctx context.Context, runID uuid.UUID) (incidents []plant.GardenIncident, runErr error) {
	ctx, span := otel.Tracer("planty/incidents").Start(ctx, "incident.detect")
	defer func() {
		if runErr != nil {
			span.RecordError(runErr)
			span.SetStatus(codes.Error, "incident detection failed")
		}
		span.End()
	}()

	run, signals, err := r.Store.IncidentSignalsForRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.Int("incident.signal_count", len(signals)))
	if len(signals) == 0 {
		return []plant.GardenIncident{}, nil
	}

	candidates := []plant.IncidentCandidate{}
	areas := groupIncidentSignals(signals, func(signal store.IncidentSignal) string { return signal.Plant.HAArea })
	locations := groupIncidentSignals(signals, func(signal store.IncidentSignal) string { return signal.Plant.Location })
	for factor, groups := range map[plant.IncidentFactor]map[string][]store.IncidentSignal{
		plant.FactorHAArea: areas, plant.FactorLocation: locations,
	} {
		for ref, members := range groups {
			if len(members) < 2 {
				continue
			}
			label := "location"
			if factor == plant.FactorHAArea {
				label = "Home Assistant area"
			}
			candidates = append(candidates, incidentCandidate(runID, factor, ref, members,
				fmt.Sprintf("Dire shared condition: %d plants in %s %q have new urgent actions from the same complete run.", len(members), label, ref), nil, nil))
		}
	}

	plantIDs := make([]uuid.UUID, 0, len(signals))
	areaNames := make([]string, 0, len(areas))
	for _, signal := range signals {
		plantIDs = append(plantIDs, signal.Plant.ID)
	}
	for area := range areas {
		areaNames = append(areaNames, area)
	}
	care, err := r.Store.IncidentCareSignals(ctx, run, plantIDs)
	if err != nil {
		return nil, err
	}
	candidates = append(candidates, commonCareCandidates(runID, signals, care)...)

	failures, err := r.Store.IncidentEnvironmentFailures(ctx, run, areaNames)
	if err != nil {
		return nil, err
	}
	failuresByArea := map[string][]store.IncidentEnvironmentFailure{}
	for _, failure := range failures {
		failuresByArea[normalizedFactor(failure.Area)] = append(failuresByArea[normalizedFactor(failure.Area)], failure)
	}
	for area, members := range areas {
		if len(members) != 1 || len(failuresByArea[area]) == 0 {
			continue
		}
		ids := make([]uuid.UUID, 0, len(failuresByArea[area]))
		for _, failure := range failuresByArea[area] {
			ids = append(ids, failure.SensorLinkID)
		}
		candidates = append(candidates, incidentCandidate(runID, plant.FactorEnvironmentFailure, area,
			members, fmt.Sprintf("Dire condition with missing evidence: a plant in Home Assistant area %q has a new urgent action while independent environmental sensors have no reading for this run.", area), nil, ids))
	}

	actuatorFailures, err := r.Store.IncidentActuatorFailures(ctx, run, plantIDs)
	if err != nil {
		return nil, err
	}
	signalsByPlant := make(map[uuid.UUID]store.IncidentSignal, len(signals))
	for _, signal := range signals {
		signalsByPlant[signal.Plant.ID] = signal
	}
	type actuatorGroup struct {
		members map[uuid.UUID]store.IncidentSignal
		events  map[uuid.UUID]bool
	}
	byActuator := map[uuid.UUID]*actuatorGroup{}
	for _, failure := range actuatorFailures {
		signal, affected := signalsByPlant[failure.PlantID]
		if !affected {
			continue
		}
		group := byActuator[failure.ActuatorID]
		if group == nil {
			group = &actuatorGroup{members: map[uuid.UUID]store.IncidentSignal{}, events: map[uuid.UUID]bool{}}
			byActuator[failure.ActuatorID] = group
		}
		group.members[signal.Plant.ID] = signal
		group.events[failure.EventID] = true
	}
	for actuatorID, group := range byActuator {
		members := make([]store.IncidentSignal, 0, len(group.members))
		for _, signal := range group.members {
			members = append(members, signal)
		}
		eventIDs := make([]uuid.UUID, 0, len(group.events))
		for eventID := range group.events {
			eventIDs = append(eventIDs, eventID)
		}
		candidate := incidentCandidate(runID, plant.FactorActuatorFailure, actuatorID.String(), members,
			fmt.Sprintf("Dire condition with failed automation: %d urgently affected plants are assigned to an actuator with a failed command near this complete run.", len(members)), nil, nil)
		candidate.Evidence.ActuatorEventIDs = eventIDs
		candidates = append(candidates, candidate)
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Factor == candidates[j].Factor {
			return candidates[i].FactorRef < candidates[j].FactorRef
		}
		return candidates[i].Factor < candidates[j].Factor
	})
	incidents = make([]plant.GardenIncident, 0, len(candidates))
	createdCount := 0
	for _, candidate := range candidates {
		incident, created, err := r.Store.UpsertIncidentCandidate(ctx, candidate)
		if err != nil {
			return nil, err
		}
		if created {
			createdCount++
		}
		incidents = append(incidents, incident)
	}
	span.SetAttributes(
		attribute.Int("incident.candidate_count", len(candidates)),
		attribute.Int("incident.created_count", createdCount),
		attribute.Int("incident.refreshed_count", len(candidates)-createdCount),
	)
	return incidents, nil
}

func groupIncidentSignals(signals []store.IncidentSignal, key func(store.IncidentSignal) string) map[string][]store.IncidentSignal {
	groups := map[string][]store.IncidentSignal{}
	for _, signal := range signals {
		ref := normalizedFactor(key(signal))
		if ref != "" {
			groups[ref] = append(groups[ref], signal)
		}
	}
	return groups
}

func normalizedFactor(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func incidentCandidate(runID uuid.UUID, factor plant.IncidentFactor, ref string, signals []store.IncidentSignal, summary string, observationIDs, sensorIDs []uuid.UUID) plant.IncidentCandidate {
	verdictIDs := make([]uuid.UUID, 0, len(signals))
	members := make([]plant.IncidentPlant, 0, len(signals))
	for _, signal := range signals {
		verdictIDs = append(verdictIDs, signal.VerdictID)
		members = append(members, plant.IncidentPlant{
			Plant: signal.Plant, Role: "affected", VerdictID: signal.VerdictID,
			Action: signal.Action, Confidence: signal.Confidence,
		})
	}
	confidence := min(0.9, 0.55+0.1*float64(len(signals)))
	if len(signals) == 1 {
		confidence = 0.55
	}
	return plant.IncidentCandidate{
		Factor: factor, FactorRef: ref, Summary: summary, Reason: incidentReason(summary, signals),
		Confidence: confidence, Plants: members,
		Evidence: plant.IncidentEvidence{
			RunID: runID, VerdictIDs: verdictIDs, ObservationIDs: observationIDs,
			SensorLinkIDs: sensorIDs, Note: "Deterministic correlation only; this does not establish causation.",
		},
	}
}

func incidentReason(summary string, signals []store.IncidentSignal) string {
	findings := make([]string, 0, len(signals))
	for _, signal := range signals {
		reasoning := strings.TrimSpace(signal.Reasoning)
		if reasoning == "" {
			continue
		}
		findings = append(findings, fmt.Sprintf("%s was marked %s. Agent reason: %s",
			signal.Plant.CommonName, signal.Action, reasoning))
	}
	sort.Strings(findings)
	if len(findings) == 0 {
		return summary
	}
	return summary + " " + strings.Join(findings, " ")
}

func commonCareCandidates(runID uuid.UUID, signals []store.IncidentSignal, care []store.IncidentCareSignal) []plant.IncidentCandidate {
	byPlant := map[uuid.UUID]store.IncidentSignal{}
	for _, signal := range signals {
		byPlant[signal.Plant.ID] = signal
	}
	byKind := map[plant.ObservationKind][]store.IncidentCareSignal{}
	for _, signal := range care {
		byKind[signal.Kind] = append(byKind[signal.Kind], signal)
	}
	out := []plant.IncidentCandidate{}
	for kind, events := range byKind {
		for start := 0; start < len(events); {
			end := start
			unique := map[uuid.UUID]store.IncidentSignal{}
			ids := []uuid.UUID{}
			for end < len(events) && events[end].OccurredAt.Sub(events[start].OccurredAt) <= CommonCareWindow {
				if signal, ok := byPlant[events[end].PlantID]; ok {
					unique[events[end].PlantID] = signal
					ids = append(ids, events[end].ID)
				}
				end++
			}
			if len(unique) >= 2 {
				members := make([]store.IncidentSignal, 0, len(unique))
				for _, signal := range unique {
					members = append(members, signal)
				}
				sort.Slice(members, func(i, j int) bool { return members[i].Plant.ID.String() < members[j].Plant.ID.String() })
				ref := fmt.Sprintf("%s:%s", kind, events[start].OccurredAt.UTC().Truncate(time.Minute).Format(time.RFC3339))
				out = append(out, incidentCandidate(runID, plant.FactorCommonCare, ref, members,
					fmt.Sprintf("Dire shared condition: %d plants received %s care within %s before new urgent actions.", len(members), kind, CommonCareWindow), ids, nil))
			}
			start = max(end, start+1)
		}
	}
	return out
}
