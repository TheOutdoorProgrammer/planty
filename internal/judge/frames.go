package judge

import (
	"fmt"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// MaxTimelineImages caps one diagnosis. Enough to show a trend, few enough that
// the oldest frame is still the same plant in the same pot.
const MaxTimelineImages = 6

// Frame is one dated image of a plant.
type Frame struct {
	TakenAt time.Time
	Media   string
	Bytes   []byte
	Caption string
}

func caption(f Frame) string {
	if f.Caption == "" {
		return ""
	}
	return " (" + f.Caption + ")"
}

// describePot reports the drainage hole independently of the other pot
// details, because a pot with no hole is the most common way a plant drowns
// and it used to be dropped whenever nobody recorded the material.
func describePot(p plant.Plant) string {
	var parts []string
	if p.PotSizeIn != nil {
		parts = append(parts, fmt.Sprintf("%.0f inch", *p.PotSizeIn))
	}
	if p.PotMaterial != "" {
		parts = append(parts, p.PotMaterial)
	}

	described := ""
	if len(parts) > 0 {
		described = "Pot: " + strings.Join(parts, " ")
	}

	if p.HasDrainage != nil && !*p.HasDrainage {
		if described == "" {
			return "Pot has NO drainage hole, so water cannot leave it."
		}
		return described + ", with NO drainage hole, so water cannot leave it."
	}
	if described == "" {
		return ""
	}
	return described + "."
}

func orUnknown(s string) string {
	if s == "" {
		return "unrecorded"
	}
	return s
}
