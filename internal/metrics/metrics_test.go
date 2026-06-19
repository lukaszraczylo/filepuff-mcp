package metrics

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCounter(t *testing.T) {
	tests := []struct {
		name     string
		ops      func(c *Counter)
		expected int64
	}{
		{
			name:     "initial value is zero",
			ops:      func(c *Counter) {},
			expected: 0,
		},
		{
			name: "single inc",
			ops: func(c *Counter) {
				c.Inc()
			},
			expected: 1,
		},
		{
			name: "multiple inc",
			ops: func(c *Counter) {
				c.Inc()
				c.Inc()
				c.Inc()
			},
			expected: 3,
		},
		{
			name: "add positive",
			ops: func(c *Counter) {
				c.Add(10)
			},
			expected: 10,
		},
		{
			name: "mixed operations",
			ops: func(c *Counter) {
				c.Inc()
				c.Add(5)
				c.Inc()
			},
			expected: 7,
		},
		{
			name: "reset",
			ops: func(c *Counter) {
				c.Add(100)
				c.Reset()
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCounter("test_counter", "test help", nil)
			tt.ops(c)
			if got := c.Value(); got != tt.expected {
				t.Errorf("Counter.Value() = %d, want %d", got, tt.expected)
			}
		})
	}
}

// Counters are monotonic: a negative Add must be ignored, not applied.
func TestCounterRejectsNegativeAdd(t *testing.T) {
	c := NewCounter("mono_counter", "test", nil)
	c.Add(5)
	c.Add(-3)
	if got := c.Value(); got != 5 {
		t.Errorf("negative Add should be ignored: Value() = %d, want 5", got)
	}
}

func TestCounterConcurrency(t *testing.T) {
	c := NewCounter("concurrent_counter", "test", nil)
	var wg sync.WaitGroup
	numGoroutines := 100
	incsPerGoroutine := 1000

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incsPerGoroutine; j++ {
				c.Inc()
			}
		}()
	}

	wg.Wait()

	expected := int64(numGoroutines * incsPerGoroutine)
	if got := c.Value(); got != expected {
		t.Errorf("Counter.Value() = %d, want %d after concurrent increments", got, expected)
	}
}

