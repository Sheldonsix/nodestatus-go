package main

import (
	"testing"
	"time"
)

func TestParseEndpoint(t *testing.T) {
	server, username, password, err := parseEndpoint("https://user:pass@example.com/status")
	if err != nil {
		t.Fatal(err)
	}
	if server != "wss://example.com" || username != "user" || password != "pass" {
		t.Fatalf("unexpected endpoint: %q %q %q", server, username, password)
	}
}

func TestTrafficCounterFirstSampleHasZeroRates(t *testing.T) {
	counter := &trafficCounter{}
	rx, tx := counter.rates(time.Unix(100, 0), 1000, 2000)
	if rx != 0 || tx != 0 {
		t.Fatalf("first sample rates = %d/%d, want 0/0", rx, tx)
	}

	rx, tx = counter.rates(time.Unix(102, 0), 3000, 5000)
	if rx != 1000 || tx != 1500 {
		t.Fatalf("second sample rates = %d/%d, want 1000/1500", rx, tx)
	}
}

func TestFillNetworkFieldsUsesCurrentServerSemantics(t *testing.T) {
	item := nodeStatus{}
	fillNetworkFields(&item, 10000, 20000, 100, 200)

	if item.NetworkRx != 10000 || item.NetworkTx != 20000 {
		t.Fatalf("network_rx/tx = %d/%d, want cumulative totals 10000/20000", item.NetworkRx, item.NetworkTx)
	}
	if item.NetworkIn != 100 || item.NetworkOut != 200 {
		t.Fatalf("network_in/out = %d/%d, want rates 100/200", item.NetworkIn, item.NetworkOut)
	}
}
