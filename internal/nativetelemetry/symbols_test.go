package nativetelemetry

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureUUID = "a185c3af-a04b-37e6-89cc-f1c53f078adf"

func fixtureSymbols(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/symbols.dwarf")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSymbolsValidateRealMachOIdentityAndBounds(t *testing.T) {
	data := fixtureSymbols(t)
	id, arch, err := InspectSymbols(data)
	if err != nil || id != fixtureUUID || arch != "arm64" {
		t.Fatalf("fixture identity = %s %s %v", id, arch, err)
	}
	if err := ValidateSymbols(data, strings.ToUpper(fixtureUUID), "arm64"); err != nil {
		t.Fatal(err)
	}
	for _, identity := range [][2]string{{"00000000-0000-0000-0000-000000000000", "arm64"}, {"8d6c4d58-85dc-4b1f-bd26-8d20319b646c", "arm64"}, {fixtureUUID, "arm64e"}, {fixtureUUID, "x86_64"}, {"../../secret", "arm64"}} {
		if ValidateSymbols(data, identity[0], identity[1]) == nil {
			t.Fatal("mismatched symbol identity accepted")
		}
	}
	for name, mutate := range map[string]func([]byte) []byte{
		"truncated":       func(data []byte) []byte { return data[:100] },
		"oversize":        func([]byte) []byte { return make([]byte, MaxSymbolBytes+1) },
		"fat":             func(data []byte) []byte { binary.BigEndian.PutUint32(data[:4], 0xcafebabe); return data },
		"executable":      func(data []byte) []byte { binary.LittleEndian.PutUint32(data[12:16], 2); return data },
		"unsupported cpu": func(data []byte) []byte { binary.LittleEndian.PutUint32(data[4:8], 0); return data },
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := InspectSymbols(mutate(bytes.Clone(data))); !errors.Is(err, ErrInvalidSymbols) {
				t.Fatalf("invalid symbols accepted: %v", err)
			}
		})
	}
}

type fixtureStore struct {
	data       []byte
	key        string
	missingKey string
}

func (s *fixtureStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	s.key = key
	if key == s.missingKey {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(s.data)), nil
}

func TestSymbolizerRejectsMismatchedImageBeforeExecuting(t *testing.T) {
	symbolizer := &Symbolizer{Store: &fixtureStore{data: fixtureSymbols(t)}, Command: "must-never-run"}
	for _, crash := range []Crash{
		{ImageUUID: fixtureUUID, Architecture: "x86_64", Frames: []Frame{{Offset: 0x328}}},
		{ImageUUID: fixtureUUID, Architecture: "arm64", Frames: []Frame{{Offset: 1 << 39}}},
		{ImageUUID: fixtureUUID, Architecture: "arm64"},
	} {
		if _, err := symbolizer.Resolve(context.Background(), crash); !errors.Is(err, ErrInvalidSymbols) {
			t.Fatalf("mismatch reached subprocess: %v", err)
		}
	}
}

func TestArchitectureFallbackStillRequiresExactImageUUID(t *testing.T) {
	const differentUUID = "8d6c4d58-85dc-4b1f-bd26-8d20319b646c"
	store := &fixtureStore{
		data:       fixtureSymbols(t),
		missingKey: "native-symbols/" + differentUUID + "/arm64e/symbols.dwarf",
	}
	symbolizer := &Symbolizer{Store: store, Command: "must-never-run"}
	_, err := symbolizer.Resolve(context.Background(), Crash{
		ImageUUID: differentUUID, Architecture: "arm64e", Frames: []Frame{{Offset: 0x328}},
	})
	if !errors.Is(err, ErrInvalidSymbols) || store.key != "native-symbols/"+differentUUID+"/arm64/symbols.dwarf" {
		t.Fatalf("fallback accepted another image: %s %v", store.key, err)
	}
}

func TestLLVMSymbolizerResolvesRealDSYM(t *testing.T) {
	command := testSymbolizerCommand(t)
	store := &fixtureStore{data: fixtureSymbols(t)}
	symbolizer := &Symbolizer{Store: store, Command: command}
	frames, err := symbolizer.Resolve(context.Background(), Crash{ImageUUID: fixtureUUID, Architecture: "arm64", Frames: []Frame{{Offset: 0x328}, {Offset: 0x328}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 || frames[0] != (ResolvedFrame{Function: "native_crash_site", File: "symbols.c", Line: 1}) || frames[1] != frames[0] {
		t.Fatalf("unexpected symbolication: %#v", frames)
	}
	if store.key != "native-symbols/"+fixtureUUID+"/arm64/symbols.dwarf" {
		t.Fatal("symbol lookup escaped private namespace")
	}
	store.missingKey = "native-symbols/" + fixtureUUID + "/arm64e/symbols.dwarf"
	frames, err = symbolizer.Resolve(context.Background(), Crash{ImageUUID: fixtureUUID, Architecture: "arm64e", Frames: []Frame{{Offset: 0x328}}})
	if err != nil || len(frames) != 1 || frames[0].Function != "native_crash_site" || store.key != "native-symbols/"+fixtureUUID+"/arm64/symbols.dwarf" {
		t.Fatalf("platform architecture did not resolve the exact app UUID: %#v %v", frames, err)
	}
}

func testSymbolizerCommand(t *testing.T) string {
	t.Helper()
	command, err := exec.LookPath("llvm-symbolizer")
	if image := os.Getenv("PLANTY_TEST_SYMBOLIZER_IMAGE"); image != "" {
		if _, err := exec.LookPath("docker"); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PLANTY_TEST_SYMBOLIZER_IMAGE", image)
		command = filepath.Join(t.TempDir(), "symbolizer")
		wrapper := "#!/bin/sh\nsymbol_file=\"${1#--obj=}\"\nexec docker run --rm -i --entrypoint llvm-symbolizer --user \"$(id -u):$(id -g)\" -v \"$symbol_file:$symbol_file:ro\" \"$PLANTY_TEST_SYMBOLIZER_IMAGE\" \"$@\"\n"
		if err := os.WriteFile(command, []byte(wrapper), 0700); err != nil {
			t.Fatal(err)
		}
	} else if err != nil {
		t.Skip("install llvm-symbolizer or set PLANTY_TEST_SYMBOLIZER_IMAGE for runtime integration")
	}
	return command
}

type fakeResolver []ResolvedFrame

func (r fakeResolver) Resolve(context.Context, Crash) ([]ResolvedFrame, error) { return r, nil }

func TestResolvedFramesExportOnlySafeSymbols(t *testing.T) {
	crash := Crash{ImageUUID: fixtureUUID, Architecture: "arm64", Frames: []Frame{{Offset: 0x328}}}
	for name, frames := range map[string]fakeResolver{
		"basename": {{Function: "native_crash_site", File: "/private/build/symbols.c", Line: 1}},
		"control":  {{Function: "bad\nframe", File: "symbols.c", Line: 1}},
		"large":    {{Function: strings.Repeat("x", 513), File: "symbols.c", Line: 1}},
	} {
		t.Run(name, func(t *testing.T) {
			relay, _ := New("", frames)
			attributes := relay.crashAttributes(context.Background(), crash)
			var stack string
			for _, attribute := range attributes {
				if attribute.Key == "exception.stacktrace" {
					stack = attribute.Value.GetStringValue()
				}
			}
			if name == "basename" && stack != "native_crash_site (symbols.c:1)\n" {
				t.Fatalf("unsafe stack: %q", stack)
			} else if name != "basename" && stack != "" {
				t.Fatal("invalid resolver output exported")
			}
		})
	}
}
