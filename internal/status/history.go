package status

import (
	"sort"
	"strconv"

	"nodestatus-go/internal/store"
)

const (
	defaultHistoryRange = 3600
	maxHistoryRange     = 30 * 24 * 3600
)

type Metric string

const (
	MetricBandwidth Metric = "bandwidth"
	MetricTraffic   Metric = "traffic"
	MetricResource  Metric = "resource"
)

var historySteps = []struct {
	rng  int
	step int
}{
	{60, 2},
	{600, 10},
	{1800, 30},
	{3600, 60},
	{86400, 300},
	{604800, 1800},
	{maxHistoryRange, 3600},
}

func ParseHistoryRange(raw string) int {
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || n <= 0 {
		return defaultHistoryRange
	}
	if n > maxHistoryRange {
		return maxHistoryRange
	}
	i := int(n)
	if n > float64(i) {
		i++
	}
	return i
}

func ParseHistoryMetric(raw string) Metric {
	switch raw {
	case string(MetricResource):
		return MetricResource
	case string(MetricTraffic):
		return MetricTraffic
	default:
		return MetricBandwidth
	}
}

func ResolveHistoryStep(rng int) int {
	for _, item := range historySteps {
		if rng <= item.rng {
			return item.step
		}
	}
	return historySteps[len(historySteps)-1].step
}

func MergeBandwidth(stored, memory []store.BandwidthPoint) []store.BandwidthPoint {
	last := int64(0)
	if len(stored) > 0 {
		last = stored[len(stored)-1].Time
	}
	out := append([]store.BandwidthPoint{}, stored...)
	for _, item := range memory {
		if item.Time > last {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out
}

func MergeResource(stored, memory []store.ResourcePoint) []store.ResourcePoint {
	last := int64(0)
	if len(stored) > 0 {
		last = stored[len(stored)-1].Time
	}
	out := append([]store.ResourcePoint{}, stored...)
	for _, item := range memory {
		if item.Time > last {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out
}

func DownsampleBandwidth(history []store.BandwidthPoint, rng int, metric Metric) []store.BandwidthPoint {
	if len(history) == 0 {
		return []store.BandwidthPoint{}
	}
	step := ResolveHistoryStep(rng)
	cutoff := history[len(history)-1].Time - int64(rng)*1000
	filtered := []store.BandwidthPoint{}
	for _, item := range history {
		if item.Time >= cutoff {
			filtered = append(filtered, item)
		}
	}
	if step <= 2 {
		out := make([]store.BandwidthPoint, len(filtered))
		for i, item := range filtered {
			in, outValue := metricValues(item, metric)
			out[i] = store.BandwidthPoint{Time: item.Time, In: in, Out: outValue}
		}
		return out
	}

	stepMS := int64(step * 1000)
	data := []store.BandwidthPoint{}
	var bucket *bandwidthBucket
	flush := func() {
		if bucket == nil {
			return
		}
		input, output := bucket.in, bucket.out
		if metric != MetricTraffic {
			input /= float64(bucket.count)
			output /= float64(bucket.count)
		}
		data = append(data, store.BandwidthPoint{Time: bucket.time, In: fptr(input), Out: fptr(output)})
		bucket = nil
	}
	for _, item := range filtered {
		input, output := metricValues(item, metric)
		if input == nil || output == nil {
			flush()
			data = append(data, store.BandwidthPoint{Time: item.Time})
			continue
		}
		key := item.Time / stepMS
		if bucket == nil || bucket.key != key {
			flush()
			bucket = &bandwidthBucket{key: key, time: item.Time, in: *input, out: *output, count: 1}
			continue
		}
		bucket.time = item.Time
		bucket.in += *input
		bucket.out += *output
		if metric != MetricTraffic {
			bucket.count++
		}
	}
	flush()
	return data
}

func DownsampleResource(history []store.ResourcePoint, rng int) []store.ResourcePoint {
	if len(history) == 0 {
		return []store.ResourcePoint{}
	}
	step := ResolveHistoryStep(rng)
	cutoff := history[len(history)-1].Time - int64(rng)*1000
	filtered := []store.ResourcePoint{}
	for _, item := range history {
		if item.Time >= cutoff {
			filtered = append(filtered, item)
		}
	}
	if step <= 2 {
		return filtered
	}

	stepMS := int64(step * 1000)
	data := []store.ResourcePoint{}
	var bucket *resourceBucket
	flush := func() {
		if bucket == nil {
			return
		}
		count := float64(bucket.count)
		data = append(data, store.ResourcePoint{
			Time: bucket.time, CPU: fptr(bucket.cpu / count), MemoryUsed: fptr(bucket.memoryUsed / count),
			MemoryTotal: fptr(bucket.memoryTotal / count), NetworkIn: fptr(bucket.networkIn / count),
			NetworkOut: fptr(bucket.networkOut / count), NetworkRx: fptr(bucket.networkRx), NetworkTx: fptr(bucket.networkTx),
		})
		bucket = nil
	}
	for _, item := range filtered {
		if !hasResourceValues(item) {
			flush()
			data = append(data, store.ResourcePoint{Time: item.Time})
			continue
		}
		key := item.Time / stepMS
		if bucket == nil || bucket.key != key {
			flush()
			bucket = &resourceBucket{
				key: key, time: item.Time, cpu: *item.CPU, memoryUsed: *item.MemoryUsed, memoryTotal: *item.MemoryTotal,
				networkIn: *item.NetworkIn, networkOut: *item.NetworkOut, networkRx: *item.NetworkRx, networkTx: *item.NetworkTx,
				count: 1,
			}
			continue
		}
		bucket.time = item.Time
		bucket.cpu += *item.CPU
		bucket.memoryUsed += *item.MemoryUsed
		bucket.memoryTotal += *item.MemoryTotal
		bucket.networkIn += *item.NetworkIn
		bucket.networkOut += *item.NetworkOut
		bucket.networkRx += *item.NetworkRx
		bucket.networkTx += *item.NetworkTx
		bucket.count++
	}
	flush()
	return data
}

type bandwidthBucket struct {
	key, time int64
	in, out   float64
	count     int
}

type resourceBucket struct {
	key, time                                   int64
	cpu, memoryUsed, memoryTotal                float64
	networkIn, networkOut, networkRx, networkTx float64
	count                                       int
}

func metricValues(item store.BandwidthPoint, metric Metric) (*float64, *float64) {
	if metric == MetricTraffic {
		return item.Rx, item.Tx
	}
	return item.In, item.Out
}

func hasResourceValues(item store.ResourcePoint) bool {
	return item.CPU != nil && item.MemoryUsed != nil && item.MemoryTotal != nil &&
		item.NetworkIn != nil && item.NetworkOut != nil && item.NetworkRx != nil && item.NetworkTx != nil
}

func fptr(v float64) *float64 { return &v }