func TestGauge(t *testing.T) {
	tests := []struct {
		name     string
		ops      func(g *Gauge)
		expected int64
	}{
		{
			name:     "initial value is zero",
			ops:      func(g *Gauge) {},
			expected: 0,
		},
		{
			name: "set value",
			ops: func(g *Gauge) {
				g.Set(42)
			},
			expected: 42,
		},
		{
			name: "inc",
			ops: func(g *Gauge) {
				g.Inc()
				g.Inc()
			},
			expected: 2,
		},
		{
			name: "dec",
			ops: func(g *Gauge) {
				g.Set(10)
				g.Dec()
				g.Dec()
			},
			expected: 8,
		},
		{
			name: "add positive",
			ops: func(g *Gauge) {
				g.Add(5)
			},
			expected: 5,
		},
		{
			name: "add negative",
			ops: func(g *Gauge) {
				g.Set(10)
				g.Add(-3)
			},
			expected: 7,
		},
		{
			name: "mixed operations",
			ops: func(g *Gauge) {
				g.Set(100)
				g.Inc()
				g.Dec()
				g.Add(-50)
			},
			expected: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGauge("test_gauge", "test help", nil)
			tt.ops(g)
			if got := g.Value(); got != tt.expected {
				t.Errorf("Gauge.Value() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestHistogram(t *testing.T) {
	t.Run("default buckets", func(t *testing.T) {
		h := NewHistogram("test_histogram", "test", nil, nil)
		if len(h.buckets) != len(DefaultDurationBuckets) {
			t.Errorf("expected default buckets, got %d buckets", len(h.buckets))
		}
	})

	t.Run("custom buckets", func(t *testing.T) {
		buckets := []float64{1.0, 5.0, 10.0}
		h := NewHistogram("test_histogram", "test", nil, buckets)
		if len(h.buckets) != 3 {
			t.Errorf("expected 3 buckets, got %d", len(h.buckets))
		}
	})

	t.Run("buckets are sorted", func(t *testing.T) {
		buckets := []float64{10.0, 1.0, 5.0}
		h := NewHistogram("test_histogram", "test", nil, buckets)
		if h.buckets[0] != 1.0 || h.buckets[1] != 5.0 || h.buckets[2] != 10.0 {
			t.Errorf("buckets not sorted: %v", h.buckets)
		}
	})

	t.Run("observe values", func(t *testing.T) {
		h := NewHistogram("test_histogram", "test", nil, []float64{1.0, 5.0, 10.0})

		h.Observe(0.5)  // goes to bucket 0 (<=1.0)
		h.Observe(3.0)  // goes to bucket 1 (<=5.0)
		h.Observe(7.0)  // goes to bucket 2 (<=10.0)
		h.Observe(15.0) // goes to +Inf bucket

		if h.Count() != 4 {
			t.Errorf("expected count 4, got %d", h.Count())
		}
	})

	t.Run("observe duration", func(t *testing.T) {
		h := NewHistogram("test_histogram", "test", nil, []float64{0.001, 0.01, 0.1})

		h.ObserveDuration(500 * time.Microsecond) // 0.0005s, goes to bucket 0
		h.ObserveDuration(5 * time.Millisecond)   // 0.005s, goes to bucket 1

		if h.Count() != 2 {
			t.Errorf("expected count 2, got %d", h.Count())
		}
	})

	t.Run("sum tracking", func(t *testing.T) {
		h := NewHistogram("test_histogram", "test", nil, []float64{1.0, 5.0, 10.0})

		h.Observe(1.0)
		h.Observe(2.0)
		h.Observe(3.0)

		expectedSum := 6.0
		if got := h.Sum(); got != expectedSum {
			t.Errorf("expected sum %f, got %f", expectedSum, got)
		}
	})
}

func TestRegistry(t *testing.T) {
	t.Run("counter registration", func(t *testing.T) {
		r := NewRegistry()

		c1 := r.Counter("test_counter", "help", nil)
		c2 := r.Counter("test_counter", "help", nil)

		if c1 != c2 {
			t.Error("expected same counter instance for same name")
		}
	})

	t.Run("counter with labels", func(t *testing.T) {
		r := NewRegistry()

		labels1 := map[string]string{"method": "get"}
		labels2 := map[string]string{"method": "post"}

		c1 := r.Counter("http_requests", "help", labels1)
		c2 := r.Counter("http_requests", "help", labels2)

		if c1 == c2 {
			t.Error("expected different counter instances for different labels")
		}
	})

	t.Run("gauge registration", func(t *testing.T) {
		r := NewRegistry()

		g1 := r.Gauge("test_gauge", "help", nil)
		g2 := r.Gauge("test_gauge", "help", nil)

		if g1 != g2 {
			t.Error("expected same gauge instance for same name")
		}
	})

	t.Run("histogram registration", func(t *testing.T) {
		r := NewRegistry()

		h1 := r.Histogram("test_histogram", "help", nil, nil)
		h2 := r.Histogram("test_histogram", "help", nil, nil)

		if h1 != h2 {
			t.Error("expected same histogram instance for same name")
		}
	})
}

func TestRegistryConcurrency(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent access to registry
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			c := r.Counter("concurrent_test", "test", nil)
			c.Inc()

			g := r.Gauge("concurrent_gauge", "test", nil)
			g.Inc()
		}(i)
	}

	wg.Wait()

	c := r.Counter("concurrent_test", "test", nil)
	if c.Value() != int64(numGoroutines) {
		t.Errorf("expected counter value %d, got %d", numGoroutines, c.Value())
	}
}

func TestRegistryExpose(t *testing.T) {
	r := NewRegistry()

	// Add some metrics
	c := r.Counter("test_requests_total", "Total requests", nil)
	c.Add(42)

	g := r.Gauge("test_connections", "Active connections", nil)
	g.Set(10)

	h := r.Histogram("test_duration_seconds", "Request duration", nil, []float64{0.1, 0.5, 1.0})
	h.Observe(0.05)
	h.Observe(0.3)
	h.Observe(0.8)

	output := r.Expose()

	// Check counter output
	if !strings.Contains(output, "# TYPE test_requests_total counter") {
		t.Error("expected counter type in output")
	}
	if !strings.Contains(output, "test_requests_total 42") {
		t.Error("expected counter value in output")
	}

	// Check gauge output
	if !strings.Contains(output, "# TYPE test_connections gauge") {
		t.Error("expected gauge type in output")
	}
	if !strings.Contains(output, "test_connections 10") {
		t.Error("expected gauge value in output")
	}

	// Check histogram output
	if !strings.Contains(output, "# TYPE test_duration_seconds histogram") {
		t.Error("expected histogram type in output")
	}
	if !strings.Contains(output, "test_duration_seconds_bucket") {
		t.Error("expected histogram buckets in output")
	}
	if !strings.Contains(output, "test_duration_seconds_sum") {
		t.Error("expected histogram sum in output")
	}
	if !strings.Contains(output, "test_duration_seconds_count") {
		t.Error("expected histogram count in output")
	}
}

func TestRegistryReset(t *testing.T) {
	r := NewRegistry()

	c := r.Counter("test_counter", "test", nil)
	c.Add(100)

	g := r.Gauge("test_gauge", "test", nil)
	g.Set(50)

	r.Reset()

	if c.Value() != 0 {
		t.Errorf("expected counter reset to 0, got %d", c.Value())
	}
	if g.Value() != 0 {
		t.Errorf("expected gauge reset to 0, got %d", g.Value())
	}
}

func TestServerMetrics(t *testing.T) {
	t.Run("creation", func(t *testing.T) {
		m := NewServerMetrics()

		if m.RequestsTotal == nil {
			t.Error("RequestsTotal should not be nil")
		}
		if m.RequestErrors == nil {
			t.Error("RequestErrors should not be nil")
		}
		if m.RequestDuration == nil {
			t.Error("RequestDuration should not be nil")
		}
		if m.CacheHits == nil {
			t.Error("CacheHits should not be nil")
		}
		if m.CacheMisses == nil {
			t.Error("CacheMisses should not be nil")
		}
		if m.ActiveLSPServers == nil {
			t.Error("ActiveLSPServers should not be nil")
		}
		if m.ParseDuration == nil {
			t.Error("ParseDuration should not be nil")
		}
		if m.ParseErrors == nil {
			t.Error("ParseErrors should not be nil")
		}
	})

	t.Run("record request success", func(t *testing.T) {
		m := NewServerMetrics()

		m.RecordRequest(100*time.Millisecond, nil)

		if m.RequestsTotal.Value() != 1 {
			t.Errorf("expected RequestsTotal 1, got %d", m.RequestsTotal.Value())
		}
		if m.RequestErrors.Value() != 0 {
			t.Errorf("expected RequestErrors 0, got %d", m.RequestErrors.Value())
		}
		if m.RequestDuration.Count() != 1 {
			t.Errorf("expected RequestDuration count 1, got %d", m.RequestDuration.Count())
		}
	})

	t.Run("record request error", func(t *testing.T) {
		m := NewServerMetrics()

		m.RecordRequest(50*time.Millisecond, &testError{})

		if m.RequestsTotal.Value() != 1 {
			t.Errorf("expected RequestsTotal 1, got %d", m.RequestsTotal.Value())
		}
		if m.RequestErrors.Value() != 1 {
			t.Errorf("expected RequestErrors 1, got %d", m.RequestErrors.Value())
		}
	})

	t.Run("record parse", func(t *testing.T) {
		m := NewServerMetrics()

		m.RecordParse(10*time.Millisecond, nil)
		m.RecordParse(5*time.Millisecond, &testError{})

		if m.ParseDuration.Count() != 2 {
			t.Errorf("expected ParseDuration count 2, got %d", m.ParseDuration.Count())
		}
		if m.ParseErrors.Value() != 1 {
			t.Errorf("expected ParseErrors 1, got %d", m.ParseErrors.Value())
		}
	})

	t.Run("record cache", func(t *testing.T) {
		m := NewServerMetrics()

		m.RecordCacheHit()
		m.RecordCacheHit()
		m.RecordCacheMiss()

		if m.CacheHits.Value() != 2 {
			t.Errorf("expected CacheHits 2, got %d", m.CacheHits.Value())
		}
		if m.CacheMisses.Value() != 1 {
			t.Errorf("expected CacheMisses 1, got %d", m.CacheMisses.Value())
		}
	})

	t.Run("set active LSP servers", func(t *testing.T) {
		m := NewServerMetrics()

		m.SetActiveLSPServers(5)
		if m.ActiveLSPServers.Value() != 5 {
			t.Errorf("expected ActiveLSPServers 5, got %d", m.ActiveLSPServers.Value())
		}

		m.SetActiveLSPServers(3)
		if m.ActiveLSPServers.Value() != 3 {
			t.Errorf("expected ActiveLSPServers 3, got %d", m.ActiveLSPServers.Value())
		}
	})

	t.Run("expose", func(t *testing.T) {
		m := NewServerMetrics()
		m.RecordRequest(100*time.Millisecond, nil)

		output := m.Expose()

		if !strings.Contains(output, "mcp_requests_total") {
			t.Error("expected mcp_requests_total in output")
		}
		if !strings.Contains(output, "mcp_request_duration_seconds") {
			t.Error("expected mcp_request_duration_seconds in output")
		}
	})
}

func TestMetricKey(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{
			name:     "no labels",
			labels:   nil,
			expected: "test_metric",
		},
		{
			name:     "single label",
			labels:   map[string]string{"method": "get"},
			expected: `test_metric{method="get"}`,
		},
		{
			name:     "multiple labels sorted",
			labels:   map[string]string{"method": "get", "code": "200"},
			expected: `test_metric{code="200",method="get"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metricKey("test_metric", tt.labels)
			if got != tt.expected {
				t.Errorf("metricKey() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFormatLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{
			name:     "no labels",
			labels:   nil,
			expected: "",
		},
		{
			name:     "single label",
			labels:   map[string]string{"method": "get"},
			expected: `{method="get"}`,
		},
		{
			name:     "multiple labels sorted",
			labels:   map[string]string{"method": "get", "code": "200"},
			expected: `{code="200",method="get"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatLabels(tt.labels)
			if got != tt.expected {
				t.Errorf("formatLabels() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// testError is a simple error type for testing
type testError struct{}

func (e *testError) Error() string { return "test error" }
