package judge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeCodex(t *testing.T, scenario string) *codexBackend {
	t.Helper()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "scenario"), []byte(scenario), 0600); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "codex")
	script := "#!/bin/sh\nexec '" + strings.ReplaceAll(exe, "'", "'\\''") + "' -test.run=^TestCodexHelperProcess$ -- \"$@\"\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return &codexBackend{binary: binary, home: home, model: AstraModel}
}

func TestCodexHelperProcess(t *testing.T) {
	if len(os.Args) < 3 || os.Args[2] != "--" {
		return
	}
	defer os.Exit(0)
	scenario, _ := os.ReadFile(filepath.Join(os.Getenv("CODEX_HOME"), "scenario"))
	if os.Args[3] == "--version" {
		if string(scenario) == "old_version" {
			fmt.Println("codex-cli 0.100.0")
		} else {
			fmt.Println("codex-cli " + codexVersion)
		}
		return
	}
	if os.Getenv("PLANTY_DATABASE_URL") != "" || os.Getenv("OPENAI_API_KEY") != "" || os.Getenv("ANTHROPIC_API_KEY") != "" {
		os.Exit(2)
	}
	if string(scenario) == "hang" {
		_ = os.WriteFile(filepath.Join(os.Getenv("CODEX_HOME"), "started"), []byte(fmt.Sprint(os.Getpid())), 0600)
		time.Sleep(time.Minute)
		return
	}
	enc := json.NewEncoder(os.Stdout)
	send := func(v any) {
		if enc.Encode(v) != nil {
			os.Exit(3)
		}
	}
	scan := bufio.NewScanner(os.Stdin)
	scan.Buffer(make([]byte, 65536), 8<<20)
	for scan.Scan() {
		var m codexMessage
		if json.Unmarshal(scan.Bytes(), &m) != nil {
			os.Exit(4)
		}
		result := any(map[string]any{})
		switch m.Method {
		case "initialized":
			continue
		case "initialize":
		case "account/read":
			kind := "chatgpt"
			if string(scenario) == "api_key" {
				kind = "apiKey"
			}
			result = map[string]any{"account": map[string]string{"type": kind}}
		case "thread/start":
			_ = os.WriteFile(filepath.Join(os.Getenv("CODEX_HOME"), "thread.json"), m.Params, 0600)
			model := AstraModel
			if string(scenario) == "wrong_model" {
				model = "different"
			}
			result = map[string]any{"thread": map[string]string{"id": "thread"}, "model": model, "modelProvider": "openai"}
		case "thread/inject_items":
			_ = os.WriteFile(filepath.Join(os.Getenv("CODEX_HOME"), "history.json"), m.Params, 0600)
		case "turn/start":
			_ = os.WriteFile(filepath.Join(os.Getenv("CODEX_HOME"), "turn.json"), m.Params, 0600)
			result = map[string]any{"turn": map[string]string{"id": "turn"}}
			if string(scenario) == "quota" {
				send(map[string]any{"id": m.ID, "result": result})
				send(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "thread", "turn": map[string]any{"id": "turn", "status": "failed", "error": map[string]any{"message": "PRIVATE DETAIL", "codexErrorInfo": "usageLimitExceeded"}}}})
				continue
			}
			if string(scenario) == "tools" || string(scenario) == "refused" {
				send(map[string]any{"id": m.ID, "result": result})
				args := map[string]any{"index": 0}
				name := "historical_photo"
				if string(scenario) == "refused" {
					name = "planty_agent"
					args = map[string]any{"command": "planty agent water"}
				}
				call := map[string]any{"id": "server-call", "method": "item/tool/call", "params": map[string]any{"threadId": "thread", "turnId": "turn", "callId": "call", "tool": name, "arguments": args}}
				send(call)
				send(call)
				continue
			}
			if string(scenario) == "unexpected" {
				send(map[string]any{"id": m.ID, "result": result})
				send(map[string]any{"id": "approval", "method": "item/commandExecution/requestApproval", "params": map[string]any{"threadId": "thread", "turnId": "turn"}})
				continue
			}
			// Completion can race the acknowledgement; neither may be dropped.
			send(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": "thread", "turnId": "turn", "item": map[string]string{"type": "agentMessage", "phase": "final_answer", "text": `{"answer":"ok"}`}}})
			send(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "thread", "turn": map[string]string{"id": "turn", "status": "completed"}}})
		case "":
			file := filepath.Join(os.Getenv("CODEX_HOME"), "tool.json")
			if _, err := os.Stat(file); os.IsNotExist(err) {
				_ = os.WriteFile(file, m.Result, 0600)
			}
			send(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": "thread", "turnId": "turn", "item": map[string]string{"type": "agentMessage", "text": `{"answer":"ok"}`}}})
			send(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "thread", "turn": map[string]string{"id": "turn", "status": "completed"}}})
			continue
		}
		send(map[string]any{"id": m.ID, "result": result})
	}
}

