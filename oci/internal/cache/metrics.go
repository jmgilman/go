package cache

import (
	"sync"
	"time"
)

// Operation types for metrics tracking
const (
	OperationManifest = "manifest"
	OperationBlob     = "blob"
)

// DetailedMetrics provides comprehensive metrics collection for cache operations.
// It tracks performance, bandwidth savings, and operational statistics.
type DetailedMetrics struct {
	mu sync.RWMutex

	// Core hit/miss statistics
	hits   int64
	misses int64

	// Eviction and error tracking
	evictions int64
	errors    int64

	// Size and entry tracking
	bytesStored    int64
	entriesStored  int64
	bytesRetrieved int64 // Total bytes served from cache

	// Bandwidth savings calculations
	networkRequests int64 // Total network requests made
	bytesDownloaded int64 // Total bytes downloaded from network
	bytesServed     int64 // Total bytes served from cache
	bandwidthSaved  int64 // Calculated bandwidth savings

	// Latency tracking (in nanoseconds)
	getLatencies    []time.Duration
	putLatencies    []time.Duration
	deleteLatencies []time.Duration

	// Operation counts by type
	manifestGets   int64
	manifestPuts   int64
	blobGets       int64
	blobPuts       int64
	tagResolutions int64

	// Time-based metrics
	startTime        time.Time
	lastHitTime      time.Time
	lastMissTime     time.Time
	lastEvictionTime time.Time
	lastErrorTime    time.Time

	// Peak usage tracking
	peakBytesStored   int64
	peakEntriesStored int64
	peakHitRate       float64
}

// NewDetailedMetrics creates a new DetailedMetrics instance.
func NewDetailedMetrics() *DetailedMetrics {
	now := time.Now()
	return &DetailedMetrics{
		startTime:        now,
		lastHitTime:      now,
		lastMissTime:     now,
		lastEvictionTime: now,
		lastErrorTime:    now,
		getLatencies:     make([]time.Duration, 0, 1000), // Pre-allocate capacity
		putLatencies:     make([]time.Duration, 0, 1000),
		deleteLatencies:  make([]time.Duration, 0, 1000),
	}
}

// RecordHit records a cache hit and updates relevant metrics.
func (m *DetailedMetrics) RecordHit(operationType string, bytesServed int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.hits++
	m.bytesServed += bytesServed
	m.bytesRetrieved += bytesServed
	m.lastHitTime = time.Now()

	// Update operation-specific counters
	switch operationType {
	case OperationManifest:
		m.manifestGets++
	case OperationBlob:
		m.blobGets++
	}

	// Update peak hit rate
	currentHitRate := m.calculateHitRate()
	if currentHitRate > m.peakHitRate {
		m.peakHitRate = currentHitRate
	}
}

// RecordMiss records a cache miss and updates network usage metrics.
func (m *DetailedMetrics) RecordMiss(operationType string, bytesDownloaded int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.misses++
	m.networkRequests++
	m.bytesDownloaded += bytesDownloaded
	m.lastMissTime = time.Now()

	// Update operation-specific counters
	switch operationType {
	case OperationManifest:
		m.manifestGets++
	case OperationBlob:
		m.blobGets++
	}
}

// RecordEviction records an eviction operation.
func (m *DetailedMetrics) RecordEviction(bytesEvicted int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.evictions++
	m.bytesStored -= bytesEvicted
	m.lastEvictionTime = time.Now()
}

// RecordError records an operation error.
func (m *DetailedMetrics) RecordError() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.errors++
	m.lastErrorTime = time.Now()
}

// RecordPut records a successful put operation.
func (m *DetailedMetrics) RecordPut(operationType string, bytesStored int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.bytesStored += bytesStored
	m.entriesStored++

	// Update operation-specific counters
	switch operationType {
	case OperationManifest:
		m.manifestPuts++
	case OperationBlob:
		m.blobPuts++
	}

	// Update peak usage
	if m.bytesStored > m.peakBytesStored {
		m.peakBytesStored = m.bytesStored
	}
	if m.entriesStored > m.peakEntriesStored {
		m.peakEntriesStored = m.entriesStored
	}
}

