package store_test

import (
	"testing"

	"github.com/kotopheiop/lampa-sync-server/internal/store"
)

func TestNormalizeArrayDedupAndIDs(t *testing.T) {
	in := []interface{}{
		float64(1),
		"2",
		map[string]interface{}{"id": float64(3)},
		float64(1),
		nil,
	}
	out := store.NormalizeArray(in)
	if len(out) != 3 {
		t.Fatalf("len=%d want 3: %#v", len(out), out)
	}
}

func TestNormalizeFavoriteObjectFillsKeys(t *testing.T) {
	fav := store.NormalizeFavoriteObject(map[string]interface{}{
		"history": []interface{}{float64(10), "10"},
		"book":    []interface{}{float64(5)},
	})
	for _, k := range store.FavoriteKeys {
		if _, ok := fav[k]; !ok {
			t.Fatalf("missing key %s", k)
		}
	}
	hist := store.AsSlice(fav["history"])
	if len(hist) != 1 {
		t.Fatalf("history dedup len=%d", len(hist))
	}
}

func TestMergeFavoriteUnions(t *testing.T) {
	a := store.NormalizeFavoriteObject(map[string]interface{}{
		"history": []interface{}{float64(1)},
		"book":    []interface{}{float64(2)},
	})
	b := store.NormalizeFavoriteObject(map[string]interface{}{
		"history": []interface{}{float64(1), float64(3)},
		"book":    []interface{}{},
	})
	m := store.MergeFavorite(a, b)
	if len(store.AsSlice(m["history"])) != 2 {
		t.Fatalf("history=%#v", m["history"])
	}
	if len(store.AsSlice(m["book"])) != 1 {
		t.Fatalf("book=%#v", m["book"])
	}
}

func TestApplyProgressUpdateSameDeviceOverwrites(t *testing.T) {
	existing := map[string]interface{}{
		"time":      float64(100),
		"percent":   float64(10),
		"device_id": "a",
		"file_mapping": map[string]interface{}{
			"old": float64(1),
		},
	}
	out := store.ApplyProgressUpdate(existing, float64(1), 50, 5, "new", "a")
	if store.NumOr(out["time"], 0) != 50 {
		t.Fatalf("time=%v", out["time"])
	}
	fm := store.MapOrEmpty(out["file_mapping"])
	if _, ok := fm["old"]; !ok {
		t.Fatal("expected old mapping kept")
	}
	if _, ok := fm["new"]; !ok {
		t.Fatal("expected new mapping")
	}
}

func TestApplyProgressUpdateDifferentDeviceTakesMax(t *testing.T) {
	existing := map[string]interface{}{
		"time":      float64(100),
		"percent":   float64(40),
		"device_id": "a",
	}
	out := store.ApplyProgressUpdate(existing, float64(1), 20, 10, nil, "b")
	if store.NumOr(out["time"], 0) != 100 {
		t.Fatalf("time=%v want max 100", out["time"])
	}
	if store.NumOr(out["percent"], 0) != 40 {
		t.Fatalf("percent=%v want max 40", out["percent"])
	}
}

func TestFindProgressByFileID(t *testing.T) {
	progress := map[string]interface{}{
		"550": map[string]interface{}{
			"time": float64(1),
			"file_mapping": map[string]interface{}{
				"abc": float64(550),
			},
		},
	}
	tmdb, rec, ok := store.FindProgressByFileID(progress, "abc")
	if !ok || tmdb != "550" || rec == nil {
		t.Fatalf("got tmdb=%q ok=%v", tmdb, ok)
	}
	_, _, ok = store.FindProgressByFileID(progress, "missing")
	if ok {
		t.Fatal("expected miss")
	}
}

func TestStringifyID(t *testing.T) {
	if store.StringifyID(float64(550)) != "550" {
		t.Fatal(store.StringifyID(float64(550)))
	}
	if store.StringifyID("x") != "x" {
		t.Fatal(store.StringifyID("x"))
	}
}

func TestBuildProgressSummary(t *testing.T) {
	progress := map[string]interface{}{
		"550": map[string]interface{}{
			"time":         float64(10),
			"percent":      float64(2),
			"file_mapping": map[string]interface{}{"f": float64(550)},
			"updated":      "2026-01-01T00:00:00Z",
			"device_id":    "d1",
			"favorite":     map[string]interface{}{"history": []interface{}{}}, // must not leak
		},
		"bad": "skip-me",
	}
	summary := store.BuildProgressSummary(progress)
	rec, ok := summary["550"].(map[string]interface{})
	if !ok {
		t.Fatalf("%#v", summary)
	}
	if store.NumOr(rec["time"], 0) != 10 {
		t.Fatalf("%v", rec["time"])
	}
	if _, leaked := rec["favorite"]; leaked {
		t.Fatal("favorite must not be in summary")
	}
	if _, ok := summary["bad"]; ok {
		t.Fatal("non-object record should be skipped")
	}
}
