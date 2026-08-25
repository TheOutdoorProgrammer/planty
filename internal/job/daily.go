package job

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/photos"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

const maxAssessmentPhotoBytes = 12 << 20

// Daily produces one verdict per plant, then a single digest notification.
type Daily struct {
	Store         *store.Store
	Judge         *judge.Judge
	Log           *slog.Logger
	Notifications Notifier
	Photos        photos.Storage
}

// Run judges every live plant and notifies only if something needs doing or the
// check was incomplete. The garden-wide run is persisted before model work so a
// process interruption cannot leave yesterday's all-clear looking current.
func (d Daily) Run(ctx context.Context) error {
	plants, err := d.Store.ListPlants(ctx, store.PlantFilter{Status: plant.StatusAlive})
	if err != nil {
		return fmt.Errorf("list plants: %w", err)
	}

	run, err := d.Store.StartJudgmentRun(ctx, len(plants))
	if err != nil {
		return fmt.Errorf("start judgment run: %w", err)
	}

	// No key is a Planty running without its opinion, not a broken cold-watch or
	// watering system. Still record that this daily check covered zero plants so
	// Today cannot inherit a false all-clear from an older successful run.
	if d.Judge == nil {
		for _, p := range plants {
			if err := d.Store.RecordJudgmentPlantResult(ctx, run.ID, store.JudgmentResultInput{
				PlantID: p.ID, Attempts: 1, FinalError: "no model backend is configured",
			}); err != nil {
				return fmt.Errorf("record unavailable judgment: %w", err)
			}
		}
		if err := d.Store.CompleteJudgmentRun(ctx, run.ID); err != nil {
			return fmt.Errorf("complete unavailable judgment run: %w", err)
		}
		d.Log.Warn("no judgment: no model backend is configured",
			"plants", len(plants), "still_running", "cold watch, watering, escalation")
		return nil
	}

	var failed int
	for _, p := range plants {
		result, err := d.judgeOne(ctx, run.ID, p)
		if err != nil {
			// One plant failing must not silence the rest of the digest.
			d.Log.Error("judgment failed", "plant", p.Slug, "error", err)
			failed++
		}
		if err := d.Store.RecordJudgmentPlantResult(ctx, run.ID,
			judgmentResult(p.ID, result, err)); err != nil {
			return fmt.Errorf("record judgment result for %s: %w", p.Slug, err)
		}
	}
	if err := d.Store.CompleteJudgmentRun(ctx, run.ID); err != nil {
		return fmt.Errorf("complete judgment run: %w", err)
	}
	d.detectIncidents(ctx, run.ID)

	// A death is worth understanding, and nobody remembers to ask for it.
	if written, err := (Postmortem{Store: d.Store, Judge: d.Judge, Log: d.Log}).
		Sweep(ctx); err != nil {
		d.Log.Error("postmortem sweep failed", "error", err)
	} else if written > 0 {
		d.Log.Info("postmortems written", "count", written)
	}

	digest, err := d.Store.ReliableDigest(ctx, plant.StaleAfter)
	if err != nil {
		return fmt.Errorf("digest: %w", err)
	}

	if failed == len(plants) && len(plants) > 0 {
		return fmt.Errorf("every plant failed judgment (%d)", failed)
	}
	if digest.AllClear() {
		d.Log.Info("nothing to do", "checked", digest.Checked)
		return nil
	}
	return d.notify(ctx, digest)
}

// AssessPlant runs the same evidence gathering and judgment as the daily job
// for one plant without sending a garden-wide notification.
func (d Daily) AssessPlant(ctx context.Context, p plant.Plant) (plant.Verdict, error) {
	if d.Judge == nil {
		return plant.Verdict{}, errors.New("no model backend is configured")
	}

	run, err := d.Store.StartJudgmentRun(ctx, 1)
	if err != nil {
		return plant.Verdict{}, fmt.Errorf("start judgment run: %w", err)
	}
	result, assessErr := d.judgeOne(ctx, run.ID, p)
	if err := d.Store.RecordJudgmentPlantResult(ctx, run.ID,
		judgmentResult(p.ID, result, assessErr)); err != nil {
		return plant.Verdict{}, fmt.Errorf("record judgment result: %w", err)
	}
	if err := d.Store.CompleteJudgmentRun(ctx, run.ID); err != nil {
		return plant.Verdict{}, fmt.Errorf("complete judgment run: %w", err)
	}
	if assessErr != nil {
		return plant.Verdict{}, assessErr
	}
	d.detectIncidents(ctx, run.ID)

	verdict, err := d.Store.LatestVerdict(ctx, p.ID)
	if err != nil {
		return plant.Verdict{}, fmt.Errorf("read fresh verdict: %w", err)
	}
	return verdict, nil
}

