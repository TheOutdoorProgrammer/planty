// Command planty serves the API behind the iOS app and the Dusk plugin, and
// runs the scheduled jobs.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/api"
	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/job"
	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/seed"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

const usage = `planty <command>

  serve    run the HTTP API
  ingest   pull current sensor values from Home Assistant
  daily    judge every plant and send the digest
  cold     check tonight's forecast and warn about plants to bring in
  seed     load the sabbatical plants and their open questions
  migrate  apply database migrations and exit`

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("planty", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		return errors.New("no command given")
	}

	dsn := os.Getenv("PLANTY_DATABASE_URL")
	if dsn == "" {
		return errors.New("PLANTY_DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		return err
	}

	switch os.Args[1] {
	case "serve":
		return serve(ctx, db, log)
	case "migrate":
		log.Info("migrations applied")
		return nil
	case "ingest":
		return job.Ingest{Store: db, HA: homeAssistant(), Log: log}.Run(ctx)
	case "daily":
		return daily(db, log).Run(ctx)
	case "cold":
		return coldWatch(db, log).Run(ctx)
	case "seed":
		return seed.Friends(ctx, db, log, os.Getenv("PLANTY_FRIEND_NAME"))
	default:
		fmt.Fprintln(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}

func serve(ctx context.Context, db *store.Store, log *slog.Logger) error {
	addr := os.Getenv("PLANTY_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           api.New(db, log).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	log.Info("listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func homeAssistant() *ha.Client {
	return ha.New(os.Getenv("PLANTY_HA_URL"), os.Getenv("PLANTY_HA_TOKEN"))
}

func daily(db *store.Store, log *slog.Logger) job.Daily {
	return job.Daily{
		Store:    db,
		HA:       homeAssistant(),
		Judge:    judge.New(os.Getenv("ANTHROPIC_API_KEY")),
		Log:      log,
		Notifier: notifier(),
	}
}

func coldWatch(db *store.Store, log *slog.Logger) job.ColdWatch {
	weather := os.Getenv("PLANTY_WEATHER_ENTITY")
	if weather == "" {
		weather = "weather.nws_home"
	}
	return job.ColdWatch{
		Store:    db,
		HA:       homeAssistant(),
		Log:      log,
		Weather:  weather,
		Notifier: notifier(),
	}
}

func notifier() string {
	if n := os.Getenv("PLANTY_NOTIFY_SERVICE"); n != "" {
		return n
	}
	return "notify"
}
