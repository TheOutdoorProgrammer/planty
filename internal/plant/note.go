package plant

import (
	"time"

	"github.com/google/uuid"
)

// Note is something a person wrote down about a plant. Many per plant and
// individually editable, unlike CareProfile.Notes, which is the single field
// the whole app overwrites.
type Note struct {
	ID      uuid.UUID `json:"id"`
	PlantID uuid.UUID `json:"plant_id"`

	Title string `json:"title,omitempty"`
	Body  string `json:"body"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Edited reports whether a note has been changed since it was written, which
// is the only reason to show two dates instead of one.
func (n Note) Edited() bool { return n.UpdatedAt.After(n.CreatedAt.Add(time.Second)) }
