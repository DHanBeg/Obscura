package sequencer

// epoch_sync_test.go — 2026-08-01'de gerçek 2-node Railway testinde bulunan
// bug'ı doğrular: epoch önceden her Sequencer instance'ında BAĞIMSIZ artan
// bir sayaçtı (s.epoch++), bu yüzden farklı anda boot olan iki node'un
// epoch'ları hiç örtüşmüyor, dolayısıyla VRF seçimi (aynı node listesiyle
// bile) node'lar arası TUTARSIZ çıkıyordu. Fix: epoch artık duvar saatinden
// türetiliyor (currentEpoch) — bu testler iki BAĞIMSIZ Sequencer instance'ının
// (farklı selfID = farklı "node") koordinasyonsuz olarak AYNI epoch'u ve
// AYNI aktif sequencer'ı hesapladığını doğruluyor.

import (
	"testing"
	"time"
)

func TestCurrentEpoch_IsPureFunctionOfTime(t *testing.T) {
	s := NewSequencer("node-1", time.Hour)
	e1 := s.currentEpoch()
	e2 := s.currentEpoch()
	if e1 != e2 {
		t.Fatalf("aynı anda ardışık çağrılar aynı epoch'u vermeli, got %d ve %d", e1, e2)
	}
}

// TestCurrentEpoch_IndependentOfConstructionTime — iki Sequencer, biri
// diğerinden bir süre SONRA construct edilse bile (farklı "boot zamanı"
// simülasyonu), aynı anda sorgulandıklarında AYNI epoch'u vermeli. Eskiden
// (s.epoch bir sayaçken) bu İMKANSIZDI — her instance kendi sıfırından
// başlardı.
func TestCurrentEpoch_IndependentOfConstructionTime(t *testing.T) {
	nodeA := NewSequencer("node-1", time.Hour)
	time.Sleep(20 * time.Millisecond) // "node-2" bir miktar geç boot oluyor
	nodeB := NewSequencer("node-2", time.Hour)

	if nodeA.currentEpoch() != nodeB.currentEpoch() {
		t.Fatalf("farklı anda construct edilen node'lar aynı epoch'u vermeli (epochDur=1h olduğu için 20ms fark önemsiz), got A=%d B=%d",
			nodeA.currentEpoch(), nodeB.currentEpoch())
	}
}

// TestCurrentSequencer_AgreesAcrossTwoIndependentInstances — eskiden BİLEREK
// SKIP ediliyordu: vrfSelect() (kaldırıldı) her instance'ın KENDİ özel VRF
// anahtarından bir rastgele değer üretip TÜM aday listesini tek başına
// seçiyordu, bu yüzden iki node ASLA aynı "rastgele" girdiyi paylaşmıyordu.
// ADR-0017 adım 5 ile düzeltildi: kazanan artık paylaşılan/doğrulanmış VRF
// proof'larından (vrfSelectHRW + hrwWinner) hesaplanıyor. Bu testte
// vrf_broadcast_test.go'daki fakeBus deseniyle gerçek proof exchange'i
// simüle ediyoruz — SADECE epoch/node listesi aynı olduğu için değil,
// proof'lar gerçekten karşılıklı toplanıp doğrulandığı için iki bağımsız
// instance'ın AYNI kazananı bulduğunu kanıtlar.
func TestCurrentSequencer_AgreesAcrossTwoIndependentInstances(t *testing.T) {
	nodeA := NewSequencer("node-1", time.Hour)
	nodeB := NewSequencer("node-2", time.Hour)

	pubA := nodeA.VRFPublicKeyHex()
	pubB := nodeB.VRFPublicKeyHex()
	nodes := []NodeInfo{
		{NodeID: "node-1", Stake: 1000, VRFPubkey: pubA},
		{NodeID: "node-2", Stake: 1000, VRFPubkey: pubB},
	}
	nodeA.SetStakeLookup(func() []NodeInfo { return nodes })
	nodeB.SetStakeLookup(func() []NodeInfo { return nodes })

	bus := &fakeBus{}
	if err := nodeA.SetVRFTransport(bus.publish, bus.subscribe); err != nil {
		t.Fatalf("nodeA SetVRFTransport: %v", err)
	}
	if err := nodeB.SetVRFTransport(bus.publish, bus.subscribe); err != nil {
		t.Fatalf("nodeB SetVRFTransport: %v", err)
	}

	epoch := nodeA.currentEpoch()
	if err := nodeA.PublishOwnProof(epoch); err != nil {
		t.Fatalf("nodeA PublishOwnProof: %v", err)
	}
	if err := nodeB.PublishOwnProof(epoch); err != nil {
		t.Fatalf("nodeB PublishOwnProof: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(nodeA.ProofsForEpoch(epoch)) == 2 && len(nodeB.ProofsForEpoch(epoch)) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("2sn içinde her iki node da 2 proof toplayamadı: nodeA=%d nodeB=%d",
				len(nodeA.ProofsForEpoch(epoch)), len(nodeB.ProofsForEpoch(epoch)))
		}
		time.Sleep(10 * time.Millisecond)
	}

	seqA := nodeA.CurrentSequencer()
	seqB := nodeB.CurrentSequencer()
	if seqA != seqB {
		t.Fatalf("iki bağımsız node instance'ı FARKLI aktif sequencer hesapladı: A=%q B=%q", seqA, seqB)
	}
	if seqA != "node-1" && seqA != "node-2" {
		t.Fatalf("aktif sequencer beklenen iki node'dan biri olmalıydı, got=%q", seqA)
	}
}

// TestCurrentEpoch_MatchesManualFormula — epoch'un gerçekten Unix-zaman /
// epochDur formülüyle eşleştiğini doğrular (uygulama detayına bağımlı ama
// regresyonu doğrudan yakalar).
func TestCurrentEpoch_MatchesManualFormula(t *testing.T) {
	s := NewSequencer("node-1", 10*time.Second)
	want := uint64(time.Now().Unix()) / 10
	got := s.currentEpoch()
	if got != want {
		t.Fatalf("epoch formülü uyuşmuyor: got=%d want=%d", got, want)
	}
}
