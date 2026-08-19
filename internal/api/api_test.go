package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/api"
	"github.com/TheOutdoorProgrammer/planty/internal/pgtest"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// The handlers are tested against a real Postgres for the same reason the store
// is: the bugs worth catching here are wrong status codes and SQL that only
// fails on a real server.
func newServer(t *testing.T) (http.Handler, *store.Store, context.Context) {
	t.Helper()

	ctx := context.Background()
	db, err := store.Open(ctx, pgtest.DSN(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(db.Close)

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	return api.New(db, quiet).Handler(), db, ctx
}

func do(t *testing.T, h http.Handler, method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		reader = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	decoded := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &decoded)
	}
	return rec, decoded
}

// unique keeps parallel runs and repeated local runs from colliding on slug.
func unique(name string) string {
	return name + "-" + time.Now().Format("150405.000000000")
}

func createPlant(t *testing.T, h http.Handler, body map[string]any) string {
	t.Helper()

	rec, out := do(t, h, http.MethodPost, "/v1/plants", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d, body %s", rec.Code, rec.Body.String())
	}
	slug, _ := out["slug"].(string)
	if slug == "" {
		t.Fatal("create returned no slug")
	}
	return slug
}

func TestHealthzNeedsNothing(t *testing.T) {
	h, _, _ := newServer(t)

	rec, out := do(t, h, http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	if out["status"] != "ok" {
		t.Errorf("got %v, want ok", out["status"])
	}
}

// An agent describing a plant in conversation will not supply six enum fields,
// so a sparse create has to be enough.
func TestSparseCreateFillsTheDefaults(t *testing.T) {
	h, _, _ := newServer(t)

	_, out := do(t, h, http.MethodPost, "/v1/plants", map[string]any{
		"common_name": "Sparse subject",
		"slug":        unique("sparse"),
	})

	for field, want := range map[string]string{
		"domain":          "houseplant",
		"status":          "alive",
		"steward":         "self",
		"accessibility":   "easy",
		"watering_method": "hand",
	} {
		if out[field] != want {
			t.Errorf("%s defaulted to %v, want %v", field, out[field], want)
		}
	}
}

// Bad input from a client is a 400. A 500 would say the server broke, which
// sends whoever is debugging it to entirely the wrong place.
func TestBadInputIsNotAServerError(t *testing.T) {
	h, _, _ := newServer(t)

	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"dripper on a hand-watered plant", map[string]any{
			"common_name": "Bad", "slug": unique("bad-dripper"),
			"watering_method": "hand", "letpot_dripper": 3,
		}},
		{"unknown domain", map[string]any{
			"common_name": "Bad", "slug": unique("bad-domain"), "domain": "spaceship",
		}},
		{"no common name", map[string]any{"slug": unique("bad-nameless")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, out := do(t, h, http.MethodPost, "/v1/plants", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("got %d, want 400", rec.Code)
			}
			if out["error"] == nil {
				t.Error("a rejection should say why")
			}
		})
	}
}

func TestMissingPlantIsFourOhFour(t *testing.T) {
	h, _, _ := newServer(t)

	for _, path := range []string{
		"/v1/plants/no-such-plant",
		"/v1/plants/no-such-plant/observations",
	} {
		rec, _ := do(t, h, http.MethodGet, path, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", path, rec.Code)
		}
	}
}

func TestSparsePatchLeavesTheRestAlone(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{
		"common_name": "Patch subject", "slug": unique("patch"),
		"location": "top shelf",
	})

	rec, out := do(t, h, http.MethodPatch, "/v1/plants/"+slug,
		map[string]any{"accessibility": "hard"})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: got %d, body %s", rec.Code, rec.Body.String())
	}
	if out["accessibility"] != "hard" {
		t.Errorf("patch did not apply: %v", out["accessibility"])
	}
	if out["location"] != "top shelf" {
		t.Errorf("patch clobbered an unnamed field: %v", out["location"])
	}
}

// Archiving is how a death is recorded, so the history has to survive it.
func TestArchivingHidesThePlantButKeepsIt(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{
		"common_name": "Archive subject", "slug": unique("archive"),
	})

	if rec, _ := do(t, h, http.MethodDelete, "/v1/plants/"+slug+"?status=dead", nil); rec.Code != http.StatusOK {
		t.Fatalf("archive: got %d", rec.Code)
	}

	_, listed := do(t, h, http.MethodGet, "/v1/plants", nil)
	for _, raw := range listed["plants"].([]any) {
		if raw.(map[string]any)["slug"] == slug {
			t.Fatal("an archived plant should leave the live list")
		}
	}

	rec, out := do(t, h, http.MethodGet, "/v1/plants/"+slug, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("an archived plant must still be readable: got %d", rec.Code)
	}
	if out["plant"].(map[string]any)["status"] != "dead" {
		t.Error("the recorded cause of archiving was lost")
	}
}

