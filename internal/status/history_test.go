package status

import (
	"reflect"
	"testing"

	"nodestatus-go/internal/store"
)

const baseTime = int64(1700000000000)

func TestHistoryParsing(t *testing.T) {
	if got := ResolveHistoryStep(600); got != 10 {
		t.Fatalf("step = %d", got)
	}
	if got := ParseHistoryRange(""); got != 3600 {
		t.Fatalf("default range = %d", got)
	}
	if got := ParseHistoryRange("9999999"); got != maxHistoryRange {
		t.Fatalf("max range = %d", got)
	}
	if got := ParseHistoryMetric("traffic"); got != MetricTraffic {
		t.Fatalf("metric = %s", got)
	}
}

func TestDownsampleBandwidth(t *testing.T) {
	history := []store.BandwidthPoint{
		bw(0, 10, 20, nil, nil),
		bw(2, 30, 40, nil, nil),
		bw(4, 50, 60, nil, nil),
		bw(10, 100, 200, nil, nil),
		bw(12, 120, 240, nil, nil),
	}
	want := []store.BandwidthPoint{bw(4, 30, 40, nil, nil), bw(12, 110, 220, nil, nil)}
	if got := DownsampleBandwidth(history, 600, MetricBandwidth); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestDownsampleTrafficPreservesGaps(t *testing.T) {
	history := []store.BandwidthPoint{
		bw(0, 10, 20, ptr(1000), ptr(2000)),
		bw(2, 20, 40, ptr(2000), ptr(4000)),
		{Time: baseTime + 4000},
		bw(6, 30, 60, ptr(3000), ptr(6000)),
		bw(8, 40, 80, ptr(4000), ptr(8000)),
	}
	want := []store.BandwidthPoint{
		bw(2, 3000, 6000, nil, nil),
		{Time: baseTime + 4000},
		bw(8, 7000, 14000, nil, nil),
	}
	if got := DownsampleBandwidth(history, 600, MetricTraffic); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestDownsampleResource(t *testing.T) {
	history := []store.ResourcePoint{
		res(0, 10, 100, 1000, 1, 2, 100, 200),
		res(2, 20, 200, 1000, 3, 4, 300, 400),
		{Time: baseTime + 4000},
		res(6, 30, 300, 1200, 5, 6, 500, 600),
		res(8, 50, 500, 1200, 7, 8, 700, 800),
	}
	want := []store.ResourcePoint{
		res(2, 15, 150, 1000, 2, 3, 400, 600),
		{Time: baseTime + 4000},
		res(8, 40, 400, 1200, 6, 7, 1200, 1400),
	}
	if got := DownsampleResource(history, 600); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func bw(sec int64, in, out float64, rx, tx *float64) store.BandwidthPoint {
	return store.BandwidthPoint{Time: baseTime + sec*1000, In: ptr(in), Out: ptr(out), Rx: rx, Tx: tx}
}

func res(sec int64, cpu, memUsed, memTotal, in, out, rx, tx float64) store.ResourcePoint {
	return store.ResourcePoint{
		Time: baseTime + sec*1000, CPU: ptr(cpu), MemoryUsed: ptr(memUsed), MemoryTotal: ptr(memTotal),
		NetworkIn: ptr(in), NetworkOut: ptr(out), NetworkRx: ptr(rx), NetworkTx: ptr(tx),
	}
}

func ptr(v float64) *float64 { return &v }
