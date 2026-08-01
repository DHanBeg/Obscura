// mempool.go — BFT blok önerileri için bekleyen-işlem kuyruğu.
//
// ADR-0017: BFT kendi mempool'unu icat etmiyor, ama sequencer paketinin
// Merkle-root mekanizmasını (sequencer.ComputeMerkleRoot) tekrar kullanıyor.
// Mempool burada TAMAMEN jenerik — hangi string'in "işlem" olduğuna dair bir
// varsayımı yok (opaque opaque op-hash/op-ID listesi). Gerçek OBS ledger
// operasyonlarının (mint/transfer/stake/slash) buraya nasıl besleneceği ve
// commit sonrası nasıl uygulanacağı ADIM 7'nin kapsamı.
package consensus

import "sync"

// Mempool — thread-safe, bekleyen op-ID kuyruğu.
type Mempool struct {
	mu  sync.Mutex
	ops []string
}

// NewMempool — boş mempool.
func NewMempool() *Mempool {
	return &Mempool{}
}

// Add, bekleyen bir operasyonu kuyruğa ekler (örn. bir ledger tx'inin ID'si).
func (m *Mempool) Add(opID string) {
	if opID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ops = append(m.ops, opID)
}

// Len, kuyruktaki bekleyen op sayısını döner.
func (m *Mempool) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.ops)
}

// Drain, kuyruktaki TÜM op'ları döner ve kuyruğu boşaltır. Bir blok
// önerisine dahil edilecek op'ları almak için kullanılır — çağıran taraf
// bunları ProposeBlock'un txRoot'unu hesaplamak (sequencer.ComputeMerkleRoot)
// ve commit sonrası uygulamak için saklamakla sorumludur (bkz. ADIM 7).
func (m *Mempool) Drain() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.ops) == 0 {
		return nil
	}
	out := m.ops
	m.ops = nil
	return out
}
