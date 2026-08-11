package store_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kotopheiop/lampa-sync-server/internal/store"
)

func TestStoreProgressRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := store.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}

	progress := map[string]interface{}{
		"550": map[string]interface{}{
			"time":      float64(12),
			"percent":   float64(3),
			"device_id": "dev",
			"file_mapping": map[string]interface{}{
				"f1": float64(550),
			},
		},
	}
	if err := st.WriteProgress(progress); err != nil {
		t.Fatal(err)
	}
	got, err := st.ReadProgress()
	if err != nil {
		t.Fatal(err)
	}
	rec := got["550"].(map[string]interface{})
	if store.NumOr(rec["time"], 0) != 12 {
		t.Fatalf("%v", rec["time"])
	}

	raw, err := os.ReadFile(filepath.Join(dir, "progress.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
}

func TestStoreFavoriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := store.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := st.WriteFavorite(map[string]interface{}{
		"history": []interface{}{float64(1), float64(2)},
		"book":    []interface{}{float64(9)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.AsSlice(saved["history"])) != 2 {
		t.Fatalf("%v", saved["history"])
	}
	got, err := st.ReadFavorite()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.AsSlice(got["book"])) != 1 {
		t.Fatalf("%v", got["book"])
	}
}

func TestStoreMigratesLegacyFavoriteFromProgress(t *testing.T) {
	dir := t.TempDir()
	progressPath := filepath.Join(dir, "progress.json")
	legacy := map[string]interface{}{
		"100": map[string]interface{}{
			"time": float64(1),
			"favorite": map[string]interface{}{
				"history": []interface{}{float64(100)},
				"book":    []interface{}{float64(7)},
			},
		},
	}
	data, _ := json.MarshalIndent(legacy, "", "  ")
	if err := os.WriteFile(progressPath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := store.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	fav, err := st.ReadFavorite()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.AsSlice(fav["history"])) != 1 {
		t.Fatalf("history=%v", fav["history"])
	}
	if len(store.AsSlice(fav["book"])) != 1 {
		t.Fatalf("book=%v", fav["book"])
	}

	// favorite nested field should be stripped from progress
	progress, err := st.ReadProgress()
	if err != nil {
		t.Fatal(err)
	}
	rec := progress["100"].(map[string]interface{})
	if _, ok := rec["favorite"]; ok {
		t.Fatal("legacy favorite should be removed from progress")
	}
	if _, err := os.Stat(filepath.Join(dir, "favorite.json")); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidProgressJSONBackedUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := store.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.ReadProgress()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("%v", got)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "progress.json.backup.*"))
	if len(matches) == 0 {
		t.Fatal("expected backup file")
	}
}

func TestUnlockedHelpersAndEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	st, err := store.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "progress.json"), []byte("   \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "favorite.json"), []byte("\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = st.WithLock(func() error {
		p, e := st.ReadProgressUnlocked()
		if e != nil {
			return e
		}
		if len(p) != 0 {
			t.Fatalf("%v", p)
		}
		p["1"] = map[string]interface{}{"time": float64(1), "percent": float64(1)}
		if e := st.WriteProgressUnlocked(p); e != nil {
			return e
		}
		f, e := st.ReadFavoriteUnlocked()
		if e != nil {
			return e
		}
		f["history"] = []interface{}{float64(1)}
		_, e = st.WriteFavoriteUnlocked(f)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.ReadProgress()
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
}

func TestFindProgressSkipsNonObject(t *testing.T) {
	progress := map[string]interface{}{
		"x": "nope",
		"2": map[string]interface{}{
			"file_mapping": map[string]interface{}{"f": float64(2)},
		},
	}
	tmdb, _, ok := store.FindProgressByFileID(progress, "f")
	if !ok || tmdb != "2" {
		t.Fatalf("%q %v", tmdb, ok)
	}
}
