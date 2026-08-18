package store

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// classify marks SQLSTATE class 23, the constraint family, as bad input.
// Everything else is the server's problem and must not report as a 400.
func classify(err error) error {
	if err == nil {
		return nil
	}

	var pg *pgconn.PgError
	if !errors.As(err, &pg) || !strings.HasPrefix(pg.Code, "23") {
		return err
	}
	return fmt.Errorf("%w: %s", plant.ErrInvalid, humanize(pg))
}

// humanize turns a constraint name into something worth reading, because
// "dripper_needs_letpot" tells a person nothing about what to send instead.
func humanize(pg *pgconn.PgError) string {
	switch pg.ConstraintName {
	case "dripper_needs_letpot":
		return "a dripper number only means something on the LetPot line"
	case "link_has_a_subject":
		return "a sensor link needs either a plant or a zone"
	case "soil_belongs_to_a_plant":
		return "a soil moisture sensor measures one pot, so it must name a plant"
	case "away_period_ordered":
		return "an away period has to end after it starts"
	case "plants_slug_key":
		return "a plant with that slug already exists"
	}
	if pg.Code == "23503" {
		return "that references something which does not exist"
	}
	return pg.Message
}
