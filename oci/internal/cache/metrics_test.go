package cache

import (
	"testing"
	"time"
)

func TestDetailedMetrics_RecordHit(t *testing.T) {
	m := NewDetailedMetrics()

	// Record some hits
	m.RecordHit("manifest", 1024)
	m.RecordHit("blob", 2048)
	m.RecordHit("manifest", 512)

	snapshot := m.GetSnapshot()

	if snapshot.Hits != 3 {
		t.Errorf("Expected 3 hits, got %d", snapshot.Hits)
	}

	if snapshot.BytesServed != 3584 {
		t.Errorf("Expected 3584 bytes served, got %d", snapshot.BytesServed)
	}

	if snapshot.ManifestGets != 2 {
		t.Errorf("Expected 2 manifest gets, got %d", snapshot.ManifestGets)
	}

	if snapshot.BlobGets != 1 {
		t.Errorf("Expected 1 blob get, got %d", snapshot.BlobGets)
	}
}

func TestDetailedMetrics_RecordMiss(t *testing.T) {
	m := NewDetailedMetrics()

	// Record some misses
	m.RecordMiss("manifest", 1024)
	m.RecordMiss("blob", 2048)

	snapshot := m.GetSnapshot()

	if snapshot.Misses != 2 {
		t.Errorf("Expected 2 misses, got %d", snapshot.Misses)
	}

	if snapshot.NetworkRequests != 2 {
		t.Errorf("Expected 2 network requests, got %d", snapshot.NetworkRequests)
	}

	if snapshot.BytesDownloaded != 3072 {
		t.Errorf("Expected 3072 bytes downloaded, got %d", snapshot.BytesDownloaded)
	}
}

func TestDetailedMetrics_HitRate(t *testing.T) {
	m := NewDetailedMetrics()

	// No hits or misses yet
	snapshot := m.GetSnapshot()
	if snapshot.HitRate != 0.0 {
		t.Errorf("Expected 0.0 hit rate, got %f", snapshot.HitRate)
	}

	// Add some hits and misses
	m.RecordHit("manifest", 100)
	m.RecordHit("blob", 100)
	m.RecordMiss("manifest", 100)
	m.RecordMiss("blob", 100)

	snapshot = m.GetSnapshot()
	expectedRate := 0.5 // 2 hits out of 4 total
	if snapshot.HitRate != expectedRate {
		t.Errorf("Expected %f hit rate, got %f", expectedRate, snapshot.HitRate)
	}
}

func TestDetailedMetrics_RecordEviction(t *testing.T) {
	m := NewDetailedMetrics()

	// Add some data first
	m.RecordPut("blob", 1024)

	// Record eviction
	m.RecordEviction(512)

	snapshot := m.GetSnapshot()

	if snapshot.Evictions != 1 {
		t.Errorf("Expected 1 eviction, got %d", snapshot.Evictions)
	}

	if snapshot.BytesStored != 512 {
		t.Errorf("Expected 512 bytes stored after eviction, got %d", snapshot.BytesStored)
	}
}

func TestDetailedMetrics_RecordLatency(t *testing.T) {
	m := NewDetailedMetrics()

	latency1 := 100 * time.Millisecond
	latency2 := 200 * time.Millisecond

	m.RecordLatency("get", latency1)
	m.RecordLatency("get", latency2)
	m.RecordLatency("put", 50*time.Millisecond)

	snapshot := m.GetSnapshot()

	expectedAvgGet := 150 * time.Millisecond
	if snapshot.AverageGetLatency != expectedAvgGet {
		t.Errorf("Expected average get latency %v, got %v", expectedAvgGet, snapshot.AverageGetLatency)
	}

	if snapshot.GetLatencySamples != 2 {
		t.Errorf("Expected 2 get latency samples, got %d", snapshot.GetLatencySamples)
	}

	if snapshot.PutLatencySamples != 1 {
		t.Errorf("Expected 1 put latency sample, got %d", snapshot.PutLatencySamples)
	}
}

