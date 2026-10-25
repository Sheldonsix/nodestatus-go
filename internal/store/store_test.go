package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreCRUDOrderEvents(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	for _, name := range []string{"one", "two"} {
		if err := st.CreateServer(ctx, ServerInput{Username: name, Password: "secret", Name: name, Type: "kvm", Location: "US", Region: "US"}); err != nil {
			t.Fatal(err)
		}
	}
	servers, err := st.ListServers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 || servers[0].Order != 1 || servers[1].Order != 2 {
		t.Fatalf("unexpected servers: %#v", servers)
	}
	if !ComparePassword(servers[0].Password, "secret") {
		t.Fatal("password was not hashed")
	}
	if err := st.UpdateOrder(ctx, []int{servers[1].ID, servers[0].ID}); err != nil {
		t.Fatal(err)
	}
	servers, _ = st.ListServers(ctx)
	if servers[0].Order != 2 || servers[1].Order != 1 {
		t.Fatalf("order not updated: %#v", servers)
	}
	if _, disconnect, err := st.UpdateServer(ctx, "one", map[string]any{"disabled": true}); err != nil || !disconnect {
		t.Fatalf("update disconnect=%v err=%v", disconnect, err)
	}
	if err := st.CreateEvent(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if err := st.ResolveEvent(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	count, events, err := st.ListEvents(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(events) != 1 || !events[0].Resolved {
		t.Fatalf("events: %d %#v", count, events)
	}
	if err := st.DeleteServer(ctx, "two"); err != nil {
		t.Fatal(err)
	}
	servers, _ = st.ListServers(ctx)
	if len(servers) != 1 {
		t.Fatalf("delete failed: %#v", servers)
	}
}

func TestDeleteHistoriesBefore(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err := st.CreateServer(ctx, ServerInput{Username: "one", Password: "secret", Name: "one", Type: "kvm", Location: "US", Region: "US"}); err != nil {
		t.Fatal(err)
	}
	srv, err := st.GetServer(ctx, "one")
	if err != nil {
		t.Fatal(err)
	}
	before := time.Date(2026, time.October, 18, 12, 34, 56, 123000000, time.UTC)
	for _, tc := range []struct {
		name string
		raw  any
		keep bool
	}{
		{"text old", dbTime(before.Add(-time.Millisecond)), false},
		{"text boundary", dbTime(before), true},
		{"text recent", dbTime(before.Add(time.Millisecond)), true},
		{"milliseconds old", before.Add(-time.Millisecond).UnixMilli(), false},
		{"milliseconds boundary", before.UnixMilli(), true},
		{"milliseconds recent", before.Add(time.Millisecond).UnixMilli(), true},
		{"real old", float64(before.UnixMilli()) - 0.5, false},
		{"real recent", float64(before.UnixMilli()) + 0.5, true},
		{"SQL date old", "2026-10-18 12:34:55", false},
		{"SQL date recent", "2026-10-18 12:34:57", true},
		{"offset old", before.Add(-time.Millisecond).In(time.FixedZone("UTC+8", 8*3600)).Format(time.RFC3339Nano), false},
		{"offset boundary", before.In(time.FixedZone("UTC+8", 8*3600)).Format(time.RFC3339Nano), true},
		{"unknown date", "invalid", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := st.DB().ExecContext(ctx, `INSERT INTO server_histories
				(server_id, created_at, cpu, memory_used, memory_total, network_in, network_out, network_rx, network_tx)
				VALUES (?, ?, 0, 0, 0, 0, 0, 0, 0)`, srv.ID, tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			id, err := result.LastInsertId()
			if err != nil {
				t.Fatal(err)
			}
			if err := st.DeleteHistoriesBefore(ctx, before); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM server_histories WHERE id = ?`, id).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if (count == 1) != tc.keep {
				t.Fatalf("remaining rows = %d, want keep = %v", count, tc.keep)
			}
		})
	}
}
