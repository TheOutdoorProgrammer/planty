package judge

import (
	"errors"
	"testing"
)

func TestPermanentModelErrorsAreNotRetried(t *testing.T) {
	transient := errors.New("provider unavailable")
	if !Retryable(transient) {
		t.Fatal("a transient provider error was treated as permanent")
	}
	if Retryable(permanent(transient)) {
		t.Fatal("a permanent provider error was treated as retryable")
	}
	if Retryable(nil) {
		t.Fatal("nil was treated as a retryable error")
	}
}
