// Package scheduledjob launches bounded, allowlisted manual copies of Planty's
// Kubernetes CronJobs.
package scheduledjob

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ErrUnavailable means the API is not running with Kubernetes job access.
var ErrUnavailable = errors.New("scheduled job control is unavailable")

// ID is a stable app-facing job identity. Kubernetes resource names remain an
// implementation detail and callers can never submit an arbitrary one.
type ID string

const (
	Ingest             ID = "ingest"
	VerifyWater        ID = "verify-water"
	ReconcileActuators ID = "reconcile-actuators"
	PrunePhotos        ID = "prune-photos"
	Daily              ID = "daily"
	Chase              ID = "chase"
	Away               ID = "away"
	Thirst             ID = "thirst"
	Cold               ID = "cold"
	Remind             ID = "remind"
)

// Definition is intentionally code-owned. A route parameter may select one of
// these exact templates; it can never become a Kubernetes name or command.
type Definition struct {
	ID       ID
	CronJob  string
	Name     string
	Purpose  string
	Category string
	Cadence  string
}

var definitions = []Definition{
	{Ingest, "planty-ingest", "Refresh sensor readings", "Pull current readings from Home Assistant.", "Care", "Every 20 minutes"},
	{Daily, "planty-daily", "Check every plant", "Create a fresh evidence-backed daily assessment.", "Care", "Daily at 8:00 AM"},
	{Remind, "planty-remind", "Send due reminders", "Send chores whose scheduled time has arrived.", "Care", "Hourly"},
	{Thirst, "planty-thirst", "Check thirsty plants", "Report what calibrated moisture probes call dry.", "Care", "Daily at 9:00 AM and 6:00 PM"},
	{Cold, "planty-cold", "Check tonight's cold", "Review the forecast and current shelter plan.", "Care", "Daily at 3:00 PM"},
	{Away, "planty-away", "Run away-mode check", "Prepare departure care or a return briefing.", "Care", "Daily at 8:30 AM"},
	{Chase, "planty-chase", "Chase overdue care", "Escalate unacknowledged findings within the existing cap.", "Care", "Daily at 1:00 PM and 8:00 PM"},
	{VerifyWater, "planty-verify-water", "Verify watering evidence", "Check whether recent manual watering reached the soil.", "Care", "Every 15 minutes"},
	{ReconcileActuators, "planty-reconcile-actuators", "Reconcile fan deadlines", "Stop any actuator whose durable run deadline passed.", "Maintenance", "Every minute"},
	{PrunePhotos, "planty-prune-photos", "Clean expired photos", "Remove expired scratch photos and finish pending deletions.", "Maintenance", "Daily at 3:30 AM"},
}

// State is the lifecycle Kubernetes reports for one manual run.
type State string

const (
	Queued    State = "queued"
	Running   State = "running"
	Succeeded State = "succeeded"
	Failed    State = "failed"
)

type Run struct {
	ID          string     `json:"id"`
	Job         ID         `json:"job"`
	State       State      `json:"state"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Detail      string     `json:"detail,omitempty"`
}

func (r Run) Active() bool { return r.State == Queued || r.State == Running }

type Scheduled struct {
	ID        ID     `json:"id"`
	Name      string `json:"name"`
	Purpose   string `json:"purpose"`
	Category  string `json:"category"`
	Cadence   string `json:"cadence"`
	Schedule  string `json:"schedule"`
	TimeZone  string `json:"time_zone"`
	Suspended bool   `json:"suspended"`
	LatestRun *Run   `json:"latest_run,omitempty"`
}

// Launcher is the seam used by HTTP and by tests. Start returns created=false
// when the same CronJob is already active, making repeated taps harmless.
type Launcher interface {
	List(context.Context) ([]Scheduled, error)
	Start(context.Context, ID) (run Run, created bool, err error)
}

func Definitions() []Definition { return append([]Definition(nil), definitions...) }

func Lookup(id ID) (Definition, bool) {
	for _, definition := range definitions {
		if definition.ID == id {
			return definition, true
		}
	}
	return Definition{}, false
}

func DefinitionForCronJob(name string) (Definition, bool) {
	for _, definition := range definitions {
		if definition.CronJob == name {
			return definition, true
		}
	}
	return Definition{}, false
}

func ValidateID(raw string) (ID, error) {
	id := ID(strings.TrimSpace(raw))
	if _, ok := Lookup(id); !ok {
		return "", fmt.Errorf("unknown scheduled job %q", raw)
	}
	return id, nil
}

func SortRunsNewestFirst(runs []Run) {
	sort.SliceStable(runs, func(i, j int) bool {
		return runTime(runs[i]).After(runTime(runs[j]))
	})
}

func runTime(run Run) time.Time {
	if run.StartedAt != nil {
		return *run.StartedAt
	}
	if run.CompletedAt != nil {
		return *run.CompletedAt
	}
	return time.Time{}
}
