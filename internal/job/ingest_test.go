package job

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
	"github.com/google/uuid"
)

type ingestRecorder struct {
	*store.Store
	link     plant.SensorLink
	readings []plant.Reading
	writeErr error
}

func (s *ingestRecorder) SensorLinks(context.Context, *uuid.UUID) ([]plant.SensorLink, error) {
	return []plant.SensorLink{s.link}, nil
}

func (s *ingestRecorder) RecordReading(_ context.Context, reading plant.Reading) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	s.readings = append(s.readings, reading)
	return nil
}

type ingestStatesFunc func(context.Context) ([]ha.State, error)

func (f ingestStatesFunc) States(ctx context.Context) ([]ha.State, error) { return f(ctx) }

func refusedConnection() error {
	return &url.Error{Op: "Get", URL: "https://home.example.com/api/states", Err: &net.OpError{
		Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED,
	}}
}

func recoveredStates() []ha.State {
	return []ha.State{{EntityID: "sensor.soil", State: "42.5", Attributes: map[string]any{"unit_of_measurement": "%"}}}
}

func newIngestRecorder() *ingestRecorder {
	return &ingestRecorder{link: plant.SensorLink{ID: uuid.New(), HAEntityID: "sensor.soil"}}
}

func TestIngestRecoversAcrossHomeAssistantRestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		db := newIngestRecorder()
		attempts := 0
		client := ingestStatesFunc(func(context.Context) ([]ha.State, error) {
			attempts++
			if len(db.readings) != 0 {
				t.Fatal("stored a reading before states were retrieved")
			}
			if time.Since(started) < 48*time.Second {
				return nil, refusedConnection()
			}
			return recoveredStates(), nil
		})
		if err := (Ingest{Store: db, HA: client, Log: quietLog()}).Run(t.Context()); err != nil {
			t.Fatalf("ingest should recover after the listener returns: %v", err)
		}
		if attempts < 2 || len(db.readings) != 1 {
			t.Fatalf("attempts=%d readings=%v, want retries and exactly one reading", attempts, db.readings)
		}
		r := db.readings[0]
		if r.SensorLinkID != db.link.ID || r.Value != 42.5 || r.Unit != "%" || r.TakenAt.Before(started.Add(48*time.Second)) {
			t.Fatalf("wrong recovered reading: %+v", r)
		}
	})
}

func TestIngestPersistentRefusalFailsWithinBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		db := newIngestRecorder()
		attempts := 0
		client := ingestStatesFunc(func(context.Context) ([]ha.State, error) {
			attempts++
			return nil, refusedConnection()
		})
		err := (Ingest{Store: db, HA: client, Log: quietLog()}).Run(t.Context())
		if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, syscall.ECONNREFUSED) {
			t.Fatalf("want deadline and original refusal, got %v", err)
		}
		if elapsed := time.Since(started); elapsed != 2*time.Minute {
			t.Fatalf("elapsed=%v, want two-minute budget", elapsed)
		}
		if attempts < 2 || attempts > 20 || len(db.readings) != 0 {
			t.Fatalf("attempts=%d readings=%v", attempts, db.readings)
		}
	})
}

func TestIngestCancellationStopsRetryWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() {
			select {
			case <-time.After(time.Second):
				cancel()
			case <-ctx.Done():
			}
		}()
		db := newIngestRecorder()
		attempts := 0
		client := ingestStatesFunc(func(context.Context) ([]ha.State, error) {
			attempts++
			return nil, refusedConnection()
		})
		err := (Ingest{Store: db, HA: client, Log: quietLog()}).Run(ctx)
		if !errors.Is(err, context.Canceled) || time.Since(started) != time.Second || attempts != 1 || len(db.readings) != 0 {
			t.Fatalf("err=%v elapsed=%v attempts=%d readings=%v", err, time.Since(started), attempts, db.readings)
		}
	})
}