func (d Daily) judgeOne(ctx context.Context, runID uuid.UUID, p plant.Plant) (judge.Result, error) {
	evidence, err := d.gather(ctx, p)
	if err != nil {
		return judge.Result{}, err
	}

	result, err := d.Judge.Assess(ctx, evidence)
	if err != nil {
		return judge.Result{}, err
	}

	_, err = d.Store.SaveVerdict(ctx, plant.Verdict{
		PlantID:    p.ID,
		ForDate:    time.Now().UTC(),
		Action:     result.Action,
		Reasoning:  result.Reasoning,
		Confidence: result.Confidence,
		Evidence:   d.verdictEvidence(evidence, result),
	})
	if err != nil {
		return result, err
	}
	if err := d.recordHealth(ctx, runID, evidence, result); err != nil {
		return result, fmt.Errorf("record health: %w", err)
	}
	return result, nil
}

func (d Daily) verdictEvidence(evidence judge.Evidence, result judge.Result) plant.Evidence {
	recorded := plant.Evidence{SensorSummary: result.Summary, ModelVersion: result.Model}
	for _, reading := range evidence.Sensors {
		recorded.ReadingIDs = append(recorded.ReadingIDs, reading.ReadingID)
	}
	for _, observation := range evidence.Recent {
		recorded.ObservationIDs = append(recorded.ObservationIDs, observation.ID)
	}
	if evidence.LatestPhoto != nil {
		recorded.PhotoIDs = append(recorded.PhotoIDs, evidence.LatestPhoto.ID)
	}
	return recorded
}

func (d Daily) recordHealth(ctx context.Context, runID uuid.UUID, evidence judge.Evidence, result judge.Result) error {
	if result.HealthMode == plant.HealthUnchanged {
		return nil
	}
	references := plant.HealthEvidence{
		Summary: result.HealthReasoning, ModelVersion: result.Model,
	}
	for _, reading := range evidence.Sensors {
		references.ReadingIDs = append(references.ReadingIDs, reading.ReadingID)
	}
	for _, observation := range evidence.Recent {
		references.ObservationIDs = append(references.ObservationIDs, observation.ID)
	}
	if evidence.LatestPhoto != nil {
		references.PhotoIDs = append(references.PhotoIDs, evidence.LatestPhoto.ID)
	}
	change := plant.HealthChange{
		PlantID: evidence.Plant.ID, Rationale: result.HealthReasoning,
		Evidence: references, Source: plant.SourceAutomation, Actor: "planty daily",
		JudgmentRunID: &runID,
	}
	if result.HealthMode == plant.HealthBaseline {
		change.Baseline = &result.HealthValue
	} else {
		change.Delta = &result.HealthValue
	}
	_, _, err := d.Store.RecordHealth(ctx, change)
	return err
}

func judgmentResult(plantID uuid.UUID, result judge.Result, err error) store.JudgmentResultInput {
	input := store.JudgmentResultInput{
		PlantID: plantID, Succeeded: err == nil, Attempts: result.Attempts,
		Model: result.Model, OriginalError: result.OriginalError,
		OriginalOutput: result.OriginalOutput,
	}
	if input.Attempts == 0 {
		input.Attempts = 1
	}
	if err == nil {
		return input
	}
	input.FinalError = err.Error()
	var assessment *judge.AssessmentError
	if errors.As(err, &assessment) {
		input.Attempts = assessment.Attempts
		input.Model = assessment.Model
		input.OriginalError = assessment.OriginalError
		input.OriginalOutput = assessment.OriginalOutput
	}
	return input
}

// RetryFailed reopens the latest partial run and judges only its failed
// plants. Existing successful rows and verdicts are never touched.
func (d Daily) RetryFailed(ctx context.Context) error {
	run, failed, err := d.Store.BeginLatestJudgmentRetry(ctx)
	if err != nil {
		return fmt.Errorf("begin failed judgment retry: %w", err)
	}
	if d.Judge == nil {
		_ = d.Store.CompleteJudgmentRun(ctx, run.ID)
		return fmt.Errorf("retry failed judgments: no model backend is configured")
	}

	remaining := 0
	for _, prior := range failed {
		result, judgeErr := d.judgeOne(ctx, run.ID, prior.Plant)
		if judgeErr != nil {
			remaining++
			d.Log.Error("judgment retry failed", "plant", prior.Plant.Slug, "error", judgeErr)
		}
		if err := d.Store.RecordJudgmentPlantResult(ctx, run.ID,
			judgmentResult(prior.Plant.ID, result, judgeErr)); err != nil {
			return fmt.Errorf("record retry for %s: %w", prior.Plant.Slug, err)
		}
	}
	if err := d.Store.CompleteJudgmentRun(ctx, run.ID); err != nil {
		return fmt.Errorf("complete judgment retry: %w", err)
	}
	d.detectIncidents(ctx, run.ID)
	if remaining > 0 {
		return fmt.Errorf("%d plants still failed judgment", remaining)
	}
	return nil
}

func (d Daily) detectIncidents(ctx context.Context, runID uuid.UUID) {
	incidents, err := (IncidentRadar{Store: d.Store}).Run(ctx, runID)
	if err != nil {
		d.Log.Error("garden incident detection failed", "run_id", runID, "error", err)
		return
	}
	if len(incidents) > 0 {
		d.Log.Info("garden incidents detected", "run_id", runID, "count", len(incidents))
	}
}

