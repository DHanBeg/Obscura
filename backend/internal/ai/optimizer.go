// Package ai — AI-powered node optimization (FAZ 4, spec 12.4)
//
// Makine öğrenmesi tabanlı peer seçimi, yük tahmini ve rota optimizasyonu.
// Algoritma: Online gradient descent üzerinde basit exponential smoothing
// (ağırlıklı ortalama). Gerçek sinir ağı FAZ 4 GA'da ONNX runtime ile eklenecek.
package ai

import (
	"math"
	"sort"
	"sync"
	"time"
)

// NodeMetrics — bir node'un gözlemlenen metrikleri
type NodeMetrics struct {
	NodeID      string
	Latency     float64 // ms (son ölçüm)
	AvgLatency  float64 // ms (EMA)
	Throughput  float64 // msg/s
	ErrorRate   float64 // 0.0-1.0
	Uptime      float64 // 0.0-1.0
	LastUpdated time.Time
}

// Score — node'un optimize edilmiş skoru (düşük = daha iyi)
func (m *NodeMetrics) Score() float64 {
	// Ağırlıklı kompozit skor: gecikme öncelikli
	// score = 0.5*latency_norm + 0.3*error_rate + 0.2*(1-uptime)
	latencyNorm := math.Min(m.AvgLatency/1000.0, 1.0) // 0-1000ms normalize
	return 0.5*latencyNorm + 0.3*m.ErrorRate + 0.2*(1.0-m.Uptime)
}

const emaAlpha = 0.3 // EMA smoothing factor

// Optimizer — online node metrikleri ve peer seçim motoru
type Optimizer struct {
	mu      sync.RWMutex
	metrics map[string]*NodeMetrics

	// Load prediction: ring buffer of recent msg counts
	loadRing []float64
	loadIdx  int
	loadSize int
}

// NewOptimizer — yeni AI optimizer
func NewOptimizer(ringSize int) *Optimizer {
	if ringSize <= 0 {
		ringSize = 60 // 60 saniyelik pencere
	}
	return &Optimizer{
		metrics:  make(map[string]*NodeMetrics),
		loadRing: make([]float64, ringSize),
		loadSize: ringSize,
	}
}

// RecordLatency — bir node için gecikme ölçümü kaydet
func (o *Optimizer) RecordLatency(nodeID string, latencyMS float64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	m := o.getOrCreate(nodeID)
	m.Latency = latencyMS
	// EMA güncelleme
	if m.AvgLatency == 0 {
		m.AvgLatency = latencyMS
	} else {
		m.AvgLatency = emaAlpha*latencyMS + (1-emaAlpha)*m.AvgLatency
	}
	m.LastUpdated = time.Now()
}

// RecordError — node hata oranını güncelle
func (o *Optimizer) RecordError(nodeID string, isError bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	m := o.getOrCreate(nodeID)
	if isError {
		m.ErrorRate = emaAlpha*1.0 + (1-emaAlpha)*m.ErrorRate
	} else {
		m.ErrorRate = emaAlpha*0.0 + (1-emaAlpha)*m.ErrorRate
	}
	m.LastUpdated = time.Now()
}

// RecordUptime — node uptime'ını güncelle (true=up, false=down)
func (o *Optimizer) RecordUptime(nodeID string, up bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	m := o.getOrCreate(nodeID)
	v := 0.0
	if up {
		v = 1.0
	}
	m.Uptime = emaAlpha*v + (1-emaAlpha)*m.Uptime
	m.LastUpdated = time.Now()
}

// RecordLoad — mevcut mesaj yükünü ring buffer'a ekle
func (o *Optimizer) RecordLoad(msgCount float64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.loadRing[o.loadIdx%o.loadSize] = msgCount
	o.loadIdx++
}

// PredictLoad — gelecek 60 saniye için yük tahmini (linear trend)
func (o *Optimizer) PredictLoad() float64 {
	o.mu.RLock()
	defer o.mu.RUnlock()

	n := float64(o.loadSize)
	if n == 0 {
		return 0
	}
	// Basit lineer regresyon: y = a + b*x
	var sumX, sumY, sumXY, sumX2 float64
	for i := 0; i < o.loadSize; i++ {
		x := float64(i)
		y := o.loadRing[i]
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}
	denom := n*sumX2 - sumX*sumX
	if math.Abs(denom) < 1e-9 {
		return sumY / n
	}
	b := (n*sumXY - sumX*sumY) / denom
	a := (sumY - b*sumX) / n
	return a + b*n // tahmin: bir sonraki nokta
}

// SelectPeers — skor bazlı en iyi N peer seç
func (o *Optimizer) SelectPeers(n int) []NodeMetrics {
	o.mu.RLock()
	defer o.mu.RUnlock()

	nodes := make([]NodeMetrics, 0, len(o.metrics))
	for _, m := range o.metrics {
		// 10 dakikadan eski veriyi atla
		if time.Since(m.LastUpdated) > 10*time.Minute {
			continue
		}
		nodes = append(nodes, *m)
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Score() < nodes[j].Score() // düşük skor = daha iyi
	})

	if n > len(nodes) {
		n = len(nodes)
	}
	return nodes[:n]
}

// AllMetrics — tüm node metriklerini döndür
func (o *Optimizer) AllMetrics() []NodeMetrics {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]NodeMetrics, 0, len(o.metrics))
	for _, m := range o.metrics {
		out = append(out, *m)
	}
	return out
}

func (o *Optimizer) getOrCreate(nodeID string) *NodeMetrics {
	m, ok := o.metrics[nodeID]
	if !ok {
		m = &NodeMetrics{
			NodeID: nodeID,
			Uptime: 1.0,
		}
		o.metrics[nodeID] = m
	}
	return m
}

// GlobalOptimizer — singleton optimizer
var GlobalOptimizer = NewOptimizer(60)
