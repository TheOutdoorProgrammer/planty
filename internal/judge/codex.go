package judge

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const AstraModel = "gpt-6-astra"
const codexVersion = "0.154.0"

type codexBackend struct {
	binary string
	home   string
	model  string
}

func (b *codexBackend) Name() string { return "codex subscription" }

func codexIfConfigured(model string) Backend {
	home := os.Getenv("PLANTY_CODEX_HOME")
	if !filepath.IsAbs(home) {
		return nil
	}
	binary := os.Getenv("PLANTY_CODEX_BIN")
	if binary == "" {
		var err error
		binary, err = exec.LookPath("codex")
		if err != nil {
			return nil
		}
	}
	return &codexBackend{binary: binary, home: home, model: model}
}

func (b *codexBackend) Judge(ctx context.Context, req Request) (_ Outcome, retErr error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	ctx, span := otel.Tracer("planty/judge").Start(ctx, "model.codex")
	defer span.End()
	defer func() {
		if retErr != nil {
			span.RecordError(retErr)
			span.SetStatus(codes.Error, "subscription judgment failed")
		}
	}()
	span.SetAttributes(attribute.String("model.provider", "codex"), attribute.String("model.name", modelFor(req, b.model)))
	unlock, err := lockCodex(ctx, b.home)
	if err != nil {
		return Outcome{}, err
	}
	defer unlock()
	dir, err := os.MkdirTemp("", "planty-codex-")
	if err != nil {
		return Outcome{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "CODEX_HOME=" + b.home}
	version := exec.CommandContext(ctx, b.binary, "--version")
	version.Env, version.Dir = env, dir
	installed, err := version.Output()
	if ctx.Err() != nil {
		return Outcome{}, ctx.Err()
	}
	if err != nil || strings.TrimSpace(string(installed)) != "codex-cli "+codexVersion {
		return Outcome{}, permanent(errors.New("codex must match the tested app-server version " + codexVersion))
	}
	cmd := exec.CommandContext(ctx, b.binary, "app-server", "--listen", "stdio://",
		"-c", `forced_login_method="chatgpt"`, "-c", `cli_auth_credentials_store="file"`)
	cmd.Dir = dir
	// The model process needs its subscription login, never Planty's database,
	// Home Assistant credentials, API keys or the operator's personal config.
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Outcome{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Outcome{}, err
	}
	if err := cmd.Start(); err != nil {
		return Outcome{}, fmt.Errorf("start codex: %w", err)
	}
	defer func() {
		_ = cmd.Cancel()
		_ = stdin.Close()
		_ = cmd.Wait()
	}()
	rpc := newCodexRPC(ctx, stdout, stdin)
	return b.converseCodex(ctx, rpc, dir, req)
}

// The auth cache is writable and shared by pods on the same volume. Keep one
// refresh owner for the whole subprocess lifetime, not just its startup.
func lockCodex(ctx context.Context, home string) (func(), error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	f, err := os.OpenFile(filepath.Join(home, "planty.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open codex auth lock: %w", err)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			_ = f.Close()
			return nil, ctx.Err()
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = f.Close()
			return nil, fmt.Errorf("lock codex auth: %w", err)
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

type codexMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

type codexRPC struct {
	writer  *json.Encoder
	in      chan codexMessage
	pending []codexMessage
	id      int
}

var errCodexProtocol = errors.New("codex protocol ended or returned an invalid message")

func newCodexRPC(ctx context.Context, reader io.Reader, writer io.Writer) *codexRPC {
	r := &codexRPC{writer: json.NewEncoder(writer), in: make(chan codexMessage, 1)}
	go func() {
		defer close(r.in)
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 65536), 8<<20)
		for scanner.Scan() {
			var m codexMessage
			if json.Unmarshal(scanner.Bytes(), &m) != nil {
				return
			}
			select {
			case r.in <- m:
			case <-ctx.Done():
				return
			}
		}
	}()
	return r
}

func (r *codexRPC) receive(ctx context.Context) (codexMessage, error) {
	select {
	case m, ok := <-r.in:
		if !ok {
			return m, errCodexProtocol
		}
		return m, nil
	case <-ctx.Done():
		return codexMessage{}, ctx.Err()
	}
}

func (r *codexRPC) call(ctx context.Context, method string, params any, result any) error {
	r.id++
	if err := r.writer.Encode(map[string]any{"id": r.id, "method": method, "params": params}); err != nil {
		return errCodexProtocol
	}
	for {
		m, err := r.receive(ctx)
		if err != nil {
			return err
		}
		if string(m.ID) == fmt.Sprint(r.id) && m.Method == "" {
			if len(m.Error) > 0 && string(m.Error) != "null" {
				return fmt.Errorf("codex rejected %s", method)
			}
			if result != nil && json.Unmarshal(m.Result, result) != nil {
				return errCodexProtocol
			}
			return nil
		}
		if len(r.pending) >= 256 {
			return errCodexProtocol
		}
		r.pending = append(r.pending, m)
	}
}

func (r *codexRPC) next(ctx context.Context) (codexMessage, error) {
	if len(r.pending) > 0 {
		m := r.pending[0]
		r.pending = r.pending[1:]
		return m, nil
	}
	return r.receive(ctx)
}

func (b *codexBackend) converseCodex(ctx context.Context, rpc *codexRPC, dir string, req Request) (Outcome, error) {
	if len(req.Turns) == 0 || req.Turns[len(req.Turns)-1].Role != RoleUser {
		return Outcome{}, errors.New("codex requires a final user turn")
	}
	if err := rpc.call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "planty", "version": "1"},
		"capabilities": map[string]bool{"experimentalApi": true},
	}, nil); err != nil {
		return Outcome{}, err
	}
	if err := rpc.writer.Encode(map[string]string{"method": "initialized"}); err != nil {
		return Outcome{}, errCodexProtocol
	}
	var account struct {
		Account struct {
			Type string `json:"type"`
		} `json:"account"`
	}
	if err := rpc.call(ctx, "account/read", map[string]bool{"refreshToken": true}, &account); err != nil {
		return Outcome{}, err
	}
	if account.Account.Type != "chatgpt" {
		return Outcome{}, permanent(errors.New("codex requires a ChatGPT subscription login"))
	}
	box := newToolbox(req.Acting, req.Offered...)
	box.live = req.Live
	defs := box.definitions()
	tools := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		tools = append(tools, map[string]any{"type": "function", "name": def.Function.Name,
			"description": def.Function.Description, "inputSchema": def.Function.Parameters})
	}
	var thread struct {
		Model    string `json:"model"`
		Provider string `json:"modelProvider"`
		Thread   struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	params := map[string]any{
		"model": modelFor(req, b.model), "modelProvider": "openai", "allowProviderModelFallback": false,
		"cwd": dir, "ephemeral": true, "sandbox": "read-only", "approvalPolicy": "never",
		"environments": []any{}, "runtimeWorkspaceRoots": []any{}, "selectedCapabilityRoots": []any{},
		"baseInstructions": req.System, "dynamicTools": tools,
		"config": codexConfig(),
	}
	if err := rpc.call(ctx, "thread/start", params, &thread); err != nil {
		return Outcome{}, err
	}
	if thread.Thread.ID == "" || thread.Model != modelFor(req, b.model) || thread.Provider != "openai" {
		return Outcome{}, errCodexProtocol
	}
	if len(req.Turns) > 1 {
		var items []map[string]any
		for _, turn := range req.Turns[:len(req.Turns)-1] {
			parts := codexParts(turn.Parts, true)
			if turn.Role == RoleAssistant {
				for _, part := range parts {
					if part["type"] == "input_text" {
						part["type"] = "output_text"
					}
				}
			}
			items = append(items, map[string]any{"type": "message", "role": string(turn.Role), "content": parts})
		}
		if err := rpc.call(ctx, "thread/inject_items", map[string]any{"threadId": thread.Thread.ID, "items": items}, nil); err != nil {
			return Outcome{}, err
		}
	}
	input := codexParts(req.Turns[len(req.Turns)-1].Parts, false)
	if len(req.Offered) > 0 {
		input = append(input, map[string]string{"type": "text", "text": offeredPhotoInstructions(req.Offered)})
	}
	turnParams := map[string]any{"threadId": thread.Thread.ID, "input": input}
	if req.Effort != "" {
		turnParams["effort"] = string(req.Effort)
	}
	if len(req.Schema) > 0 {
		turnParams["outputSchema"] = req.Schema
	}
	var turn struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := rpc.call(ctx, "turn/start", turnParams, &turn); err != nil {
		return Outcome{}, err
	}
	if turn.Turn.ID == "" {
		return Outcome{}, errCodexProtocol
	}
	return codexOutcome(ctx, rpc, thread.Thread.ID, turn.Turn.ID, modelFor(req, b.model), box)
}