func (d Daily) gather(ctx context.Context, p plant.Plant) (judge.Evidence, error) {
	evidence := judge.Evidence{Plant: p, Season: season(time.Now())}

	links, err := d.Store.SensorLinks(ctx, &p.ID)
	if err != nil {
		return evidence, err
	}
	for _, link := range links {
		reading, err := d.Store.LatestReading(ctx, link.ID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return evidence, err
		}

		state := judge.SensorState{
			ReadingID:  reading.ID,
			Role:       link.Role,
			Raw:        reading.Value,
			Calibrated: link.Calibrated(),
			TakenAt:    reading.TakenAt,
		}
		if state.Calibrated {
			if f, err := link.Fraction(reading.Value); err == nil {
				state.Fraction = &f
			}
		}
		evidence.Sensors = append(evidence.Sensors, state)
	}

	if recent, err := d.Store.Observations(ctx, p.ID, 8); err == nil {
		evidence.Recent = recent
	}
	if at, err := d.Store.LastWatered(ctx, p.ID); err == nil {
		evidence.LastWatered = &at
	}
	if current, err := d.Store.LatestHealth(ctx, p.ID); err == nil {
		evidence.CurrentHealth = &current
	} else if !errors.Is(err, store.ErrNotFound) {
		return evidence, err
	}
	for _, sensor := range evidence.Sensors {
		if evidence.CurrentHealth == nil || sensor.TakenAt.After(evidence.CurrentHealth.CreatedAt) {
			evidence.HealthEvidenceNew = true
		}
	}
	for _, observation := range evidence.Recent {
		if evidence.CurrentHealth == nil || observation.OccurredAt.After(evidence.CurrentHealth.CreatedAt) {
			evidence.HealthEvidenceNew = true
		}
	}
	d.gatherPhoto(ctx, &evidence)
	if evidence.LatestPhoto != nil &&
		(evidence.CurrentHealth == nil || evidence.LatestPhoto.TakenAt.After(evidence.CurrentHealth.CreatedAt)) {
		evidence.HealthEvidenceNew = true
	}
	return evidence, nil
}

func (d Daily) gatherPhoto(ctx context.Context, evidence *judge.Evidence) {
	if d.Photos == nil {
		evidence.PhotoCoverage = "photo storage is not configured for this job"
		return
	}
	shots, err := d.Store.Photos(ctx, evidence.Plant.ID, 1)
	if err != nil || len(shots) == 0 {
		evidence.PhotoCoverage = "this plant has no readable photograph"
		return
	}
	shot := shots[0]
	body, err := d.Photos.Get(ctx, shot.StorageKey)
	if err != nil {
		evidence.PhotoCoverage = "object storage could not return the latest photograph"
		return
	}
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(body, maxAssessmentPhotoBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxAssessmentPhotoBytes {
		evidence.PhotoCoverage = "the latest photograph was empty, corrupt, or too large"
		return
	}
	media := "image/jpeg"
	switch strings.ToLower(filepath.Ext(shot.StorageKey)) {
	case ".png":
		media = "image/png"
	case ".webp":
		media = "image/webp"
	}
	evidence.LatestPhoto = &judge.PhotoEvidence{
		ID: shot.ID, TakenAt: shot.TakenAt, Caption: shot.Caption,
		Frame: judge.Frame{Media: media, Bytes: raw},
	}
	evidence.PhotoCoverage = "attached"
}

func (d Daily) notify(ctx context.Context, digest plant.Digest) error {
	var b strings.Builder

	for _, entry := range digest.Entries {
		fmt.Fprintf(&b, "%s: %s\n", entry.Plant.CommonName, entry.Verdict.Reasoning)
	}
	incomplete := !digest.RunComplete || digest.Failed > 0 || digest.Checked != digest.Expected
	if incomplete {
		fmt.Fprintf(&b, "\nWarning: Planty checked %d of %d plants; %d failed. This check is incomplete.\n",
			digest.Checked, digest.Expected, digest.Failed)
	}
	if digest.StaleSince != nil {
		b.WriteString("\nWarning: the last complete check is stale.\n")
	}

	title := fmt.Sprintf("%d plants need you", len(digest.Entries))
	switch {
	case incomplete && len(digest.Entries) == 0:
		title = "Planty's check was incomplete"
	case len(digest.Entries) == 1:
		title = "One plant needs you"
	}

	return notify(ctx, d.Notifications, title, b.String(), nil)
}

// season shapes advice: the same soil reading means different things in winter.
func season(t time.Time) string {
	switch t.Month() {
	case time.December, time.January, time.February:
		return "winter, so growth is slow and water needs are lower"
	case time.March, time.April, time.May:
		return "spring, the start of active growth"
	case time.June, time.July, time.August:
		return "summer, peak growth and fastest drying"
	default:
		return "autumn, growth slowing down"
	}
}
