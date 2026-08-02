package sequencer

// vrf_broadcast_test.go — ADR-0017 adım 5: proof yayını/toplama altyapısı.
// İki bağımsız Sequencer instance'ını sahte (in-memory) bir GossipSub gibi
// birbirine bağlayıp gerçek proof exchange + doğrulama akışını test eder.

import (
	"testing"
	"time"
)

// fakeBus — SetVRFTransport'un beklediği publishFn/subscribeFn imzalarını
// gerçek p2p olmadan simüle eden, tek topic'lik in-memory pub/sub.
type fakeBus struct {
	subs []chan<- []byte
}

func (b *fakeBus) publish(topic string, data []byte) error {
	for _, ch := range b.subs {
		ch <- data
	}
	return nil
}

func (b *fakeBus) subscribe(topic string, ch chan<- []byte) error {
	b.subs = append(b.subs, ch)
	return nil
}

func TestPublishOwnProof_NoTransportIsNoop(t *testing.T) {
	s := NewSequencer("node-1", time.Hour)
	if err := s.PublishOwnProof(42); err != nil {
		t.Fatalf("transport yokken PublishOwnProof hata vermemeliydi: %v", err)
	}
	proofs := s.ProofsForEpoch(42)
	if len(proofs) != 1 || proofs["node-1"] == "" {
		t.Fatalf("transport olmasa da kendi proof'u yerel store'a eklenmeliydi, got=%+v", proofs)
	}
}

// TestVRFProofExchange_TwoNodesCollectEachOthers — iki bağımsız Sequencer,
// sahte bus üzerinden birbirine bağlanınca, her ikisi de HEM kendi HEM
// karşı tarafın proof'unu ProofsForEpoch'ta görmeli (doğrulanmış olarak).
func TestVRFProofExchange_TwoNodesCollectEachOthers(t *testing.T) {
	nodeA := NewSequencer("node-1", time.Hour)
	nodeB := NewSequencer("node-2", time.Hour)

	// Her node diğerinin vrf_pubkey'ini "federation'dan öğrenmiş" gibi
	// currentNodes() listesine ekliyoruz (stakeFn ile enjekte).
	pubA := nodeA.VRFPublicKeyHex()
	pubB := nodeB.VRFPublicKeyHex()
	nodeA.SetStakeLookup(func() []NodeInfo {
		return []NodeInfo{{NodeID: "node-1", Stake: 1, VRFPubkey: pubA}, {NodeID: "node-2", Stake: 1, VRFPubkey: pubB}}
	})
	nodeB.SetStakeLookup(func() []NodeInfo {
		return []NodeInfo{{NodeID: "node-1", Stake: 1, VRFPubkey: pubA}, {NodeID: "node-2", Stake: 1, VRFPubkey: pubB}}
	})

	bus := &fakeBus{}
	if err := nodeA.SetVRFTransport(bus.publish, bus.subscribe); err != nil {
		t.Fatalf("nodeA SetVRFTransport: %v", err)
	}
	if err := nodeB.SetVRFTransport(bus.publish, bus.subscribe); err != nil {
		t.Fatalf("nodeB SetVRFTransport: %v", err)
	}

	const epoch = 7
	if err := nodeA.PublishOwnProof(epoch); err != nil {
		t.Fatalf("nodeA PublishOwnProof: %v", err)
	}
	if err := nodeB.PublishOwnProof(epoch); err != nil {
		t.Fatalf("nodeB PublishOwnProof: %v", err)
	}

	// fakeBus.publish senkron (kanal üzerinden anında yazıyor) ama alıcı
	// goroutine (SetVRFTransport'taki for-range) küçük bir gecikmeyle işler.
	deadline := time.Now().Add(2 * time.Second)
	for {
		pa := nodeA.ProofsForEpoch(epoch)
		pb := nodeB.ProofsForEpoch(epoch)
		if len(pa) == 2 && len(pb) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("2sn içinde her iki node da 2 proof toplayamadı: nodeA=%d nodeB=%d", len(pa), len(pb))
		}
		time.Sleep(10 * time.Millisecond)
	}

	pa := nodeA.ProofsForEpoch(epoch)
	pb := nodeB.ProofsForEpoch(epoch)
	if pa["node-1"] == "" || pa["node-2"] == "" {
		t.Fatalf("nodeA her iki proof'u da görmeliydi, got=%+v", pa)
	}
	if pb["node-1"] == "" || pb["node-2"] == "" {
		t.Fatalf("nodeB her iki proof'u da görmeliydi, got=%+v", pb)
	}
	// Aynı node için her iki tarafta da AYNI proof görülmeli (gossip'te değişmedi).
	if pa["node-2"] != pb["node-2"] {
		t.Fatalf("node-2'nin proof'u iki tarafta farklı: A=%s B=%s", pa["node-2"], pb["node-2"])
	}
}

