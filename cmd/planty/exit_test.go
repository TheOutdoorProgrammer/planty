package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/judge"
)

func TestCommandExitCode(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"success", nil, 0},
		{"ordinary failure", errors.New("database unavailable"), 1},
		{"rate limit", errors.New("provider returned 429: try again later"), 1},
		{"quota text without classification", errors.New("model usage quota exhausted"), 1},
		{"quota", judge.ErrQuotaExhausted, 3},
		{"wrapped quota", fmt.Errorf("assessment: %w", judge.ErrQuotaExhausted), 3},
		{"quota and notification failure", errors.Join(judge.ErrQuotaExhausted, errors.New("push unavailable")), 3},
		{"provider unavailable", judge.ErrProviderUnavailable, 4},
		{"wrapped provider unavailable", fmt.Errorf("assessment: %w", judge.ErrProviderUnavailable), 4},
		{"provider and notification failure", errors.Join(judge.ErrProviderUnavailable, errors.New("push unavailable")), 4},
		{"provider text without classification", errors.New("model provider unavailable"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := commandExitCode(tc.err); got != tc.want {
				t.Fatalf("exit code = %d, want %d", got, tc.want)
			}
		})
	}
}
