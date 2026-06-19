// Package metrics provides Prometheus-style metrics collection for the MCP server.
// It offers low-overhead, thread-safe metric types suitable for observability.
package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Counter is a monotonically increasing metric.
type Counter struct {
	name   string
	help   string
	labels map[string]string
	value  atomic.Int64
}

// NewCounter creates a new counter metric.
func NewCounter(name, help string, labels map[string]string) *Counter {
	return &Counter{
		name:   name,
		help:   help,
		labels: labels,
	}
}

// Inc increments the counter by 1.
func (c *Counter) Inc() {
	c.value.Add(1)
}

// Add adds the given value to the counter. Counters are monotonic, so a
// negative delta (which would corrupt rate()-style calculations downstream) is
// ignored rather than applied.
func (c *Counter) Add(delta int64) {
	if delta < 0 {
		return
	}
	c.value.Add(delta)
}

// Value returns the current counter value.
func (c *Counter) Value() int64 {
	return c.value.Load()
}

// Reset resets the counter to 0.
func (c *Counter) Reset() {
	c.value.Store(0)
}

// Gauge is a metric that can go up or down.
type Gauge struct {
	name   string
	help   string
	labels map[string]string
	value  atomic.Int64
}

// NewGauge creates a new gauge metric.
func NewGauge(name, help string, labels map[string]string) *Gauge {
	return &Gauge{
		name:   name,
		help:   help,
		labels: labels,
	}
}

// Set sets the gauge to the given value.
func (g *Gauge) Set(val int64) {
	g.value.Store(val)
}

// Inc increments the gauge by 1.
func (g *Gauge) Inc() {
	g.value.Add(1)
}

// Dec decrements the gauge by 1.
func (g *Gauge) Dec() {
	g.value.Add(-1)
}

// Add adds the given value to the gauge.
func (g *Gauge) Add(delta int64) {
	g.value.Add(delta)
}

// Value returns the current gauge value.
func (g *Gauge) Value() int64 {
	return g.value.Load()
}

// Histogram tracks the distribution of values in predefined buckets.
type Histogram struct {
	name    string
	help    string
	labels  map[string]string
	buckets []float64
	counts  []atomic.Int64
	sum     atomic.Int64 // sum of all observed values (in nanoseconds for durations)
	count   atomic.Int64 // total count of observations
}

// DefaultDurationBuckets are default buckets for request durations (in seconds).
var DefaultDurationBuckets = []float64{
	0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
}

// NewHistogram creates a new histogram with the given buckets.
func NewHistogram(name, help string, labels map[string]string, buckets []float64) *Histogram {
	if buckets == nil {
		buckets = DefaultDurationBuckets
	}
	// Ensure buckets are sorted
	sorted := make([]float64, len(buckets))
	copy(sorted, buckets)
	sort.Float64s(sorted)

	h := &Histogram{
		name:    name,
		help:    help,
		labels:  labels,
		buckets: sorted,
		counts:  make([]atomic.Int64, len(sorted)+1), // +1 for +Inf bucket
	}
	return h
}

// Observe records a value in the histogram.
func (h *Histogram) Observe(val float64) {
	h.count.Add(1)
	// Store sum in nanoseconds for precision with durations
	h.sum.Add(int64(val * 1e9))

	// Find bucket and increment
	for i, bound := range h.buckets {
		if val <= bound {
			h.counts[i].Add(1)
			return
		}
	}
	// Value exceeds all buckets, add to +Inf
	h.counts[len(h.buckets)].Add(1)
}

// ObserveDuration records a duration in seconds.
func (h *Histogram) ObserveDuration(d time.Duration) {
	h.Observe(d.Seconds())
}

// Count returns the total number of observations.
func (h *Histogram) Count() int64 {
	return h.count.Load()
}

// Sum returns the sum of all observations.
func (h *Histogram) Sum() float64 {
	return float64(h.sum.Load()) / 1e9
}