// TestHandleIncomingVRFProof_UnknownNodeIsRejected — vrf_pubkey'i bilinmeyen
// bir node_id'den gelen proof toplanmamalı (sahte/tanınmayan gönderici).
func TestHandleIncomingVRFProof_UnknownNodeIsRejected(t *testing.T) {
	s := NewSequencer("node-1", time.Hour)
	s.SetStakeLookup(func() []NodeInfo { return []NodeInfo{{NodeID: "node-1", Stake: 1}} })

	attacker := NewSequencer("mallory", time.Hour)
	proofHex, _ := attacker.VRFProof(3)

	raw := []byte(`{"node_id":"mallory","epoch":3,"proof_hex":"` + proofHex + `"}`)
	s.handleIncomingVRFProof(raw)

	if got := s.ProofsForEpoch(3); len(got) != 0 {
		t.Fatalf("bilinmeyen node_id'den gelen proof reddedilmeliydi, got=%+v", got)
	}
}

// TestHandleIncomingVRFProof_ForgedProofIsRejected — bilinen bir node_id
// adına ama YANLIŞ anahtarla üretilmiş (sahte) proof reddedilmeli — asıl
// bug buydu: eskiden herkes node_id'den anahtarı hesaplayıp sahte proof
// üretebilirdi. Şimdi gerçek pubkey ile eşleşmeyen proof geçmemeli.
func TestHandleIncomingVRFProof_ForgedProofIsRejected(t *testing.T) {
	s := NewSequencer("node-1", time.Hour)
	real := NewSequencer("node-2", time.Hour)
	forger := NewSequencer("forger", time.Hour) // "node-2" adına ama forger'ın anahtarıyla üretecek

	s.SetStakeLookup(func() []NodeInfo {
		return []NodeInfo{{NodeID: "node-1", Stake: 1}, {NodeID: "node-2", Stake: 1, VRFPubkey: real.VRFPublicKeyHex()}}
	})

	forgedProofHex, _ := forger.VRFProof(9) // gerçek node-2'nin anahtarıyla DEĞİL
	raw := []byte(`{"node_id":"node-2","epoch":9,"proof_hex":"` + forgedProofHex + `"}`)
	s.handleIncomingVRFProof(raw)

	if got := s.ProofsForEpoch(9); len(got) != 0 {
		t.Fatalf("sahte proof (yanlış anahtarla üretilmiş) reddedilmeliydi, got=%+v", got)
	}
}

// TestStoreProofLocked_PrunesOldEpochs — sınırsız büyümesin diye eski
// epoch'lar budanmalı (maxRetainedEpochs).
func TestStoreProofLocked_PrunesOldEpochs(t *testing.T) {
	s := NewSequencer("node-1", time.Hour)
	s.mu.Lock()
	s.vrfProofs = newVRFProofStore()
	for e := uint64(1); e <= 10; e++ {
		s.storeProofLocked(e, "node-1", "deadbeef")
	}
	s.mu.Unlock()

	if len(s.vrfProofs.byEpoch) > maxRetainedEpochs+1 {
		t.Fatalf("eski epoch'lar budanmamış, kalan epoch sayısı=%d", len(s.vrfProofs.byEpoch))
	}
	if _, ok := s.vrfProofs.byEpoch[1]; ok {
		t.Fatal("epoch=1 çoktan budanmış olmalıydı")
	}
	if _, ok := s.vrfProofs.byEpoch[10]; !ok {
		t.Fatal("en son epoch (10) korunmalıydı")
	}
}
