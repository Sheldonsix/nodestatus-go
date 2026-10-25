package status

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"nodestatus-go/internal/store"
)

func TestHistoryRetention(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err := st.CreateServer(ctx, store.ServerInput{Username: "one", Password: "secret", Name: "one", Type: "kvm", Location: "US", Region: "US"}); err != nil {
		t.Fatal(err)
	}
	srv, err := st.GetServer(ctx, "one")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := store.HistoryInput{ServerID: srv.ID, CreatedAt: now.Add(-8 * 24 * time.Hour)}
	recent := store.HistoryInput{ServerID: srv.ID, CreatedAt: now.Add(-6 * 24 * time.Hour)}
	if err := st.CreateHistories(ctx, []store.HistoryInput{old, recent}); err != nil {
		t.Fatal(err)
	}
	hub, err := NewHub(st, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()
	check := func() {
		t.Helper()
		points, err := st.ReadBandwidthHistory(ctx, "one", now.Add(-30*24*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if len(points) != 1 || points[0].Time != recent.CreatedAt.UnixMilli() {
			t.Fatalf("expected only the recent history, got %#v", points)
		}
	}
	check()
	if err := st.CreateHistories(ctx, []store.HistoryInput{old}); err != nil {
		t.Fatal(err)
	}
	// The periodic flush must clean up even without connected agents or new samples.
	hub.FlushHistory()
	check()
}
