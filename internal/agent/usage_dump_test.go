package agent

import (
	"os"
	"testing"
)

// Writes the reference the model is handed, so it can be exercised against a
// real model run without the judge package importing this one.
func TestDumpUsage(t *testing.T) {
	path := os.Getenv("PLANTY_DUMP_USAGE")
	if path == "" {
		t.Skip("set PLANTY_DUMP_USAGE to a path")
	}
	if err := os.WriteFile(path, []byte(Usage), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d bytes", len(Usage))
}