func codexConfig() map[string]any {
	return map[string]any{
		"features.shell_tool": false, "features.view_image": false, "features.code_mode": false,
		"features.code_mode_host": false, "features.apps": false, "features.collab": false,
		"features.multi_agent": false, "features.browser_use": false, "features.computer_use": false,
		"agents.enabled": false, "features.multi_agent_v2": false,
		"features.codex_hooks": false, "features.skip_host_skill_discovery": true,
		"features.plugins": false, "features.plugin_hooks": false, "features.hooks": false,
		"features.image_generation": false, "features.imagegenext": false,
		"features.sleep_tool": false, "features.deferred_executor": false,
		"features.memories": false, "features.memory_tool": false,
		"features.default_mode_request_user_input": false,
		"tools.update_plan.enabled":                false, "tools.experimental_request_user_input.enabled": false,
		"web_search": "disabled", "project_doc_max_bytes": 0,
		"forced_login_method": "chatgpt", "mcp_servers": map[string]any{},
	}
}

func codexParts(parts []Part, raw bool) []map[string]string {
	out := make([]map[string]string, 0, len(parts))
	for _, p := range parts {
		if p.Image == nil {
			kind := "text"
			if raw {
				kind = "input_text"
			}
			out = append(out, map[string]string{"type": kind, "text": p.Text})
		} else {
			media := p.Image.Media
			if media == "" {
				media = "image/jpeg"
			}
			kind, key := "image", "url"
			if raw {
				kind, key = "input_image", "image_url"
			}
			out = append(out, map[string]string{"type": kind, key: "data:" + media + ";base64," + base64.StdEncoding.EncodeToString(p.Image.Bytes)})
		}
	}
	return out
}