// RecordDelete records a deletion operation.
func (m *DetailedMetrics) RecordDelete(bytesRemoved int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.bytesStored -= bytesRemoved
	m.entriesStored--
	if m.entriesStored < 0 {
		m.entriesStored = 0
	}
}

// RecordLatency records the latency of an operation.
func (m *DetailedMetrics) RecordLatency(operation string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch operation {
	case "get":
		m.getLatencies = append(m.getLatencies, duration)
		if len(m.getLatencies) > 10000 { // Keep only last 10k measurements
			m.getLatencies = m.getLatencies[len(m.getLatencies)-5000:]
		}
	case "put":
		m.putLatencies = append(m.putLatencies, duration)
		if len(m.putLatencies) > 10000 {
			m.putLatencies = m.putLatencies[len(m.putLatencies)-5000:]
		}
	case "delete":
		m.deleteLatencies = append(m.deleteLatencies, duration)
		if len(m.deleteLatencies) > 10000 {
			m.deleteLatencies = m.deleteLatencies[len(m.deleteLatencies)-5000:]
		}
	}
}

// RecordTagResolution records a tag resolution operation.
func (m *DetailedMetrics) RecordTagResolution() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tagResolutions++
}

// calculateHitRate calculates the current hit rate.
func (m *DetailedMetrics) calculateHitRate() float64 {
	total := m.hits + m.misses
	if total == 0 {
		return 0.0
	}
	return float64(m.hits) / float64(total)
}

// GetSnapshot returns a thread-safe snapshot of current metrics.
func (m *DetailedMetrics) GetSnapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Calculate bandwidth savings (cache hits avoid network requests)
	// Assume each hit saves the average bytes per network request
	var avgBytesPerRequest int64
	if m.networkRequests > 0 {
		avgBytesPerRequest = m.bytesDownloaded / m.networkRequests
	}
	bandwidthSaved := m.hits * avgBytesPerRequest

	// Calculate average latencies
	var avgGetLatency, avgPutLatency, avgDeleteLatency time.Duration
	if len(m.getLatencies) > 0 {
		totalGet := time.Duration(0)
		for _, lat := range m.getLatencies {
			totalGet += lat
		}
		avgGetLatency = totalGet / time.Duration(len(m.getLatencies))
	}
	if len(m.putLatencies) > 0 {
		totalPut := time.Duration(0)
		for _, lat := range m.putLatencies {
			totalPut += lat
		}
		avgPutLatency = totalPut / time.Duration(len(m.putLatencies))
	}
	if len(m.deleteLatencies) > 0 {
		totalDelete := time.Duration(0)
		for _, lat := range m.deleteLatencies {
			totalDelete += lat
		}
		avgDeleteLatency = totalDelete / time.Duration(len(m.deleteLatencies))
	}

	return MetricsSnapshot{
		// Core statistics
		Hits:      m.hits,
		Misses:    m.misses,
		HitRate:   m.calculateHitRate(),
		Evictions: m.evictions,
		Errors:    m.errors,

		// Size metrics
		BytesStored:    m.bytesStored,
		EntriesStored:  m.entriesStored,
		BytesRetrieved: m.bytesRetrieved,

		// Bandwidth metrics
		NetworkRequests: m.networkRequests,
		BytesDownloaded: m.bytesDownloaded,
		BytesServed:     m.bytesServed,
		BandwidthSaved:  bandwidthSaved,

		// Operation counts
		ManifestGets:   m.manifestGets,
		ManifestPuts:   m.manifestPuts,
		BlobGets:       m.blobGets,
		BlobPuts:       m.blobPuts,
		TagResolutions: m.tagResolutions,

		// Latency metrics
		AverageGetLatency:    avgGetLatency,
		AveragePutLatency:    avgPutLatency,
		AverageDeleteLatency: avgDeleteLatency,

		// Time metrics
		Uptime:                time.Since(m.startTime),
		TimeSinceLastHit:      time.Since(m.lastHitTime),
		TimeSinceLastMiss:     time.Since(m.lastMissTime),
		TimeSinceLastEviction: time.Since(m.lastEvictionTime),
		TimeSinceLastError:    time.Since(m.lastErrorTime),

		// Peak metrics
		PeakBytesStored:   m.peakBytesStored,
		PeakEntriesStored: m.peakEntriesStored,
		PeakHitRate:       m.peakHitRate,

		// Sample counts for latency calculations
		GetLatencySamples:    len(m.getLatencies),
		PutLatencySamples:    len(m.putLatencies),
		DeleteLatencySamples: len(m.deleteLatencies),
	}
}

