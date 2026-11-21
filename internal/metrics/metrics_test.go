package metrics

import (
	"strings"
	"testing"
)

func TestExposition(t *testing.T) {
	c := NewCounter("test_events_total", "Test events.", "kind")
	c.Inc(`a"b\c`)
	c.Add(2, `a"b\c`)
	h := NewHistogram("test_duration_seconds", "Test durations.", []float64{0.1, 1}, "route")
	h.Observe(0.05, "/x")
	h.Observe(0.5, "/x")
	h.Observe(5, "/x")

	var out strings.Builder
	Write(&out, []Gauge{{Name: "test_gauge", Help: "Test gauge.", Value: 7}})
	for _, want := range []string{
		"# HELP test_gauge Test gauge.\n# TYPE test_gauge gauge\ntest_gauge 7\n",
		"# TYPE test_events_total counter\ntest_events_total{kind=\"a\\\"b\\\\c\"} 3\n",
		"# TYPE test_duration_seconds histogram\n" +
			"test_duration_seconds_bucket{route=\"/x\",le=\"0.1\"} 1\n" +
			"test_duration_seconds_bucket{route=\"/x\",le=\"1\"} 2\n" +
			"test_duration_seconds_bucket{route=\"/x\",le=\"+Inf\"} 3\n" +
			"test_duration_seconds_sum{route=\"/x\"} 5.55\n" +
			"test_duration_seconds_count{route=\"/x\"} 3\n",
		"# TYPE go_goroutines gauge\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing:\n%s\nin:\n%s", want, out.String())
		}
	}
	if got := c.Value(`a"b\c`); got != 3 {
		t.Fatalf("value %v", got)
	}
}
