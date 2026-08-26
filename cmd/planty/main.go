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
	"strings"
	"syscall"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/agent"
	"github.com/TheOutdoorProgrammer/planty/internal/api"
	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/job"
	"github.com/TheOutdoorProgrammer/planty/internal/judge"
	"github.com/TheOutdoorProgrammer/planty/internal/photos"
	"github.com/TheOutdoorProgrammer/planty/internal/push"
	"github.com/TheOutdoorProgrammer/planty/internal/scheduledjob"
	"github.com/TheOutdoorProgrammer/planty/internal/seed"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
	"github.com/TheOutdoorProgrammer/planty/internal/telemetry"
)

const usage = `planty <command>

  serve    run the HTTP API
  ingest   pull current sensor values from Home Assistant
  verify-water  verify completed manual watering attempts after probes settle
  reconcile-actuators  turn off plant actuators whose durable lease expired
  prune-photos  delete expired scratch photos and finish pending deletions
  daily    judge every plant and send the digest
  retry    rerun only the plants that failed the latest daily judgment
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
	log = slog.New(telemetry.LogHandler(log.Handler()))
	if err := run(log); err != nil {
		log.Error("planty", "error", err)
		os.Exit(1)
	}
}

var (
	version = "dev"
	commit  = "none"
)

func run(log *slog.Logger) error {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		return errors.New("no command given")
	}

	if os.Args[1] == "version" {
		fmt.Printf("planty %s (%s)\n", version, commit)
		return nil
	}

	if os.Args[1] == "gate" {
		allowed, ok := gateAgentVerbs(os.Args[2:])
		if !ok {
			fmt.Fprintln(os.Stderr, "planty gate: invalid agent verb boundary")
			os.Exit(agent.Blocked)
		}
		os.Exit(agent.Gate(os.Stdin, os.Stderr, allowed...))
	}

	dsn := os.Getenv("PLANTY_DATABASE_URL")
	if dsn == "" {
		return errors.New("PLANTY_DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownTelemetry, err := telemetry.Start(ctx, "planty", version, log)
	if err != nil {
		return fmt.Errorf("start telemetry: %w", err)
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := shutdownTelemetry(flushCtx); err != nil {
			log.Error("flush telemetry", "error", err)
		}
	}()

	if os.Args[1] == "agent" {
		store.SilenceMigrations()
	}

	db, err := store.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		return err
	}

	notifications := push.NewFromEnv(db, log)

	switch os.Args[1] {
	case "serve":
		return serve(ctx, db, log, notifications)
	case "migrate":
		log.Info("migrations applied")
		return nil
	case "ingest":
		return job.Ingest{Store: db, HA: homeAssistant(), Log: log}.Run(ctx)
	case "verify-water":
		return job.VerifyWater{Store: db, Log: log, Notifications: notifications}.Run(ctx)
	case "reconcile-actuators":
		_, err := actuatorControl(db, log).Reconcile(ctx, time.Now().UTC())
		return err
	case "prune-photos":
		return prunePhotos(ctx, db, log)
	case "daily":
		return daily(ctx, db, log, notifications).Run(ctx)
	case "retry":
		return daily(ctx, db, log, notifications).RetryFailed(ctx)
	case "cold":
		return coldWatch(db, log, notifications).Run(ctx)
	case "away":
		return job.Away{Store: db, Log: log, Notifications: notifications}.Run(ctx)
	case "chase":
		return job.Escalate{Store: db, Log: log, Notifications: notifications}.Run(ctx)
	case "remind":
		sent, err := job.Remind{
			Store: db, Log: log, Notifications: notifications,
		}.Run(ctx)
		if err == nil {
			log.Info("reminders sent", "count", sent)
		}
		return err
	case "thirst":
		return job.Thirst{Store: db, Log: log, Notifications: notifications}.Run(ctx)
	case "water":
		return water(ctx, db, log, notifications)
	case "autopsy":
		return autopsy(ctx, db, log)
	case "agent":
		control := actuatorControl(db, log)
		return agent.Run(ctx, agent.Deps{Store: db, Actuators: &control, Water: func(ctx context.Context) error {
			return water(ctx, db, log, notifications)
		}},
			os.Stdout, os.Args[2:])
	case "seed":
		return seed.Friends(ctx, db, log, os.Getenv("PLANTY_FRIEND_NAME"))
	default:
		fmt.Fprintln(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}

func prunePhotos(ctx context.Context, db *store.Store, log *slog.Logger) error {
	config, enabled := photoConfig()
	if !enabled {
		return errors.New("photo storage is not configured")
	}
	storage, err := photos.Open(ctx, config)
	if err != nil {
		return err
	}
	return job.PrunePhotos{Store: db, Photos: storage, Log: log}.Run(ctx)
}

func serve(ctx context.Context, db *store.Store, log *slog.Logger, notifications *push.Sender) error {
	addr := os.Getenv("PLANTY_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	apiToken := strings.TrimSpace(os.Getenv("PLANTY_API_TOKEN"))
	if apiToken == "" {
		return errors.New("PLANTY_API_TOKEN is required when serving the API")
	}

	seat := judge.New().Able(acting()).Assigned(db).Instructed(db)
	server := api.New(db, log).WithBearerToken(apiToken).WithJudge(seat).WithPush(notifications)
	if launcher, err := scheduledjob.NewInCluster(); err != nil {
		log.Warn("scheduled job control unavailable", "error", err)
	} else {
		server = server.WithScheduledJobs(launcher)
	}
	go reconcileActuators(ctx, actuatorControl(db, log), log)
	if config, enabled := photoConfig(); enabled {
		manager := photos.Manage(ctx, config, func(state photos.State, err error) {
			if err != nil {
				log.Warn("photo storage unavailable; retrying", "state", state, "error", err)
				return
			}
			log.Info("photo storage ready", "judge", backendName(seat), "can_act", acting() != nil)
		})
		server = server.WithPhotos(manager, seat)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           telemetry.HTTPHandler(server.Handler(), "planty.http"),
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

func actuatorControl(db *store.Store, log *slog.Logger) job.ActuatorControl {
	return job.ActuatorControl{Store: db, HA: homeAssistant(), Log: log}
}

func reconcileActuators(ctx context.Context, control job.ActuatorControl, log *slog.Logger) {
	reconcile := func() {
		stopped, err := control.Reconcile(ctx, time.Now().UTC())
		if err != nil {
			log.Error("actuator reconciliation failed", "error", err)
		}
		if stopped > 0 {
			log.Info("stopped overdue actuators", "count", stopped)
		}
	}
	reconcile()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

func photoConfig() (photos.Config, bool) {
	endpoint := os.Getenv("PLANTY_S3_ENDPOINT")
	if endpoint == "" {
		return photos.Config{}, false
	}
	bucket := os.Getenv("PLANTY_S3_BUCKET")
	if bucket == "" {
		bucket = "planty"
	}
	return photos.Config{
		Endpoint:       endpoint,
		PublicEndpoint: os.Getenv("PLANTY_S3_PUBLIC_ENDPOINT"),
		AccessKey:      os.Getenv("PLANTY_S3_ACCESS_KEY"),
		SecretKey:      os.Getenv("PLANTY_S3_SECRET_KEY"),
		Bucket:         bucket,
		UseSSL:         os.Getenv("PLANTY_S3_SSL") == "true",
		PublicSSL:      os.Getenv("PLANTY_S3_PUBLIC_SSL") != "false",
	}, true
}

// Home Assistant is only Planty's sensor, weather, and actuator bus. Notification
// delivery is intentionally not attached here; scheduled alerts go straight to APNs.
func homeAssistant() *ha.Client {
	return ha.New(os.Getenv("PLANTY_HA_URL"), os.Getenv("PLANTY_HA_TOKEN"))
}

func daily(ctx context.Context, db *store.Store, log *slog.Logger, notifications job.Notifier) job.Daily {
	var storage photos.Storage
	if config, enabled := photoConfig(); enabled {
		opened, err := photos.Open(ctx, config)
		if err != nil {
			log.Warn("daily visual evidence unavailable", "error", err)
		} else {
			storage = opened
		}
	}
	return job.Daily{
		Store:         db,
		Judge:         judge.New().Able(acting()).Assigned(db).Instructed(db),
		Log:           log,
		Notifications: notifications,
		Photos:        storage,
	}
}

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
		Refuse:  agent.RefuseFor,
	}
}

func gateAgentVerbs(args []string) ([]string, bool) {
	if len(args) == 0 {
		return nil, true
	}
	if len(args) != 1 || !strings.HasPrefix(args[0], "--agent-verbs=") {
		return nil, false
	}
	raw := strings.TrimPrefix(args[0], "--agent-verbs=")
	if raw == "" {
		return nil, false
	}
	verbs := strings.Split(raw, ",")
	for _, verb := range verbs {
		if verb == "" || strings.ContainsAny(verb, " ") {
			return nil, false
		}
	}
	return verbs, true
}

func backendName(j *judge.Judge) string {
	if j == nil {
		return "none"
	}
	return j.Backend()
}

func coldWatch(db *store.Store, log *slog.Logger, notifications job.Notifier) job.ColdWatch {
	weather := os.Getenv("PLANTY_WEATHER_ENTITY")
	if weather == "" {
		weather = "weather.nws_home"
	}
	return job.ColdWatch{
		Store:         db,
		HA:            homeAssistant(),
		Log:           log,
		Weather:       weather,
		Notifications: notifications,
	}
}

func water(ctx context.Context, db *store.Store, log *slog.Logger, notifications job.Notifier) error {
	runFor, err := pumpDuration(os.Getenv("PLANTY_PUMP_SECONDS"))
	if err != nil {
		return err
	}
	return job.Water{
		Store:         db,
		HA:            homeAssistant(),
		Log:           log,
		Notifications: notifications,
		PumpSwitch:    os.Getenv("PLANTY_PUMP_SWITCH"),
		PumpSensor:    os.Getenv("PLANTY_PUMP_SENSOR"),
		RunFor:        runFor,
	}.Run(ctx)
}

func pumpDuration(raw string) (time.Duration, error) {
	if raw == "" {
		return 2 * time.Minute, nil
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	const maxDurationSeconds = int64((1<<63 - 1) / time.Second)
	if err != nil || seconds <= 0 || seconds > maxDurationSeconds {
		return 0, fmt.Errorf("PLANTY_PUMP_SECONDS must be a positive integer, got %q", raw)
	}
	return time.Duration(seconds) * time.Second, nil
}

func autopsy(ctx context.Context, db *store.Store, log *slog.Logger) error {
	if len(os.Args) < 3 {
		return errors.New("usage: planty autopsy <slug>")
	}
	seat := judge.New().Assigned(db).Instructed(db)
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
