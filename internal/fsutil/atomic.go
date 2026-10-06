// Package fsutil holds crash-safe file helpers. The bot runs on a Raspberry Pi
// that can lose power at any moment, so state files are never written in place.
package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// BackupSuffix is appended to a file's path to name its last-known-good copy.
const BackupSuffix = ".bak"

// WriteFileAtomic writes data to path without ever leaving a truncated file
// behind: it writes and fsyncs a temp file in the same directory, copies the
// current version to path+".bak", then renames the temp file into place.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	// Keep the previous version as a fallback. Best effort: a failed backup
	// must not block saving fresh state.
	if prev, err := os.ReadFile(path); err == nil && len(prev) > 0 {
		_ = writeBackup(path+BackupSuffix, prev, perm)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("renaming into place: %w", err)
	}
	syncDir(dir)
	return nil
}

func writeBackup(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	d.Close()
}

// ReadFileWithFallback reads path, falling back to its .bak copy when the main
// file is missing or fails validate. fromBackup reports which one was used.
// When neither exists the returned error satisfies os.IsNotExist.
func ReadFileWithFallback(path string, validate func([]byte) error) (data []byte, fromBackup bool, err error) {
	data, mainErr := os.ReadFile(path)
	if mainErr == nil {
		if mainErr = validate(data); mainErr == nil {
			return data, false, nil
		}
	}

	backup, bakErr := os.ReadFile(path + BackupSuffix)
	if bakErr == nil && validate(backup) == nil {
		return backup, true, nil
	}

	if errors.Is(mainErr, os.ErrNotExist) && errors.Is(bakErr, os.ErrNotExist) {
		return nil, false, mainErr
	}
	return nil, false, fmt.Errorf("reading %s: %w (backup: %v)", path, mainErr, bakErr)
}
