package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/api"
	"github.com/TheOutdoorProgrammer/planty/internal/nativesymbols"
	"github.com/TheOutdoorProgrammer/planty/internal/nativetelemetry"
)

func configureNativeSymbols(ctx context.Context, server *api.Server, storage nativesymbols.Store) {
	repository := os.Getenv("PLANTY_SYMBOLS_REPOSITORY")
	repositoryID := os.Getenv("PLANTY_SYMBOLS_REPOSITORY_ID")
	if repository == "" || repositoryID == "" {
		return
	}
	authorizer := nativesymbols.NewGitHubAuthorizer(ctx, repository, repositoryID)
	server.WithNativeSymbols(nativesymbols.NewHandler(storage, authorizer,
		nativetelemetry.SymbolKey, nativetelemetry.ValidateSymbols))
}

func publishNativeSymbols(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: planty publish-symbols <archive-dSYMs-directory>")
	}
	files, err := filepath.Glob(filepath.Join(args[0], "*.dSYM", "Contents", "Resources", "DWARF", "*"))
	if err != nil || len(files) == 0 {
		return errors.New("archive contains no dSYM DWARF objects")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	for _, path := range files {
		file, err := os.Open(path)
		if err != nil {
			return errors.New("cannot open archived symbol object")
		}
		data, err := io.ReadAll(io.LimitReader(file, nativesymbols.MaxSymbolBytes+1))
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			return errors.New("cannot read archived symbol object")
		}
		imageUUID, architecture, err := nativetelemetry.InspectSymbols(data)
		if err != nil {
			return errors.New("archive contains an invalid or unsupported symbol object")
		}
		if err := nativesymbols.Publish(ctx, os.Getenv("PLANTY_BASE_URL"),
			os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"), os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"),
			imageUUID, architecture, data); err != nil {
			return err
		}
	}
	return nil
}
