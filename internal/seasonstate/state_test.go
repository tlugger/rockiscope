package seasonstate

import (
	"encoding/json"
	"os"
	"testing"
)

func TestReadMissingAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, _, err := Read(dir)
	if !os.IsNotExist(err) || st == nil || st.Done == nil {
		t.Fatalf("missing file: st=%v err=%v", st, err)
	}

	in := New()
	in.Adoptions = []Adoption{{Season: 2026, TeamID: 135, TeamName: "San Diego Padres"}}
	in.HotStove = []HotStoveEntry{{ID: 1, Description: "x", Compatibility: -1}}
	data, _ := json.Marshal(in)
	os.WriteFile(Path(dir), data, 0644)

	out, fromBackup, err := Read(dir)
	if err != nil || fromBackup {
		t.Fatalf("err=%v fromBackup=%v", err, fromBackup)
	}
	if len(out.Adoptions) != 1 || out.HotStove[0].Compatibility != -1 || out.Rollovers == nil {
		t.Errorf("round trip lost data: %+v", out)
	}
}

func TestReadOlderFileInitsMaps(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(Path(dir), []byte(`{"done":{"k":"v"}}`), 0644)
	st, _, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Readings == nil || st.Champions == nil || st.OpeningDays == nil || st.Done["k"] != "v" {
		t.Errorf("maps not initialized: %+v", st)
	}
}

func TestReadCorrupt(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(Path(dir), []byte(`{`), 0644)
	if _, _, err := Read(dir); err == nil || os.IsNotExist(err) {
		t.Errorf("expected parse error, got %v", err)
	}
}
