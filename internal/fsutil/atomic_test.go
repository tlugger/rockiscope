package fsutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func validJSON(b []byte) error {
	var v interface{}
	return json.Unmarshal(b, &v)
}

func TestWriteFileAtomic_KeepsBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	if err := WriteFileAtomic(path, []byte(`{"v":1}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte(`{"v":2}`), 0644); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != `{"v":2}` {
		t.Errorf("main = %s", got)
	}
	bak, _ := os.ReadFile(path + BackupSuffix)
	if string(bak) != `{"v":1}` {
		t.Errorf("backup = %s", bak)
	}

	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 2 {
		t.Errorf("expected only file + backup, got %d entries", len(entries))
	}
}

func TestReadFileWithFallback_CorruptMainUsesBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(path, []byte(`{"v":`), 0644) // truncated mid-write
	os.WriteFile(path+BackupSuffix, []byte(`{"v":1}`), 0644)

	data, fromBackup, err := ReadFileWithFallback(path, validJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !fromBackup || string(data) != `{"v":1}` {
		t.Errorf("fromBackup=%v data=%s", fromBackup, data)
	}
}

func TestReadFileWithFallback_MissingMainUsesBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(path+BackupSuffix, []byte(`{"v":1}`), 0644)

	_, fromBackup, err := ReadFileWithFallback(path, validJSON)
	if err != nil || !fromBackup {
		t.Fatalf("err=%v fromBackup=%v", err, fromBackup)
	}
}

func TestReadFileWithFallback_NothingExists(t *testing.T) {
	_, _, err := ReadFileWithFallback(filepath.Join(t.TempDir(), "nope.json"), validJSON)
	if !os.IsNotExist(err) {
		t.Errorf("expected not-exist error, got %v", err)
	}
}

func TestReadFileWithFallback_BothCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(path, []byte(`{`), 0644)
	os.WriteFile(path+BackupSuffix, []byte(`{`), 0644)

	_, _, err := ReadFileWithFallback(path, validJSON)
	if err == nil || os.IsNotExist(err) {
		t.Errorf("expected corruption error, got %v", err)
	}
}