func TestIngestDoesNotRetryOtherFailures(t *testing.T) {
	for _, failure := range []error{errors.New("home assistant GET /api/states: 401 Unauthorized"), errors.New("invalid JSON"), context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			db := newIngestRecorder()
			attempts := 0
			client := ingestStatesFunc(func(context.Context) ([]ha.State, error) {
				attempts++
				return recoveredStates(), failure
			})
			err := (Ingest{Store: db, HA: client, Log: quietLog()}).Run(t.Context())
			if !errors.Is(err, failure) || attempts != 1 || len(db.readings) != 0 {
				t.Fatalf("err=%v attempts=%d readings=%v", err, attempts, db.readings)
			}
		})
	}
}

func TestIngestDoesNotReplayWrites(t *testing.T) {
	db := newIngestRecorder()
	db.writeErr = refusedConnection()
	attempts := 0
	client := ingestStatesFunc(func(context.Context) ([]ha.State, error) {
		attempts++
		return recoveredStates(), nil
	})
	err := (Ingest{Store: db, HA: client, Log: quietLog()}).Run(t.Context())
	if !errors.Is(err, db.writeErr) || attempts != 1 {
		t.Fatalf("err=%v states calls=%d, want write error without replay", err, attempts)
	}
}

func TestIngestRecoversFromRealConnectionRefusal(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/states" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected states request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`[{"entity_id":"sensor.soil","state":"42.5","attributes":{"unit_of_measurement":"%"}}]`))
	}))
	address := server.Listener.Addr().String()
	if err := server.Listener.Close(); err != nil {
		t.Fatal(err)
	}
	client := ha.New("http://"+address, "test-token")
	attempts := 0
	read := ingestStatesFunc(func(ctx context.Context) ([]ha.State, error) {
		attempts++
		states, err := client.States(ctx)
		if attempts == 1 {
			if !errors.Is(err, syscall.ECONNREFUSED) {
				t.Fatalf("expected actual TCP refusal, got %v", err)
			}
			listener, listenErr := net.Listen("tcp", address)
			if listenErr != nil {
				t.Fatal(listenErr)
			}
			server.Listener = listener
			server.Start()
			t.Cleanup(server.Close)
		}
		return states, err
	})
	db := newIngestRecorder()
	if err := (Ingest{Store: db, HA: read, Log: quietLog()}).Run(t.Context()); err != nil {
		t.Fatalf("ingest after listener recovery: %v", err)
	}
	if attempts != 2 || len(db.readings) != 1 || db.readings[0].Value != 42.5 {
		t.Fatalf("attempts=%d readings=%v", attempts, db.readings)
	}
}

func TestIngestHTTPFailuresRemainErrors(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusServiceUnavailable, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.WriteHeader(status)
				_, _ = w.Write([]byte("invalid JSON"))
			}))
			defer server.Close()
			db := newIngestRecorder()
			err := (Ingest{Store: db, HA: ha.New(server.URL, "test-token"), Log: quietLog()}).Run(t.Context())
			server.Close()
			if err == nil || requests != 1 || len(db.readings) != 0 {
				t.Fatalf("err=%v requests=%d readings=%v", err, requests, db.readings)
			}
		})
	}
}

func TestIngestDeadlineBoundsActiveRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
		defer cancel()
		db := newIngestRecorder()
		attempts := 0
		client := ingestStatesFunc(func(ctx context.Context) ([]ha.State, error) {
			attempts++
			if attempts == 1 {
				return nil, refusedConnection()
			}
			<-ctx.Done()
			return nil, ctx.Err()
		})
		err := (Ingest{Store: db, HA: client, Log: quietLog()}).Run(ctx)
		if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, syscall.ECONNREFUSED) || time.Since(started) != 12*time.Second || attempts != 2 || len(db.readings) != 0 {
			t.Fatalf("err=%v elapsed=%v attempts=%d readings=%v", err, time.Since(started), attempts, db.readings)
		}
	})
}

func TestIngestAlreadyCanceledDoesNotRequestStates(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	db := newIngestRecorder()
	client := ingestStatesFunc(func(context.Context) ([]ha.State, error) {
		t.Fatal("requested states after cancellation")
		return nil, nil
	})
	if err := (Ingest{Store: db, HA: client, Log: quietLog()}).Run(ctx); !errors.Is(err, context.Canceled) || len(db.readings) != 0 {
		t.Fatalf("err=%v readings=%v", err, db.readings)
	}
}