// MetricsSnapshot provides a point-in-time view of cache metrics.
type MetricsSnapshot struct {
	// Core hit/miss statistics
	Hits    int64   `json:"hits"`
	Misses  int64   `json:"misses"`
	HitRate float64 `json:"hit_rate"`

	// Eviction and error tracking
	Evictions int64 `json:"evictions"`
	Errors    int64 `json:"errors"`

	// Size and entry tracking
	BytesStored    int64 `json:"bytes_stored"`
	EntriesStored  int64 `json:"entries_stored"`
	BytesRetrieved int64 `json:"bytes_retrieved"`

	// Bandwidth metrics
	NetworkRequests int64 `json:"network_requests"`
	BytesDownloaded int64 `json:"bytes_downloaded"`
	BytesServed     int64 `json:"bytes_served"`
	BandwidthSaved  int64 `json:"bandwidth_saved"`

	// Operation counts by type
	ManifestGets   int64 `json:"manifest_gets"`
	ManifestPuts   int64 `json:"manifest_puts"`
	BlobGets       int64 `json:"blob_gets"`
	BlobPuts       int64 `json:"blob_puts"`
	TagResolutions int64 `json:"tag_resolutions"`

	// Latency metrics (in nanoseconds)
	AverageGetLatency    time.Duration `json:"avg_get_latency_ns"`
	AveragePutLatency    time.Duration `json:"avg_put_latency_ns"`
	AverageDeleteLatency time.Duration `json:"avg_delete_latency_ns"`

	// Time-based metrics
	Uptime                time.Duration `json:"uptime"`
	TimeSinceLastHit      time.Duration `json:"time_since_last_hit"`
	TimeSinceLastMiss     time.Duration `json:"time_since_last_miss"`
	TimeSinceLastEviction time.Duration `json:"time_since_last_eviction"`
	TimeSinceLastError    time.Duration `json:"time_since_last_error"`

	// Peak usage tracking
	PeakBytesStored   int64   `json:"peak_bytes_stored"`
	PeakEntriesStored int64   `json:"peak_entries_stored"`
	PeakHitRate       float64 `json:"peak_hit_rate"`

	// Sample counts for latency calculations
	GetLatencySamples    int `json:"get_latency_samples"`
	PutLatencySamples    int `json:"put_latency_samples"`
	DeleteLatencySamples int `json:"delete_latency_samples"`
}

// Reset clears all metrics data.
func (m *DetailedMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// Reset all fields individually to avoid mutex issues
	m.hits = 0
	m.misses = 0
	m.evictions = 0
	m.errors = 0
	m.bytesStored = 0
	m.entriesStored = 0
	m.bytesRetrieved = 0
	m.networkRequests = 0
	m.bytesDownloaded = 0
	m.bytesServed = 0
	m.bandwidthSaved = 0
	m.manifestGets = 0
	m.manifestPuts = 0
	m.blobGets = 0
	m.blobPuts = 0
	m.tagResolutions = 0
	m.startTime = now
	m.lastHitTime = now
	m.lastMissTime = now
	m.lastEvictionTime = now
	m.lastErrorTime = now
	m.peakBytesStored = 0
	m.peakEntriesStored = 0
	m.peakHitRate = 0.0

	// Clear latency slices
	m.getLatencies = m.getLatencies[:0]
	m.putLatencies = m.putLatencies[:0]
	m.deleteLatencies = m.deleteLatencies[:0]
}