func TestDetailedMetrics_BandwidthSavings(t *testing.T) {
	m := NewDetailedMetrics()

	// Record some network activity (misses)
	m.RecordMiss("blob", 1024) // Downloads 1024 bytes
	m.RecordMiss("blob", 2048) // Downloads 2048 bytes

	// Record some cache hits that would have required similar downloads
	m.RecordHit("blob", 1024) // Would have downloaded ~1536 bytes (avg)
	m.RecordHit("blob", 1024) // Would have downloaded ~1536 bytes (avg)

	snapshot := m.GetSnapshot()

	// Average bytes per request: (1024 + 2048) / 2 = 1536
	// Bandwidth saved: 2 hits * 1536 = 3072
	if snapshot.BandwidthSaved != 3072 {
		t.Errorf("Expected 3072 bytes bandwidth saved, got %d", snapshot.BandwidthSaved)
	}
}

func TestDetailedMetrics_PeakTracking(t *testing.T) {
	m := NewDetailedMetrics()

	// Add entries and check peak tracking
	m.RecordPut("blob", 1024)
	m.RecordPut("blob", 2048) // Total: 3072 bytes, 2 entries

	snapshot := m.GetSnapshot()

	if snapshot.PeakBytesStored != 3072 {
		t.Errorf("Expected peak bytes stored 3072, got %d", snapshot.PeakBytesStored)
	}

	if snapshot.PeakEntriesStored != 2 {
		t.Errorf("Expected peak entries stored 2, got %d", snapshot.PeakEntriesStored)
	}

	// Remove an entry
	m.RecordDelete(1024)

	// Peak should remain the same
	snapshot = m.GetSnapshot()
	if snapshot.PeakBytesStored != 3072 {
		t.Errorf("Expected peak bytes stored to remain 3072, got %d", snapshot.PeakBytesStored)
	}
}

func TestDetailedMetrics_PeakHitRate(t *testing.T) {
	m := NewDetailedMetrics()

	// Build up to a high hit rate
	m.RecordHit("manifest", 100)
	m.RecordHit("manifest", 100)
	m.RecordHit("manifest", 100) // 3 hits, 0 misses = 100% hit rate

	snapshot := m.GetSnapshot()
	if snapshot.PeakHitRate != 1.0 {
		t.Errorf("Expected peak hit rate 1.0, got %f", snapshot.PeakHitRate)
	}

	// Add a miss, hit rate drops but peak remains
	m.RecordMiss("manifest", 100) // Now 3 hits, 1 miss = 75% hit rate

	snapshot = m.GetSnapshot()
	if snapshot.HitRate != 0.75 {
		t.Errorf("Expected current hit rate 0.75, got %f", snapshot.HitRate)
	}

	if snapshot.PeakHitRate != 1.0 {
		t.Errorf("Expected peak hit rate to remain 1.0, got %f", snapshot.PeakHitRate)
	}
}

func TestDetailedMetrics_Reset(t *testing.T) {
	m := NewDetailedMetrics()

	// Add some data
	m.RecordHit("manifest", 1024)
	m.RecordMiss("blob", 2048)
	m.RecordPut("blob", 512)
	m.RecordLatency("get", 100*time.Millisecond)

	// Verify data exists
	snapshot := m.GetSnapshot()
	if snapshot.Hits != 1 || snapshot.Misses != 1 || snapshot.BytesStored != 512 {
		t.Errorf("Data not recorded correctly before reset")
	}

	// Reset
	m.Reset()

	// Verify data is cleared
	snapshot = m.GetSnapshot()
	if snapshot.Hits != 0 || snapshot.Misses != 0 || snapshot.BytesStored != 0 {
		t.Errorf("Data not cleared after reset")
	}

	if snapshot.GetLatencySamples != 0 {
		t.Errorf("Latency samples not cleared after reset")
	}
}

func TestDetailedMetrics_ThreadSafety(t *testing.T) {
	m := NewDetailedMetrics()

	// Run concurrent operations
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				m.RecordHit("manifest", 100)
				m.RecordMiss("blob", 200)
				m.RecordLatency("get", time.Duration(j)*time.Millisecond)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	snapshot := m.GetSnapshot()

	if snapshot.Hits != 1000 {
		t.Errorf("Expected 1000 hits in concurrent test, got %d", snapshot.Hits)
	}

	if snapshot.Misses != 1000 {
		t.Errorf("Expected 1000 misses in concurrent test, got %d", snapshot.Misses)
	}

	if snapshot.GetLatencySamples != 1000 {
		t.Errorf("Expected 1000 latency samples in concurrent test, got %d", snapshot.GetLatencySamples)
	}
}
