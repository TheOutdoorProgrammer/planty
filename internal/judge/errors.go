package judge

import "errors"

// ErrQuotaExhausted means the provider's usage allowance must reset before more work can succeed.
var ErrQuotaExhausted = errors.New("model usage quota exhausted")

type permanentError struct {
	cause error
}

func (e *permanentError) Error() string { return e.cause.Error() }
func (e *permanentError) Unwrap() error { return e.cause }

func permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{cause: err}
}

// Retryable reports whether repeating a failed model request can plausibly
// produce a different result without changing its input or configuration.
func Retryable(err error) bool {
	var target *permanentError
	return err != nil && !errors.As(err, &target)
}