func codexOutcome(ctx context.Context, rpc *codexRPC, threadID, turnID, model string, box *toolbox) (Outcome, error) {
	out := Outcome{Model: model}
	seen := map[string]any{}
	seenCalls := map[string]string{}
	allowed := map[string]bool{}
	for _, def := range box.definitions() {
		allowed[def.Function.Name] = true
	}
	for {
		m, err := rpc.next(ctx)
		if err != nil {
			return Outcome{}, err
		}
		var event struct {
			ThreadID  string          `json:"threadId"`
			TurnID    string          `json:"turnId"`
			CallID    string          `json:"callId"`
			Tool      string          `json:"tool"`
			Namespace string          `json:"namespace"`
			Arguments json.RawMessage `json:"arguments"`
			Item      struct {
				Type  string `json:"type"`
				Text  string `json:"text"`
				Phase string `json:"phase"`
			} `json:"item"`
			Turn struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Error  *struct {
					Info json.RawMessage `json:"codexErrorInfo"`
				} `json:"error"`
			} `json:"turn"`
		}
		if json.Unmarshal(m.Params, &event) != nil {
			return Outcome{}, errCodexProtocol
		}
		if len(m.ID) > 0 {
			if m.Method != "item/tool/call" || event.ThreadID != threadID || event.TurnID != turnID || event.Namespace != "" || event.CallID == "" {
				return Outcome{}, errors.New("codex requested an ungranted operation")
			}
			if !allowed[event.Tool] || !json.Valid(event.Arguments) {
				return Outcome{}, errors.New("codex requested an ungranted tool")
			}
			identity := event.Tool + ":" + string(event.Arguments)
			if prior, ok := seenCalls[event.CallID]; ok && prior != identity {
				return Outcome{}, errCodexProtocol
			}
			result, exists := seen[event.CallID]
			if !exists {
				if len(seen) >= rounds {
					return Outcome{}, errors.New("codex exceeded the tool call limit")
				}
				call := toolCall{ID: event.CallID, Type: "function"}
				call.Function.Name, call.Function.Arguments = event.Tool, string(event.Arguments)
				value := box.run(ctx, call)
				content := []map[string]string{{"type": "inputText", "text": value.Content}}
				for _, part := range value.Images {
					if part.ImageURL != nil {
						content = append(content, map[string]string{"type": "inputImage", "imageUrl": part.ImageURL.URL})
					}
				}
				result = map[string]any{"success": true, "contentItems": content}
				seen[event.CallID] = result
				seenCalls[event.CallID] = identity
				out.addStep(Step{Kind: StepAction, Tool: event.Tool, Detail: describeCall(call), Output: clip(value.Summary)})
			}
			if err := rpc.writer.Encode(map[string]any{"id": m.ID, "result": result}); err != nil {
				return Outcome{}, errCodexProtocol
			}
			continue
		}
		if event.ThreadID != threadID {
			continue
		}
		switch m.Method {
		case "item/completed":
			if event.TurnID == turnID && event.Item.Type == "agentMessage" && event.Item.Phase != "commentary" {
				out.Answer = event.Item.Text
			}
		case "turn/completed":
			if event.Turn.ID != turnID {
				continue
			}
			if event.Turn.Error != nil {
				var code string
				_ = json.Unmarshal(event.Turn.Error.Info, &code)
				if code == "usageLimitExceeded" {
					return Outcome{}, permanent(fmt.Errorf("codex: %w", ErrQuotaExhausted))
				}
				switch code {
				case "unauthorized", "badRequest":
					return Outcome{}, permanent(fmt.Errorf("codex turn failed: %s", code))
				case "contextWindowExceeded", "rateLimitExceeded", "serverOverloaded":
					return Outcome{}, fmt.Errorf("codex turn failed: %s", code)
				default:
					return Outcome{}, errors.New("codex turn failed")
				}
			}
			if event.Turn.Status != "completed" || strings.TrimSpace(out.Answer) == "" {
				return Outcome{}, errors.New("codex returned no completed answer")
			}
			return out, nil
		}
	}
}
