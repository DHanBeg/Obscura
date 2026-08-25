package consensus

// integration_test.go — ADIM 9: parçaların (mempool, proposer, transport,
// oy toplama, persistence) BİRLİKTE gerçekten çalıştığını doğrulayan uçtan
// uca testler. Diğer test dosyaları her parçayı izole test ediyor; burası
// "gerçek bir LocalTransport üzerinden propose→prevote→precommit→commit
// döngüsü gerçekten tamamlanıyor mu" sorusuna cevap veriyor.

import (
	"sync"
	"testing"
	"time"
)

// waitFor, cond true dönene kadar kısa aralıklarla dener; timeout'ta test'i
// FailNow ile durdurur. Engine'in kendi goroutine'i (loop()) üzerinden
// asenkron işlediği mesajları beklemek için kullanılır.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("zaman aşımı — beklenen koşul gerçekleşmedi")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestEndToEnd_SingleNode_ProposeVoteCommitPersist — tek-node döngüsü
// (ADIM 0-2-6-7-8'in TAMAMI birlikte): quorum=1, proposer her zaman self.
// ProposeBlock çağrısından gerçek DB'ye kalıcılaştırılmış bloğa kadar TÜM
// zincir (self-prevote → self-precommit → commit → SaveBlock/SaveBlockOps)
// gerçek LocalTransport üzerinden uçtan uca doğrulanır.
func TestEndToEnd_SingleNode_ProposeVoteCommitPersist(t *testing.T) {
	db := newTestDB(t)
	transport := NewLocalTransport()

	var mu sync.Mutex
	var committed []Block

	e := NewEngine(
		"node-1", 1, // quorum=1 — gerçek tek-node
		func(b Block) {
			mu.Lock()
			committed = append(committed, b)
			mu.Unlock()
			if err := SaveBlock(db, b, "2026-08-02T00:00:00Z"); err != nil {
				t.Errorf("SaveBlock: %v", err)
			}
			if err := SaveBlockOps(db, b.Height, b.Ops, "2026-08-02T00:00:00Z"); err != nil {
				t.Errorf("SaveBlockOps: %v", err)
			}
		},
		transport.Publish,
		transport.Subscribe,
		func() string { return "node-1" }, // her zaman proposer
		func(h uint64) string {
			_, hash, _ := LatestBlockHash(db)
			return hash
		},
		nil, nil,
	)
	if err := e.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := e.ProposeBlock([]string{"op-1", "op-2"}); err != nil {
		t.Fatalf("ProposeBlock: %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(committed) == 1
	})

	if e.Height() != 2 {
		t.Fatalf("commit sonrası height=2 beklenirdi, got=%d", e.Height())
	}

	height, hash, err := LatestBlockHash(db)
	if err != nil {
		t.Fatalf("LatestBlockHash: %v", err)
	}
	if height != 1 {
		t.Fatalf("DB'de height=1 beklenirdi, got=%d", height)
	}
	if hash == "" {
		t.Fatal("DB'de commit edilmiş bloğun hash'i boş")
	}

	ops, err := OpsForBlock(db, 1)
	if err != nil {
		t.Fatalf("OpsForBlock: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("audit-log'da 2 op beklenirdi, got=%v", ops)
	}
}

// TestEndToEnd_TwoNodes_ReachQuorumAndBothCommit — GERÇEK çok-taraflı oy
// toplama: 2 bağımsız Engine, PAYLAŞILAN bir LocalTransport üzerinden
// konuşuyor, quorum=2. node-A'nın önerisi her iki node'a da ulaşıyor, her
// ikisi de prevote/precommit yayınlıyor, her ikisi de diğerinin oyunu görüp
// quorum'a ulaşıyor ve İKİSİ DE bağımsız olarak commit ediyor. Transport/
// imza/stake henüz eklenmedi (ADIM 3-4-5, 2. node gerçek gelene ertelendi)
// ama oy-toplama/quorum mantığının BİRDEN FAZLA katılımcıyla çalıştığını
// bu test kanıtlıyor.
func TestEndToEnd_TwoNodes_ReachQuorumAndBothCommit(t *testing.T) {
	transport := NewLocalTransport() // node-A ve node-B AYNI transport'u paylaşıyor

	var muA, muB sync.Mutex
	var committedA, committedB []Block

	proposerFn := func() string { return "node-A" }

	eA := NewEngine("node-A", 2,
		func(b Block) { muA.Lock(); committedA = append(committedA, b); muA.Unlock() },
		transport.Publish, transport.Subscribe, proposerFn, nil, nil, nil)
	eB := NewEngine("node-B", 2,
		func(b Block) { muB.Lock(); committedB = append(committedB, b); muB.Unlock() },
		transport.Publish, transport.Subscribe, proposerFn, nil, nil, nil)

	if err := eA.Start(); err != nil {
		t.Fatalf("eA.Start: %v", err)
	}
	if err := eB.Start(); err != nil {
		t.Fatalf("eB.Start: %v", err)
	}

	if err := eA.ProposeBlock([]string{"op-1"}); err != nil {
		t.Fatalf("ProposeBlock: %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		muA.Lock()
		muB.Lock()
		defer muA.Unlock()
		defer muB.Unlock()
		return len(committedA) == 1 && len(committedB) == 1
	})

	muA.Lock()
	muB.Lock()
	defer muA.Unlock()
	defer muB.Unlock()
	if committedA[0].Hash != committedB[0].Hash {
		t.Fatalf("iki node FARKLI blok commit etti — node-A=%s node-B=%s", committedA[0].Hash, committedB[0].Hash)
	}
	if committedA[0].Proposer != "node-A" {
		t.Fatalf("commit edilen bloğun proposer'ı node-A olmalıydı, got=%s", committedA[0].Proposer)
	}
}