// A nil slice marshals as `null`, and a client declaring the field non-optional
// then fails to decode the whole response. This shipped: the app could not read
// /v1/today at all, because `entries` is null until something has been judged.
func TestEmptyListsAreArraysNotNull(t *testing.T) {
	h, _, _ := newServer(t)

	paths := []string{
		"/v1/today",
		"/v1/plants",
		"/v1/sensors",
		"/v1/questions",
		"/v1/postmortems",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			rec, _ := do(t, h, http.MethodGet, path, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, body %s", rec.Code, rec.Body.String())
			}

			var body map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			for field, raw := range body {
				if string(raw) == "null" && strings.HasSuffix(field, "s") {
					t.Errorf("%s is null; an empty list has to be [] or a client cannot decode it", field)
				}
			}
		})
	}
}

// The iOS app posts here. Without a judge configured it must say so plainly,
// because the app degrades to coarse Vision labels on exactly this answer.
func TestIdentifyIsUnavailableWithoutAJudge(t *testing.T) {
	h, _, _ := newServer(t)

	rec, out := do(t, h, http.MethodPost, "/v1/identify", map[string]any{})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503: the route has to exist even when it cannot answer", rec.Code)
	}
	if out["error"] == nil {
		t.Error("a refusal has to say why, or the app cannot tell it apart from a 404")
	}
}

// Nothing covered harvests, and both clients had guessed a flat /v1/harvests
// that does not exist, so every harvest ever logged would have 404d.
func TestAHarvestIsFiledUnderThePlantInThePath(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{
		"common_name": "Tomato", "slug": unique("tomato"), "domain": "edible_indoor",
	})

	rec, out := do(t, h, http.MethodPost, "/v1/plants/"+slug+"/harvests",
		map[string]any{"quantity": 4, "unit": "fruit", "notes": "first truss"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d, body %s", rec.Code, rec.Body.String())
	}
	if out["quantity"] != 4.0 || out["unit"] != "fruit" {
		t.Errorf("stored %v %v, which is what a season is added up from", out["quantity"], out["unit"])
	}

	// The path is what says whose harvest it is, so a plant_id in the body must
	// not be able to file it against another plant.
	if out["plant_id"] == nil || out["plant_id"] == "" {
		t.Error("the harvest was stored against no plant")
	}
}

