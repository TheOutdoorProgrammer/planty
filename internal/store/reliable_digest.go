package store

import (
	"context"
	"errors"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// ReliableDigest overlays the garden-wide run truth onto the actionable verdict
// list. Digest's historical query still owns entries; judgment_runs owns whether
// the latest attempt was complete enough to trust those entries as an all-clear.
func (s *Store) ReliableDigest(ctx context.Context, staleAfter time.Duration) (plant.Digest, error) {
	digest, err := s.Digest(ctx, staleAfter)
	if err != nil {
		return digest, err
	}

	run, err := s.LatestJudgmentRun(ctx)
	if errors.Is(err, ErrNotFound) {
		digest.Checked = 0
		digest.Expected = 0
		digest.Failed = 0
		digest.RunComplete = false
		digest.StaleSince = nil
		digest.NeverRun = true
		return digest, nil
	}
	if err != nil {
		return digest, err
	}

	digest.Date = run.StartedAt
	digest.Checked = run.Succeeded
	digest.Expected = run.Expected
	digest.Failed = run.Failed
	digest.RunComplete = run.CompletedAt != nil
	digest.NeverRun = false
	digest.StaleSince = nil
	failed, err := s.FailedJudgments(ctx, run.ID)
	if err != nil {
		return digest, err
	}
	for _, result := range failed {
		digest.Failures = append(digest.Failures, result.JudgmentFailure)
	}

	// An unfinished or failed run is incomplete, not stale. The explicit counts
	// carry that state to clients. Staleness means the latest completed attempt
	// is simply too old.
	if run.CompletedAt != nil && time.Since(*run.CompletedAt) > staleAfter {
		at := *run.CompletedAt
		digest.StaleSince = &at
	}
	return digest, nil
}
