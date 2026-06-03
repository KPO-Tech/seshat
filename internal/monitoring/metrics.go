package monitoring

import (
	"sync"
	"time"
)

// MetricType represents the type of metric
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"   // Counting events
	MetricTypeGauge     MetricType = "gauge"     // Point-in-time values
	MetricTypeHistogram MetricType = "histogram" // Distribution of values
	MetricTypeTimer     MetricType = "timer"     // Duration measurements
)

// Metric represents a single metric with labels
type Metric struct {
	// Name is the metric name
	Name string `json:"name"`

	// Type is the metric type
	Type MetricType `json:"type"`

	// Value is the metric value
	Value float64 `json:"value"`

	// Labels are key-value pairs for categorization
	Labels map[string]string `json:"labels"`

	// Timestamp is when the metric was recorded
	Timestamp time.Time `json:"timestamp"`
}

// Counter is a metric that only increases
type Counter struct {
	name   string
	labels map[string]string
	value  uint64
	mu     sync.RWMutex
}

// NewCounter creates a new counter metric
func NewCounter(name string, labels map[string]string) *Counter {
	return &Counter{
		name:   name,
		labels: labels,
		value:  0,
	}
}

// Add adds a value to the counter
func (c *Counter) Add(delta uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value += delta
}

// Inc increments the counter by 1
func (c *Counter) Inc() {
	c.Add(1)
}

// Value returns the current counter value
func (c *Counter) Value() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.value
}

// Reset resets the counter to 0
func (c *Counter) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value = 0
}

// ToMetric converts the counter to a metric
func (c *Counter) ToMetric() Metric {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return Metric{
		Name:      c.name,
		Type:      MetricTypeCounter,
		Value:     float64(c.value),
		Labels:    c.labels,
		Timestamp: time.Now(),
	}
}

// Gauge is a metric that can go up or down
type Gauge struct {
	name   string
	labels map[string]string
	value  float64
	mu     sync.RWMutex
}

// NewGauge creates a new gauge metric
func NewGauge(name string, labels map[string]string) *Gauge {
	return &Gauge{
		name:   name,
		labels: labels,
		value:  0,
	}
}

// Set sets the gauge value
func (g *Gauge) Set(value float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value = value
}

// Add adds a value to the gauge
func (g *Gauge) Add(delta float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value += delta
}

// Value returns the current gauge value
func (g *Gauge) Value() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.value
}

// ToMetric converts the gauge to a metric
func (g *Gauge) ToMetric() Metric {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return Metric{
		Name:      g.name,
		Type:      MetricTypeGauge,
		Value:     g.value,
		Labels:    g.labels,
		Timestamp: time.Now(),
	}
}

// Histogram tracks the distribution of values
type Histogram struct {
	name    string
	labels  map[string]string
	buckets []float64
	counts  []uint64
	count   uint64
	sum     float64
	mu      sync.RWMutex
}

// NewHistogram creates a new histogram metric
func NewHistogram(name string, labels map[string]string, buckets []float64) *Histogram {
	return &Histogram{
		name:    name,
		labels:  labels,
		buckets: buckets,
		counts:  make([]uint64, len(buckets)+1),
	}
}

// Observe records a value in the histogram
func (h *Histogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.count++
	h.sum += value

	// Find the right bucket
	bucketIdx := len(h.buckets) // Overflow bucket
	for i, upperBound := range h.buckets {
		if value <= upperBound {
			bucketIdx = i
			break
		}
	}

	h.counts[bucketIdx]++
}

// Count returns the total number of observations
func (h *Histogram) Count() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.count
}

// Sum returns the sum of all observations
func (h *Histogram) Sum() float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.sum
}

// Average returns the average value
func (h *Histogram) Average() float64 {
	count := h.Count()
	if count == 0 {
		return 0
	}
	return h.Sum() / float64(count)
}

// Percentile calculates the approximate percentile
func (h *Histogram) Percentile(percentile float64) float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.counts) == 0 {
		return 0
	}

	total := uint64(0)
	for _, count := range h.counts {
		total += count
	}

	if total == 0 {
		return 0
	}

	threshold := uint64(float64(total) * percentile / 100)
	if threshold == 0 {
		threshold = 1
	}
	counted := uint64(0)

	for i, count := range h.counts {
		counted += count
		if counted >= threshold {
			if i >= len(h.buckets) {
				return h.buckets[len(h.buckets)-1]
			}
			return h.buckets[i]
		}
	}

	return h.buckets[len(h.buckets)-1]
}

// Reset clears all observations
func (h *Histogram) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.counts = make([]uint64, len(h.buckets)+1)
	h.count = 0
	h.sum = 0
}

// ToMetric converts the histogram to a metric
func (h *Histogram) ToMetric() Metric {
	h.mu.RLock()
	defer h.mu.RUnlock()

	value := 0.0
	if h.count > 0 {
		value = h.sum / float64(h.count)
	}

	return Metric{
		Name:      h.name,
		Type:      MetricTypeHistogram,
		Value:     value,
		Labels:    h.labels,
		Timestamp: time.Now(),
	}
}

// Timer measures duration of operations
type Timer struct {
	name      string
	labels    map[string]string
	histogram *Histogram
}

// NewTimer creates a new timer metric
func NewTimer(name string, labels map[string]string) *Timer {
	// Default buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
	buckets := []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

	return &Timer{
		name:      name,
		labels:    labels,
		histogram: NewHistogram(name, labels, buckets),
	}
}

// Record records a duration in milliseconds
func (t *Timer) Record(duration time.Duration) {
	ms := float64(duration.Milliseconds())
	t.histogram.Observe(ms)
}

// Start returns a function that records the elapsed time
func (t *Timer) Start() func() {
	start := time.Now()
	return func() {
		duration := time.Since(start)
		t.Record(duration)
	}
}

// Count returns the total number of recordings
func (t *Timer) Count() uint64 {
	return t.histogram.Count()
}

// Average returns the average duration in milliseconds
func (t *Timer) Average() float64 {
	return t.histogram.Average()
}

// Percentile returns the duration at the given percentile
func (t *Timer) Percentile(percentile float64) time.Duration {
	ms := t.histogram.Percentile(percentile)
	return time.Duration(ms * float64(time.Millisecond))
}

// Reset clears all recordings
func (t *Timer) Reset() {
	t.histogram.Reset()
}

// ToMetric converts the timer to a metric
func (t *Timer) ToMetric() Metric {
	return t.histogram.ToMetric()
}