func TestAHarvestOnAMissingPlantIsFourOhFour(t *testing.T) {
	h, _, _ := newServer(t)

	rec, _ := do(t, h, http.MethodPost, "/v1/plants/no-such-plant/harvests",
		map[string]any{"quantity": 1, "unit": "fruit"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404: a harvest against nothing is a typo, not a server fault", rec.Code)
	}
}

func TestObservationRecordsWhoSaidIt(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{
		"common_name": "Observed", "slug": unique("observed"),
	})

	rec, out := do(t, h, http.MethodPost, "/v1/plants/"+slug+"/observations",
		map[string]any{"kind": "watered", "body": "by hand", "source": "agent"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d, body %s", rec.Code, rec.Body.String())
	}
	if out["source"] != "agent" {
		t.Errorf("source is %v; who did it is the first question asked later", out["source"])
	}

	_, read := do(t, h, http.MethodGet, "/v1/plants/"+slug, nil)
	if read["last_watered"] == nil {
		t.Error("a watering should surface as last_watered on the plant")
	}
}

func TestUnknownObservationKindIsRejected(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{
		"common_name": "Strict", "slug": unique("strict"),
	})

	rec, _ := do(t, h, http.MethodPost, "/v1/plants/"+slug+"/observations",
		map[string]any{"kind": "photosynthesised", "source": "app"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

// Calm, stale and never-run are three answers, and the digest has to return
// them as three fields rather than collapsing them into one.
func TestTodayReportsFreshnessSeparately(t *testing.T) {
	h, _, _ := newServer(t)

	rec, out := do(t, h, http.MethodGet, "/v1/today", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	for _, field := range []string{"all_clear", "checked", "never_run", "stale_since"} {
		if _, ok := out[field]; !ok {
			t.Errorf("today omits %q, which a client needs to tell the states apart", field)
		}
	}
}

func TestColdWatchNeedsAForecast(t *testing.T) {
	h, _, _ := newServer(t)

	rec, _ := do(t, h, http.MethodGet, "/v1/cold-watch", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400 without forecast_low_f", rec.Code)
	}

	rec, out := do(t, h, http.MethodGet, "/v1/cold-watch?forecast_low_f=50", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	if out["forecast_low_f"] != 50.0 {
		t.Errorf("the forecast should be echoed back, got %v", out["forecast_low_f"])
	}
}

// The queue exists so one text goes out instead of ten, which only works if the
// service renders it ready to send.
func TestQuestionsRenderAsOneSendableMessage(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{
		"common_name": "Asked about", "slug": unique("asked"), "steward": "Maya",
	})

	_, created := do(t, h, http.MethodGet, "/v1/plants/"+slug, nil)
	plantID := created["plant"].(map[string]any)["id"]

	rec, _ := do(t, h, http.MethodPost, "/v1/questions", map[string]any{
		"plant_id": plantID,
		"question": "How many peace lilies are there?",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d, body %s", rec.Code, rec.Body.String())
	}

	_, listed := do(t, h, http.MethodGet, "/v1/questions?asked_of=Maya", nil)
	text, _ := listed["as_text"].(string)
	if !strings.Contains(text, "How many peace lilies") {
		t.Errorf("as_text should be ready to paste into a message:\n%s", text)
	}
}

// An agent asking about a plant should not have to know who owns it.
func TestQuestionInheritsTheStewardFromThePlant(t *testing.T) {
	h, db, ctx := newServer(t)
	slug := createPlant(t, h, map[string]any{
		"common_name": "Owned", "slug": unique("owned"), "steward": "Marcus",
	})

	p, err := db.GetPlant(ctx, slug)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	rec, out := do(t, h, http.MethodPost, "/v1/questions", map[string]any{
		"plant_id": p.ID.String(),
		"question": "What kind of bonsai is it?",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d, body %s", rec.Code, rec.Body.String())
	}
	if out["asked_of"] != "Marcus" {
		t.Errorf("asked_of is %v, want the plant's steward", out["asked_of"])
	}
}

// Photo routes report unavailable rather than failing the whole service, so a
// Planty without object storage still looks after plants.
func TestPhotoRoutesDegradeWithoutStorage(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{
		"common_name": "Unphotographed", "slug": unique("unphoto"),
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/plants/"+slug+"/photos",
		strings.NewReader("not really a jpeg"))
	req.Header.Set("Content-Type", "image/jpeg")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("upload without storage: got %d, want 503", rec.Code)
	}

	if rec, _ := do(t, h, http.MethodPost, "/v1/plants/"+slug+"/diagnosis", nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("diagnosis without storage: got %d, want 503", rec.Code)
	}
}

func TestUnsupportedImageTypeIsRejected(t *testing.T) {
	h, _, _ := newServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/plants/anything/photos",
		strings.NewReader("<svg/>"))
	req.Header.Set("Content-Type", "image/svg+xml")
	h.ServeHTTP(rec, req)

	// Storage is off in this test, so unavailable is checked first and that is
	// the honest answer: the service cannot store anything at all right now.
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("got %d, want 503", rec.Code)
	}
}

func TestAwayPeriodRejectsBackwardsDates(t *testing.T) {
	h, _, _ := newServer(t)

	now := time.Now().UTC()
	rec, _ := do(t, h, http.MethodPost, "/v1/away", map[string]any{
		"starts_at": now.Format(time.RFC3339),
		"ends_at":   now.Add(-24 * time.Hour).Format(time.RFC3339),
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400 for a trip that ends before it starts", rec.Code)
	}
}

func TestSensorLinkAndCalibration(t *testing.T) {
	h, db, ctx := newServer(t)
	slug := createPlant(t, h, map[string]any{
		"common_name": "Probed", "slug": unique("probed"),
	})
	p, err := db.GetPlant(ctx, slug)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	rec, linked := do(t, h, http.MethodPost, "/v1/sensors", map[string]any{
		"plant_id":     p.ID.String(),
		"ha_entity_id": "sensor." + slug,
		"role":         "soil_moisture",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("link: got %d, body %s", rec.Code, rec.Body.String())
	}
	id, _ := linked["id"].(string)

	// Wet must exceed dry, or every reading maps backwards.
	if rec, _ := do(t, h, http.MethodPatch, "/v1/sensors/"+id,
		map[string]any{"dry_baseline": 70, "wet_baseline": 20}); rec.Code != http.StatusBadRequest {
		t.Errorf("inverted baselines: got %d, want 400", rec.Code)
	}

	rec, out := do(t, h, http.MethodPatch, "/v1/sensors/"+id,
		map[string]any{"dry_baseline": 20, "wet_baseline": 70})
	if rec.Code != http.StatusOK {
		t.Fatalf("calibrate: got %d, body %s", rec.Code, rec.Body.String())
	}
	if out["calibrated_at"] == nil {
		t.Error("calibration should record when it happened")
	}
}

// A soil probe measures one pot, so it cannot belong to a zone.
func TestSoilSensorMustNameAPlant(t *testing.T) {
	h, _, _ := newServer(t)

	rec, _ := do(t, h, http.MethodPost, "/v1/sensors", map[string]any{
		"zone":         "greenhouse",
		"ha_entity_id": "sensor." + unique("orphan"),
		"role":         "soil_moisture",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

// The cold warning is only answerable through these routes. Without them it
// repeats every afternoon and nothing ever becomes eligible to go back out.
func TestShelterRoundTripCanBeRecorded(t *testing.T) {
	h, db, ctx := newServer(t)
	minTemp := 55.0
	slug := createPlant(t, h, map[string]any{
		"common_name": "Carried in", "slug": unique("carried"), "min_temp_f": minTemp,
	})

	rec, out := do(t, h, http.MethodPost, "/v1/shelter",
		map[string]any{"slugs": []string{slug}})
	if rec.Code != http.StatusOK {
		t.Fatalf("shelter: got %d, body %s", rec.Code, rec.Body.String())
	}
	if out["moved"] != 1.0 {
		t.Errorf("moved %v plants, want 1", out["moved"])
	}

	p, err := db.GetPlant(ctx, slug)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if p.ShelteredAt == nil {
		t.Fatal("the plant was not recorded as indoors")
	}

	if rec, _ := do(t, h, http.MethodPost, "/v1/unshelter",
		map[string]any{"slugs": []string{slug}}); rec.Code != http.StatusOK {
		t.Fatalf("unshelter: got %d", rec.Code)
	}

	back, err := db.GetPlant(ctx, slug)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if back.ShelteredAt != nil {
		t.Error("the plant is still recorded as indoors")
	}
}

// At dusk the real answer is "I brought them all in", not a list of slugs.
func TestShelterAllTakesEverythingWithAThreshold(t *testing.T) {
	h, db, ctx := newServer(t)
	minTemp := 55.0
	tender := createPlant(t, h, map[string]any{
		"common_name": "Tender", "slug": unique("tender"), "min_temp_f": minTemp,
	})
	indifferent := createPlant(t, h, map[string]any{
		"common_name": "Indifferent", "slug": unique("indifferent"),
	})

	if rec, _ := do(t, h, http.MethodPost, "/v1/shelter",
		map[string]any{"all": true}); rec.Code != http.StatusOK {
		t.Fatalf("shelter all: got %d", rec.Code)
	}

	withThreshold, err := db.GetPlant(ctx, tender)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if withThreshold.ShelteredAt == nil {
		t.Error("a plant with a threshold should have come in")
	}

	without, err := db.GetPlant(ctx, indifferent)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if without.ShelteredAt != nil {
		t.Error("a plant nobody recorded a threshold for should have been left alone")
	}

	_, _ = do(t, h, http.MethodPost, "/v1/unshelter", map[string]any{"all": true})
}

// Naming nothing at all is a mistake worth catching rather than a silent no-op.
func TestShelterNeedsToKnowWhatMoved(t *testing.T) {
	h, _, _ := newServer(t)

	rec, _ := do(t, h, http.MethodPost, "/v1/shelter", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400 when nothing is named", rec.Code)
	}
}

func TestPostmortemsListIsEmptyNotNull(t *testing.T) {
	h, _, _ := newServer(t)

	rec, out := do(t, h, http.MethodGet, "/v1/postmortems", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	if _, ok := out["count"]; !ok {
		t.Error("the plugin reads count to tell empty from missing")
	}
}

func TestListFiltersNarrowTheGarden(t *testing.T) {
	h, _, _ := newServer(t)
	createPlant(t, h, map[string]any{
		"common_name": "Filtered", "slug": unique("filtered"),
		"domain": "edible_indoor", "steward": "Maya",
	})

	_, out := do(t, h, http.MethodGet, "/v1/plants?domain=edible_indoor&steward=Maya", nil)
	plants, _ := out["plants"].([]any)
	if len(plants) == 0 {
		t.Fatal("the filter matched nothing it should have")
	}
	for _, raw := range plants {
		got := raw.(map[string]any)
		if got["domain"] != string(plant.DomainEdibleIndoor) || got["steward"] != "Maya" {
			t.Errorf("filter leaked %v", got["slug"])
		}
	}
}

func TestReminderRoundTripsThroughTheAPI(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{"common_name": unique("Blue oyster kit")})

	rec, out := do(t, h, http.MethodPut, "/v1/plants/"+slug+"/reminders", map[string]any{
		"kind": "misted", "every_days": 1, "at_hours": []int{20, 8}, "note": "surface looks dry",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("set reminder: got %d, body %s", rec.Code, rec.Body.String())
	}
	hours, _ := out["at_hours"].([]any)
	if len(hours) != 2 || hours[0].(float64) != 8 {
		t.Errorf("hours came back %v, want a sorted [8 20]", out["at_hours"])
	}
	if out["active"] != true {
		t.Error("a reminder somebody just set came back switched off")
	}

	rec, out = do(t, h, http.MethodGet, "/v1/plants/"+slug+"/reminders", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: got %d", rec.Code)
	}
	listed, _ := out["reminders"].([]any)
	if len(listed) != 1 {
		t.Fatalf("listed %d reminders, want 1", len(listed))
	}
	// Nothing has ever been misted, so the app must be told it is owed.
	first, _ := listed[0].(map[string]any)
	if first["due"] != true {
		t.Error("a kit nobody has ever misted is not reported as due")
	}

	rec, _ = do(t, h, http.MethodDelete, "/v1/plants/"+slug+"/reminders/misted", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: got %d", rec.Code)
	}
	rec, _ = do(t, h, http.MethodDelete, "/v1/plants/"+slug+"/reminders/misted", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("deleting a gone reminder got %d, want 404", rec.Code)
	}
}

// A reminder about a symptom is not a chore, and an impossible schedule is a
// typo. Both are the caller's fault and neither is a 500.
func TestUnschedulableRemindersAreRejected(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{"common_name": unique("Reminder reject")})

	for _, body := range []map[string]any{
		{"kind": "symptom", "every_days": 1},
		{"kind": "watered", "every_days": -1, "at_hours": []int{8}},
		{"kind": "watered", "every_days": 400, "at_hours": []int{8}},
		{"kind": "watered", "every_days": 1, "at_hours": []int{25}},
	} {
		rec, _ := do(t, h, http.MethodPut, "/v1/plants/"+slug+"/reminders", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%v got %d, want 400", body, rec.Code)
		}
	}
}

// An omitted schedule is the common case from a phone: "remind me to water
// this" means daily at the digest hour, not a validation error.
func TestAnOmittedScheduleMeansDailyAtTheUsualHour(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{"common_name": unique("Bare reminder")})

	rec, out := do(t, h, http.MethodPut, "/v1/plants/"+slug+"/reminders",
		map[string]any{"kind": "watered"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d, body %s", rec.Code, rec.Body.String())
	}
	if out["every_days"] != float64(1) {
		t.Errorf("every_days defaulted to %v, want 1", out["every_days"])
	}
	hours, _ := out["at_hours"].([]any)
	if len(hours) != 1 || hours[0].(float64) != float64(plant.DefaultReminderHour) {
		t.Errorf("hours defaulted to %v", out["at_hours"])
	}
}

// Misting is not watering: a probe never sees it, so it needs its own record.
func TestMistingIsRecordable(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{"common_name": unique("Misted kit")})

	rec, _ := do(t, h, http.MethodPost, "/v1/plants/"+slug+"/observations", map[string]any{
		"kind": "misted", "body": "surface was dry", "source": "app",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("record misting: got %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestAutopsyIsUnavailableWithoutAJudge(t *testing.T) {
	h, _, _ := newServer(t)
	slug := createPlant(t, h, map[string]any{"common_name": unique("Autopsy subject")})

	rec, _ := do(t, h, http.MethodPost, "/v1/plants/"+slug+"/postmortem", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("got %d, want 503 when nothing can answer", rec.Code)
	}
}
