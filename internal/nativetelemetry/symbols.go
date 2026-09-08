package nativetelemetry

import (
	"bytes"
	"context"
	"debug/macho"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	common "go.opentelemetry.io/proto/otlp/common/v1"
)

const MaxSymbolBytes = 64 << 20
const machOTypeDSYM macho.Type = 0xa

var ErrInvalidSymbols = errors.New("invalid native symbols")

type ObjectStore interface {
	Get(context.Context, string) (io.ReadCloser, error)
}

type Symbolizer struct {
	Store   ObjectStore
	Command string
}

func NewSymbolizer(store ObjectStore) *Symbolizer {
	return &Symbolizer{Store: store, Command: "llvm-symbolizer"}
}

func SymbolKey(imageUUID, architecture string) (string, error) {
	id, err := uuid.Parse(imageUUID)
	if err != nil || len(imageUUID) != 36 || id == uuid.Nil {
		return "", ErrInvalidSymbols
	}
	switch architecture {
	case "arm64", "arm64e", "x86_64":
	default:
		return "", ErrInvalidSymbols
	}
	return "native-symbols/" + id.String() + "/" + architecture + "/symbols.dwarf", nil
}

func ValidateSymbols(data []byte, imageUUID, architecture string) error {
	_, _, err := inspectSymbols(data, imageUUID, architecture)
	return err
}

func inspectSymbols(data []byte, imageUUID, architecture string) (uint64, uint64, error) {
	if _, err := SymbolKey(imageUUID, architecture); err != nil {
		return 0, 0, err
	}
	metadata, err := readSymbols(data)
	if err != nil || metadata.UUID != uuid.MustParse(imageUUID).String() || metadata.Architecture != architecture {
		return 0, 0, ErrInvalidSymbols
	}
	return metadata.Base, metadata.Size, nil
}

func InspectSymbols(data []byte) (imageUUID, architecture string, err error) {
	metadata, err := readSymbols(data)
	return metadata.UUID, metadata.Architecture, err
}

type symbolMetadata struct {
	UUID         string
	Architecture string
	Base         uint64
	Size         uint64
}

func readSymbols(data []byte) (symbolMetadata, error) {
	var metadata symbolMetadata
	if len(data) == 0 || len(data) > MaxSymbolBytes {
		return metadata, ErrInvalidSymbols
	}
	file, err := macho.NewFile(bytes.NewReader(data))
	if err != nil {
		return metadata, ErrInvalidSymbols
	}
	defer func() { _ = file.Close() }()
	switch file.Cpu {
	case macho.CpuArm64:
		switch file.SubCpu & 0x00ffffff {
		case 0:
			metadata.Architecture = "arm64"
		case 2:
			metadata.Architecture = "arm64e"
		}
	case macho.CpuAmd64:
		if file.SubCpu&0x00ffffff == 3 {
			metadata.Architecture = "x86_64"
		}
	}
	if metadata.Architecture == "" || file.Type != machOTypeDSYM {
		return metadata, ErrInvalidSymbols
	}
	for _, load := range file.Loads {
		raw := load.Raw()
		if len(raw) == 24 && file.ByteOrder.Uint32(raw[:4]) == 0x1b {
			id, err := uuid.FromBytes(raw[8:24])
			if err != nil || id == uuid.Nil || metadata.UUID != "" {
				return metadata, ErrInvalidSymbols
			}
			metadata.UUID = id.String()
		}
	}
	text := file.Segment("__TEXT")
	if metadata.UUID == "" || text == nil || text.Memsz == 0 || text.Addr+text.Memsz < text.Addr {
		return metadata, ErrInvalidSymbols
	}
	if _, err := file.DWARF(); err != nil {
		return metadata, ErrInvalidSymbols
	}
	metadata.Base, metadata.Size = text.Addr, text.Memsz
	return metadata, nil
}