// Registry holds all registered metrics.
type Registry struct {
	mu         sync.RWMutex
	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
}

// NewRegistry creates a new metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
	}
}

// Counter returns or creates a counter with the given name.
func (r *Registry) Counter(name, help string, labels map[string]string) *Counter {
	key := metricKey(name, labels)
	r.mu.RLock()
	if c, ok := r.counters[key]; ok {
		r.mu.RUnlock()
		return c
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if c, ok := r.counters[key]; ok {
		return c
	}

	c := NewCounter(name, help, labels)
	r.counters[key] = c
	return c
}

// Gauge returns or creates a gauge with the given name.
func (r *Registry) Gauge(name, help string, labels map[string]string) *Gauge {
	key := metricKey(name, labels)
	r.mu.RLock()
	if g, ok := r.gauges[key]; ok {
		r.mu.RUnlock()
		return g
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	if g, ok := r.gauges[key]; ok {
		return g
	}

	g := NewGauge(name, help, labels)
	r.gauges[key] = g
	return g
}

// Histogram returns or creates a histogram with the given name.
func (r *Registry) Histogram(name, help string, labels map[string]string, buckets []float64) *Histogram {
	key := metricKey(name, labels)
	r.mu.RLock()
	if h, ok := r.histograms[key]; ok {
		r.mu.RUnlock()
		return h
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	if h, ok := r.histograms[key]; ok {
		return h
	}

	h := NewHistogram(name, help, labels, buckets)
	r.histograms[key] = h
	return h
}

// metricKey creates a unique key for a metric based on name and labels.
func metricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	var parts []string
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf("%s=%q", k, v))
	}
	sort.Strings(parts)
	return name + "{" + strings.Join(parts, ",") + "}"
}

// formatLabels formats labels for Prometheus output.
func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	var parts []string
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf("%s=%q", k, v))
	}
	sort.Strings(parts)
	return "{" + strings.Join(parts, ",") + "}"
}

// Expose returns all metrics in Prometheus text format.
func (r *Registry) Expose() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder

	// Export counters
	for _, c := range r.counters {
		if c.help != "" {
			fmt.Fprintf(&sb, "# HELP %s %s\n", c.name, c.help)
		}
		fmt.Fprintf(&sb, "# TYPE %s counter\n", c.name)
		fmt.Fprintf(&sb, "%s%s %d\n", c.name, formatLabels(c.labels), c.value.Load())
	}

	// Export gauges
	for _, g := range r.gauges {
		if g.help != "" {
			fmt.Fprintf(&sb, "# HELP %s %s\n", g.name, g.help)
		}
		fmt.Fprintf(&sb, "# TYPE %s gauge\n", g.name)
		fmt.Fprintf(&sb, "%s%s %d\n", g.name, formatLabels(g.labels), g.value.Load())
	}

	// Export histograms
	for _, h := range r.histograms {
		if h.help != "" {
			fmt.Fprintf(&sb, "# HELP %s %s\n", h.name, h.help)
		}
		fmt.Fprintf(&sb, "# TYPE %s histogram\n", h.name)

		// Cumulative bucket counts
		var cumulative int64
		for i, bound := range h.buckets {
			cumulative += h.counts[i].Load()
			labelStr := formatLabels(h.labels)
			if labelStr == "" {
				fmt.Fprintf(&sb, "%s_bucket{le=\"%g\"} %d\n", h.name, bound, cumulative)
			} else {
				// Insert le label into existing labels
				fmt.Fprintf(&sb, "%s_bucket%s %d\n", h.name,
					strings.Replace(labelStr, "}", fmt.Sprintf(",le=\"%g\"}", bound), 1), cumulative)
			}
		}
		// +Inf bucket
		cumulative += h.counts[len(h.buckets)].Load()
		labelStr := formatLabels(h.labels)
		if labelStr == "" {
			fmt.Fprintf(&sb, "%s_bucket{le=\"+Inf\"} %d\n", h.name, cumulative)
		} else {
			fmt.Fprintf(&sb, "%s_bucket%s %d\n", h.name,
				strings.Replace(labelStr, "}", ",le=\"+Inf\"}", 1), cumulative)
		}

		// Sum and count
		fmt.Fprintf(&sb, "%s_sum%s %g\n", h.name, formatLabels(h.labels), h.Sum())
		fmt.Fprintf(&sb, "%s_count%s %d\n", h.name, formatLabels(h.labels), h.count.Load())
	}

	return sb.String()
}