func TestCodexSubscriptionProtocolAndIsolation(t *testing.T) {
	t.Setenv("PLANTY_DATABASE_URL", "must-not-reach-model")
	t.Setenv("OPENAI_API_KEY", "must-not-select-metered-billing")
	t.Setenv("ANTHROPIC_API_KEY", "must-not-reach-model")
	b := fakeCodex(t, "success")
	req := Request{System: "plant care", Schema: probeSchema, Effort: EffortMedium,
		Turns: []Turn{ask(text("before")), answered("prior"), ask(picture("image/png", greenSquare(t)), text("now"))}}
	out, err := b.Judge(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if out.Model != AstraModel || decodeAnswer(t, out) != "ok" {
		t.Fatalf("bad outcome: %+v", out)
	}
	var params map[string]any
	raw, _ := os.ReadFile(filepath.Join(b.home, "thread.json"))
	if json.Unmarshal(raw, &params) != nil {
		t.Fatal("no thread request")
	}
	for _, key := range []string{"environments", "runtimeWorkspaceRoots", "selectedCapabilityRoots"} {
		if value, ok := params[key].([]any); !ok || len(value) != 0 {
			t.Fatalf("%s must be explicit empty array: %s", key, raw)
		}
	}
	if params["allowProviderModelFallback"] != false || params["model"] != AstraModel || params["ephemeral"] != true {
		t.Fatalf("unsafe thread: %s", raw)
	}
	cfg := params["config"].(map[string]any)
	if cfg["forced_login_method"] != "chatgpt" || cfg["agents.enabled"] != false || cfg["features.multi_agent_v2"] != false {
		t.Fatal("unsafe config")
	}
	raw, _ = os.ReadFile(filepath.Join(b.home, "turn.json"))
	_ = json.Unmarshal(raw, &params)
	actualSchema, _ := json.Marshal(params["outputSchema"])
	expectedSchema, _ := json.Marshal(probeSchema)
	if string(actualSchema) != string(expectedSchema) || params["effort"] != "medium" || !strings.Contains(string(raw), "data:image/png;base64,") {
		t.Fatalf("lost request data: %s", raw)
	}
	raw, _ = os.ReadFile(filepath.Join(b.home, "history.json"))
	if !strings.Contains(string(raw), "output_text") || !strings.Contains(string(raw), "prior") {
		t.Fatalf("lost history: %s", raw)
	}
}

func TestCodexFailsClosed(t *testing.T) {
	for _, scenario := range []string{"api_key", "wrong_model", "old_version", "unexpected", "quota"} {
		t.Run(scenario, func(t *testing.T) {
			b := fakeCodex(t, scenario)
			_, err := b.Judge(context.Background(), Request{Turns: []Turn{ask(text("probe"))}})
			if err == nil {
				t.Fatal("unsafe request succeeded")
			}
			if scenario == "quota" && (!errors.Is(err, ErrQuotaExhausted) || Retryable(err)) {
				t.Fatalf("quota classification: %v", err)
			}
			if strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("provider detail leaked")
			}
			unlock, lockErr := lockCodex(context.Background(), b.home)
			if lockErr != nil {
				t.Fatal(lockErr)
			}
			unlock()
		})
	}
}

func TestCodexAlreadyCancelledDoesNotAcquireAuth(t *testing.T) {
	b := fakeCodex(t, "success")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := b.Judge(ctx, Request{Turns: []Turn{ask(text("probe"))}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(b.home, "planty.lock")); !os.IsNotExist(err) {
		t.Fatal("cancelled request touched auth")
	}
}

func TestToolboxLiveFlagIsRequestScoped(t *testing.T) {
	t.Setenv("PLANTY_CHAT", "inherited")
	binary := filepath.Join(t.TempDir(), "planty")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s' \"${PLANTY_CHAT:-scheduled}\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, live := range []bool{true, false} {
		box := newToolbox(&Acting{Binary: binary, Refuse: func(string, []string) string { return "" }})
		box.live = live
		want := "scheduled"
		if live {
			want = "1"
		}
		if got := box.runAgent(context.Background(), "planty agent today"); got != want {
			t.Errorf("live=%v got %q", live, got)
		}
	}
}

func TestCodexToolsReuseGatesAndDeduplicate(t *testing.T) {
	for _, scenario := range []string{"tools", "refused"} {
		t.Run(scenario, func(t *testing.T) {
			b := fakeCodex(t, scenario)
			calls := 0
			out, err := b.Judge(context.Background(), Request{Turns: []Turn{ask(text("probe"))},
				Offered: []Offer{{Label: "historical", Media: "image/png", Bytes: greenSquare(t)}},
				Acting:  &Acting{Binary: "/does/not/exist", Refuse: func(string, []string) string { calls++; return "no watering" }},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Steps) != 1 {
				t.Fatalf("duplicate tool ran: %+v", out.Steps)
			}
			raw, _ := os.ReadFile(filepath.Join(b.home, "tool.json"))
			if scenario == "tools" && !strings.Contains(string(raw), "inputImage") {
				t.Fatal("missing photo result")
			}
			if scenario == "refused" && (calls != 1 || !strings.Contains(string(raw), "Refused: no watering")) {
				t.Fatalf("gate bypass: %s", raw)
			}
		})
	}
}

func TestCodexCancellationReapsProcessAndUnlocks(t *testing.T) {
	b := fakeCodex(t, "hang")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := b.Judge(ctx, Request{Turns: []Turn{ask(text("probe"))}}); done <- err }()
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(filepath.Join(b.home, "started")); err == nil {
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("child did not start")
		case <-ticker.C:
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("child did not exit")
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	unlock, err := lockCodex(ctx, b.home)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}

func TestCodexAuthLockAcrossProcesses(t *testing.T) {
	if os.Getenv("PLANTY_LOCK_CHILD") != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, err := lockCodex(ctx, os.Getenv("PLANTY_LOCK_CHILD"))
		if !errors.Is(err, context.DeadlineExceeded) {
			os.Exit(3)
		}
		os.Exit(0)
	}
	home := t.TempDir()
	unlock, err := lockCodex(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	exe, _ := os.Executable()
	cmd := exec.Command(exe, "-test.run=^TestCodexAuthLockAcrossProcesses$")
	cmd.Env = append(os.Environ(), "PLANTY_LOCK_CHILD="+home)
	if err := cmd.Run(); err != nil {
		t.Fatalf("cross-process exclusion failed: %v", err)
	}
}
