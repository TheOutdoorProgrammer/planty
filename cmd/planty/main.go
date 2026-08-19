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
	"strconv"
	"syscall"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/agent"
	"github.com/TheOutdoorProgrammer/planty/internal/api"
	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/job"
	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/photos"
	"github.com/TheOutdoorProgrammer/planty/internal/seed"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

const usage = `planty <command>

  serve    run the HTTP API
  ingest   pull current sensor values from Home Assistant
  daily    judge every plant and send the digest
  cold     check tonight's forecast, both bringing in and putting back out
  away     pre-departure watering pass, or the briefing on return
  chase    chase verdicts nobody acknowledged, one rung up the ladder
  remind   send the chores that are owed: watering, misting, feeding
  thirst   report which plants the probes call dry, and water nothing
  water    run the LetPot line and verify it, only when asked by hand
  autopsy  work out what killed a plant: planty autopsy <slug>
  agent    everything a model may do to the garden: planty agent help
  gate     PreToolUse hook deciding whether a tool call may proceed
  seed     load the sabbatical plants and their open questions
  migrate  apply database migrations and exit
  version  print the version and exit`

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("planty", "error", err)
		os.Exit(1)
	}
}

// Set by the linker at release. A binary somebody installed from a tap has to
// be able to say what it is without a database in front of it.
var (
	version = "dev"
	commit  = "none"
)

func run(log *slog.Logger) error {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		return errors.New("no command given")
	}

	// Answered before the database is demanded, so `planty version` works on a
	// laptop that has never seen the cluster.
	if os.Args[1] == "version" {
		fmt.Printf("planty %s (%s)\n", version, commit)
		return nil
	}

	// A hook answers on every tool call and has no business opening Postgres.
	if os.Args[1] == "gate" {
		os.Exit(agent.Gate(os.Stdin, os.Stderr))
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
	case "away":
		return job.Away{Store: db, HA: homeAssistant(), Log: log, Notifier: notifier()}.Run(ctx)
	case "chase":
		return job.Escalate{Store: db, HA: homeAssistant(), Log: log, Notifier: notifier()}.Run(ctx)
	case "remind":
		sent, err := job.Remind{
			Store: db, HA: homeAssistant(), Log: log, Notifier: notifier(),
		}.Run(ctx)
		if err == nil {
			log.Info("reminders sent", "count", sent)
		}
		return err
	case "thirst":
		return job.Thirst{Store: db, HA: homeAssistant(), Log: log, Notifier: notifier()}.Run(ctx)
	case "water":
		return water(db, log).Run(ctx)
	case "autopsy":
		return autopsy(ctx, db, log)
	case "agent":
		// The agent's water verb runs the same surveyed LetPot pass as `water`.
		return agent.Run(ctx, agent.Deps{Store: db, Water: water(db, log).Run},
			os.Stdout, os.Args[2:])
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

	server := api.New(db, log)
	if store, err := photoStore(ctx); err != nil {
		// Photos are worth having, not worth refusing to serve plants over.
		log.Warn("photo storage unavailable, photo routes disabled", "error", err)
	} else if store != nil {
		seat := judge.New().Able(acting())
		server = server.WithPhotos(store, seat)
		log.Info("photo storage ready", "judge", backendName(seat), "can_act", acting() != nil)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
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

// photoStore returns nil without error when object storage is not configured,
// which is the normal state before MinIO credentials are set.
func photoStore(ctx context.Context) (*photos.Store, error) {
	endpoint := os.Getenv("PLANTY_S3_ENDPOINT")
	if endpoint == "" {
		return nil, nil
	}
	bucket := os.Getenv("PLANTY_S3_BUCKET")
	if bucket == "" {
		bucket = "planty"
	}
	return photos.Open(ctx, photos.Config{
		Endpoint:       endpoint,
		PublicEndpoint: os.Getenv("PLANTY_S3_PUBLIC_ENDPOINT"),
		AccessKey:      os.Getenv("PLANTY_S3_ACCESS_KEY"),
		SecretKey:      os.Getenv("PLANTY_S3_SECRET_KEY"),
		Bucket:         bucket,
		UseSSL:         os.Getenv("PLANTY_S3_SSL") == "true",
		PublicSSL:      os.Getenv("PLANTY_S3_PUBLIC_SSL") != "false",
	})
}

func homeAssistant() *ha.Client {
	return ha.New(os.Getenv("PLANTY_HA_URL"), os.Getenv("PLANTY_HA_TOKEN"))
}

func daily(db *store.Store, log *slog.Logger) job.Daily {
	return job.Daily{
		Store:    db,
		HA:       homeAssistant(),
		Judge:    judge.New(),
		Log:      log,
		Notifier: notifier(),
	}
}

// acting is what a conversation may write, nil when it may write nothing.
// Only `serve` asks: a scheduled verdict that recorded observations would be
// judging evidence it wrote itself. PLANTY_JUDGE_CAN_ACT=false switches it off.
func acting() *judge.Acting {
	if os.Getenv("PLANTY_JUDGE_CAN_ACT") == "false" {
		return nil
	}
	binary, err := os.Executable()
	if err != nil {
		return nil
	}
	return &judge.Acting{
		Binary:  binary,
		Usage:   agent.Usage,
		Trusted: agent.Trusted,
		Sources: agent.Sources,
	}
}

// backendName says which way judgments are being bought, which is the first
// thing to check when a verdict is missing or a bill is a surprise.
func backendName(j *judge.Judge) string {
	if j == nil {
		return "none"
	}
	return j.Backend()
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

func water(db *store.Store, log *slog.Logger) job.Water {
	runFor := 2 * time.Minute
	if raw := os.Getenv("PLANTY_PUMP_SECONDS"); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			runFor = time.Duration(secs) * time.Second
		}
	}
	return job.Water{
		Store:      db,
		HA:         homeAssistant(),
		Log:        log,
		Notifier:   notifier(),
		PumpSwitch: os.Getenv("PLANTY_PUMP_SWITCH"),
		PumpSensor: os.Getenv("PLANTY_PUMP_SENSOR"),
		RunFor:     runFor,
	}
}

func autopsy(ctx context.Context, db *store.Store, log *slog.Logger) error {
	if len(os.Args) < 3 {
		return errors.New("usage: planty autopsy <slug>")
	}
	// Asked for by hand, so refusing outright beats a nil judge panicking.
	seat := judge.New()
	if seat == nil {
		return errors.New("an autopsy needs a model: set ANTHROPIC_API_KEY, " +
			"or install the claude CLI and sign it in")
	}

	record, err := job.Postmortem{
		Store: db,
		Judge: seat,
		Log:   log,
	}.Run(ctx, os.Args[2])
	if err != nil {
		return err
	}

	fmt.Printf("\nLikely cause: %s\n\n%s\n\nLesson: %s\n",
		record.LikelyCause, record.Narrative, record.Lesson)
	return nil
}

func notifier() string {
	if n := os.Getenv("PLANTY_NOTIFY_SERVICE"); n != "" {
		return n
	}
	return "notify"
}