// Reset resets all metrics to zero.
func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, c := range r.counters {
		c.Reset()
	}
	for _, g := range r.gauges {
		g.Set(0)
	}
	// Note: Histograms don't have a simple reset due to atomic bucket counts
}

// ServerMetrics provides pre-defined metrics for the MCP server.
type ServerMetrics struct {
	registry *Registry

	// Request metrics
	RequestsTotal   *Counter
	RequestErrors   *Counter
	RequestDuration *Histogram

	// Cache metrics
	CacheHits   *Counter
	CacheMisses *Counter

	// LSP metrics
	ActiveLSPServers *Gauge

	// Parse metrics
	ParseDuration *Histogram
	ParseErrors   *Counter
}

// NewServerMetrics creates a new set of server metrics.
func NewServerMetrics() *ServerMetrics {
	r := NewRegistry()

	return &ServerMetrics{
		registry: r,

		RequestsTotal: r.Counter(
			"mcp_requests_total",
			"Total number of MCP requests processed",
			nil,
		),
		RequestErrors: r.Counter(
			"mcp_request_errors_total",
			"Total number of MCP request errors",
			nil,
		),
		RequestDuration: r.Histogram(
			"mcp_request_duration_seconds",
			"Request duration in seconds",
			nil,
			DefaultDurationBuckets,
		),

		CacheHits: r.Counter(
			"mcp_cache_hits_total",
			"Total number of cache hits",
			nil,
		),
		CacheMisses: r.Counter(
			"mcp_cache_misses_total",
			"Total number of cache misses",
			nil,
		),

		ActiveLSPServers: r.Gauge(
			"mcp_lsp_servers_active",
			"Number of active LSP server connections",
			nil,
		),

		ParseDuration: r.Histogram(
			"mcp_parse_duration_seconds",
			"Parse duration in seconds",
			nil,
			[]float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
		),
		ParseErrors: r.Counter(
			"mcp_parse_errors_total",
			"Total number of parse errors",
			nil,
		),
	}
}

// Expose returns all metrics in Prometheus text format.
func (m *ServerMetrics) Expose() string {
	return m.registry.Expose()
}

// Registry returns the underlying metrics registry.
func (m *ServerMetrics) Registry() *Registry {
	return m.registry
}

// RecordRequest records a request with its duration and error status.
func (m *ServerMetrics) RecordRequest(duration time.Duration, err error) {
	m.RequestsTotal.Inc()
	m.RequestDuration.ObserveDuration(duration)
	if err != nil {
		m.RequestErrors.Inc()
	}
}

// RecordParse records a parse operation with its duration and error status.
func (m *ServerMetrics) RecordParse(duration time.Duration, err error) {
	m.ParseDuration.ObserveDuration(duration)
	if err != nil {
		m.ParseErrors.Inc()
	}
}

// RecordCacheHit records a cache hit.
func (m *ServerMetrics) RecordCacheHit() {
	m.CacheHits.Inc()
}

// RecordCacheMiss records a cache miss.
func (m *ServerMetrics) RecordCacheMiss() {
	m.CacheMisses.Inc()
}

// SetActiveLSPServers sets the number of active LSP servers.
func (m *ServerMetrics) SetActiveLSPServers(count int64) {
	m.ActiveLSPServers.Set(count)
}
