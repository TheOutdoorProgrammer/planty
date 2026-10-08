package judge

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Exercise the pinned native host without a model or subscription credential.
// CLI version/help checks cannot detect a missing or unusable tool runtime.
func TestCodexCodeModeHostRuntime(t *testing.T) {
	host := os.Getenv("PLANTY_TEST_CODEX_HOST")
	if host == "" {
		t.Skip("set PLANTY_TEST_CODEX_HOST to the packaged code-mode host")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, host)
	cmd.Env = []string{"HOME=" + t.TempDir()}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	send := func(message string) {
		t.Helper()
		if err := binary.Write(stdin, binary.LittleEndian, uint32(len(message))); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(stdin, message); err != nil {
			t.Fatal(err)
		}
	}
	receive := func() map[string]any {
		t.Helper()
		var size uint32
		if err := binary.Read(stdout, binary.LittleEndian, &size); err != nil {
			t.Fatal(err)
		}
		if size > 1<<20 {
			t.Fatalf("unexpected frame size %d", size)
		}
		data := make([]byte, size)
		if _, err := io.ReadFull(stdout, data); err != nil {
			t.Fatal(err)
		}
		var message map[string]any
		if err := json.Unmarshal(data, &message); err != nil {
			t.Fatal(err)
		}
		return message
	}
	send(`{"type":"connection/hello","supportedVersions":[1],"requiredCapabilities":[],"optionalCapabilities":[]}`)
	if message := receive(); message["type"] != "connection/ready" || message["selectedVersion"] != float64(1) {
		t.Fatalf("host handshake failed: %v", message)
	}
	send(`{"type":"operation/request","id":1,"request":{"method":"session/open","sessionId":"planty-ci"}}`)
	if message := receive(); message["type"] != "operation/response" {
		t.Fatalf("session open failed: %v", message)
	} else if result, _ := message["result"].(map[string]any); result["status"] != "ok" {
		t.Fatalf("session open failed: %v", message)
	}
	send(`{"type":"operation/request","id":2,"request":{"method":"session/execute","sessionId":"planty-ci","request":{"tool_call_id":"probe","enabled_tools":[],"source":"text(6 * 7)","yield_time_ms":1000,"max_output_tokens":100}}}`)
	for range 8 {
		message := receive()
		if message["type"] != "execute/initialResponse" {
			continue
		}
		data, err := json.Marshal(message["result"])
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Status string `json:"status"`
			Value  struct {
				Result *struct {
					ContentItems []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content_items"`
				} `json:"Result"`
			} `json:"value"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		if result.Status == "ok" && result.Value.Result != nil {
			for _, item := range result.Value.Result.ContentItems {
				if item.Type == "input_text" && item.Text == "42" {
					return
				}
			}
		}
		t.Fatalf("runtime did not evaluate the probe: %s", data)
	}
	t.Fatal("host did not return an execution result")
}
