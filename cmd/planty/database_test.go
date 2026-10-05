package main

import "testing"

func TestOnlyScheduledCommandsWaitForDatabase(t *testing.T) {
	for _, command := range []string{"ingest", "verify-water", "reconcile-actuators", "prune-photos", "daily", "cold", "away", "chase", "remind", "thirst"} {
		if !waitForDatabase(command) {
			t.Errorf("scheduled command %q must survive startup outages", command)
		}
	}
	for _, command := range []string{"water", "agent", "retry", "autopsy", "seed", "migrate", "serve", "unknown"} {
		if waitForDatabase(command) {
			t.Errorf("interactive command %q must not wait to perform a delayed action", command)
		}
	}
}
