package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// legacySidecars are the SQLite companion files that must travel with a
// relocated database so no write-ahead content is orphaned.
var legacySidecars = []string{"-wal", "-shm"}

// legacyProbeTimeout bounds the write-lock probe so a stuck writer reports an
// actionable error instead of blocking the command.
const legacyProbeTimeout = 2 * time.Second

// MoveLegacyDatabase relocates a project database from the path a previous
// release used to the path the engine uses now, together with its write-ahead
// sidecars, and reports whether a move happened.
//
// The move is attempted only when the legacy database exists and the current
// one does not. Anything ambiguous fails closed with an ACTION: the caller is
// told what to resolve instead of the engine guessing which database is
// authoritative.
func MoveLegacyDatabase(legacyPath string, currentPath string) (bool, error) {
	if legacyPath == "" || currentPath == "" || legacyPath == currentPath {
		return false, nil
	}

	legacyExists, err := fileExists(legacyPath)
	if err != nil {
		return false, fmt.Errorf("inspect legacy database: %w", err)
	}
	if !legacyExists {
		return false, nil
	}

	currentExists, err := fileExists(currentPath)
	if err != nil {
		return false, fmt.Errorf("inspect project database: %w", err)
	}
	if currentExists {
		return false, fmt.Errorf(
			"legacy database %s and project database %s both exist\n"+
				"ACTION: Keep the authoritative database, archive or remove the other, then run the command again.",
			legacyPath, currentPath)
	}

	if err := checkpointLegacyDatabase(legacyPath); err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o700); err != nil {
		return false, fmt.Errorf("create project database directory: %w", err)
	}
	if err := os.Rename(legacyPath, currentPath); err != nil {
		return false, fmt.Errorf(
			"move legacy database %s to %s: %w\n"+
				"ACTION: Move the file manually when the two paths are on different filesystems.",
			legacyPath, currentPath, err)
	}

	if err := moveLegacySidecars(legacyPath, currentPath); err != nil {
		return true, err
	}
	return true, nil
}

// checkpointLegacyDatabase opens the legacy database with an exclusive write
// lock so a live writer is refused and the write-ahead log is folded back into
// the main file before it is renamed.
func checkpointLegacyDatabase(path string) error {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return legacyUnavailable(path, err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), legacyProbeTimeout)
	defer cancel()

	connection, err := database.Conn(ctx)
	if err != nil {
		return legacyUnavailable(path, err)
	}
	defer connection.Close()

	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return legacyUnavailable(path, err)
	}
	if _, err := connection.ExecContext(ctx, "ROLLBACK"); err != nil {
		return legacyUnavailable(path, err)
	}
	if _, err := connection.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return legacyUnavailable(path, err)
	}
	return nil
}

// moveLegacySidecars relocates the sidecars a checkpoint may have left behind.
func moveLegacySidecars(legacyPath string, currentPath string) error {
	for _, suffix := range legacySidecars {
		exists, err := fileExists(legacyPath + suffix)
		if err != nil {
			return fmt.Errorf("inspect legacy database sidecar: %w", err)
		}
		if !exists {
			continue
		}
		if err := os.Rename(legacyPath+suffix, currentPath+suffix); err != nil {
			return fmt.Errorf(
				"move legacy database sidecar %s: %w\n"+
					"ACTION: Move or remove the sidecar manually before running the command again.",
				legacyPath+suffix, err)
		}
	}
	return nil
}

func legacyUnavailable(path string, err error) error {
	return fmt.Errorf(
		"legacy database %s is locked or unreadable: %w\n"+
			"ACTION: Close every process still using it, then run the command again.",
		path, err)
}

func fileExists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
