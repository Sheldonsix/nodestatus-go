package store

import (
	"context"
	"path/filepath"
	"testing"
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
