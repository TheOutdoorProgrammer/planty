package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/photos"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

const ScratchPhotoRetention = 30 * 24 * time.Hour

type PrunePhotos struct {
	Store     *store.Store
	Photos    photos.Storage
	Log       *slog.Logger
	Retention time.Duration
	Now       func() time.Time
}

func (p PrunePhotos) Run(ctx context.Context) error {
	retention := p.Retention
	if retention <= 0 {
		retention = ScratchPhotoRetention
	}
	now := time.Now().UTC()
	if p.Now != nil {
		now = p.Now().UTC()
	}
	if p.Photos == nil {
		return errors.New("photo storage is not configured")
	}

	var deleted int
	for {
		claimed, err := p.Store.ClaimExpiredScratchPhotos(ctx, now.Add(-retention), 100)
		if err != nil {
			return fmt.Errorf("claim photos for deletion: %w", err)
		}
		if len(claimed) == 0 {
			p.Log.Info("photo pruning complete", "deleted", deleted)
			return nil
		}
		var failures []error
		for _, shot := range claimed {
			if err := p.Photos.Delete(ctx, shot.StorageKey); err != nil {
				failures = append(failures, fmt.Errorf("delete object %s: %w", shot.StorageKey, err))
				continue
			}
			if err := p.Store.FinalizePhotoDeletion(ctx, shot.ID); err != nil {
				failures = append(failures, fmt.Errorf("delete photo row %s: %w", shot.ID, err))
				continue
			}
			deleted++
		}
		if len(failures) > 0 {
			return errors.Join(failures...)
		}
	}
}