func (s *Symbolizer) Resolve(ctx context.Context, crash Crash) ([]ResolvedFrame, error) {
	if s.Store == nil {
		return nil, errors.New("native symbol store unavailable")
	}
	key, err := SymbolKey(crash.ImageUUID, crash.Architecture)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	reader, err := s.Store.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	data, err := io.ReadAll(io.LimitReader(reader, MaxSymbolBytes+1))
	if err != nil {
		return nil, err
	}
	base, size, err := inspectSymbols(data, crash.ImageUUID, crash.Architecture)
	if err != nil {
		return nil, err
	}
	if len(crash.Frames) == 0 || len(crash.Frames) > 64 {
		return nil, ErrInvalidSymbols
	}
	var addresses strings.Builder
	for _, frame := range crash.Frames {
		if frame.Offset >= size {
			return nil, ErrInvalidSymbols
		}
		fmt.Fprintf(&addresses, "0x%x\n", base+frame.Offset)
	}
	file, err := os.CreateTemp("", "planty-native-symbols-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, s.Command, "--obj="+file.Name(), "--default-arch="+crash.Architecture, "--output-style=JSON", "--basenames", "--no-inlines", "--no-debuginfod")
	command.Env = append(os.Environ(), "LLVM_SYMBOLIZER_OPTS=", "DEBUGINFOD_URLS=")
	command.WaitDelay = time.Second
	command.Stdin = strings.NewReader(addresses.String())
	output := &boundedOutput{limit: 256 << 10}
	command.Stdout = output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return nil, errors.New("native symbolication failed")
	}
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	frames := make([]ResolvedFrame, 0, len(crash.Frames))
	for range crash.Frames {
		var result struct {
			Symbol []struct {
				FunctionName string
				FileName     string
				Line         int64
			}
		}
		if err := decoder.Decode(&result); err != nil || len(result.Symbol) != 1 {
			return nil, errors.New("native symbolication incomplete")
		}
		symbol := result.Symbol[0]
		fileName := filepath.Base(symbol.FileName)
		if !safeSymbol(symbol.FunctionName, 512) || symbol.FunctionName == "??" || !safeSymbol(fileName, 128) || fileName == "??" || fileName == "." || symbol.Line <= 0 || symbol.Line > 10000000 {
			return nil, errors.New("native symbolication unresolved")
		}
		frames = append(frames, ResolvedFrame{Function: symbol.FunctionName, File: fileName, Line: symbol.Line})
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("native symbolication unexpected output")
	}
	return frames, nil
}

func safeSymbol(value string, limit int) bool {
	return value != "" && len(value) <= limit && strings.IndexFunc(value, unicode.IsControl) == -1
}

type boundedOutput struct {
	bytes.Buffer
	limit int
}

func (output *boundedOutput) Write(data []byte) (int, error) {
	if len(data) > output.limit-output.Len() {
		return 0, errors.New("symbolicator output exceeds limit")
	}
	return output.Buffer.Write(data)
}

func (r *Relay) crashAttributes(ctx context.Context, crash Crash) []*common.KeyValue {
	attributes := []*common.KeyValue{
		textAttribute("crash.image.uuid", uuid.MustParse(crash.ImageUUID).String()),
		textAttribute("crash.image.architecture", crash.Architecture),
	}
	offsets := make([]string, len(crash.Frames))
	for i, frame := range crash.Frames {
		offsets[i] = "0x" + strconv.FormatUint(frame.Offset, 16)
	}
	attributes = append(attributes, textAttribute("crash.frame.offsets", strings.Join(offsets, ",")))
	if r.symbols != nil {
		frames, err := r.symbols.Resolve(ctx, crash)
		if err == nil && len(frames) == len(crash.Frames) && len(frames) > 0 && len(frames) <= 64 {
			var stack strings.Builder
			for _, frame := range frames {
				if !safeSymbol(frame.Function, 512) || !safeSymbol(filepath.Base(frame.File), 128) || frame.Line <= 0 || frame.Line > 10000000 {
					return append(attributes, textAttribute("symbolication.status", "unresolved"))
				}
				fmt.Fprintf(&stack, "%s (%s:%d)\n", frame.Function, filepath.Base(frame.File), frame.Line)
			}
			return append(attributes, textAttribute("symbolication.status", "resolved"), textAttribute("exception.stacktrace", stack.String()))
		}
	}
	return append(attributes, textAttribute("symbolication.status", "unresolved"))
}
