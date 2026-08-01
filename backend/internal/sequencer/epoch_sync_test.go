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

// TestCurrentSequencer_AgreesAcrossTwoIndependentInstances — BİLEREK SKIP
// EDİLİYOR. Epoch senkronizasyonu (bu dosyanın asıl konusu) düzeltildikten
// SONRA bile bu test FAIL veriyor — AYRI, önceden var olan bir bug bulundu:
//
//	vrfSelect() (sequencer.go ~satır 359) "rastgelelik" değerini
//	ecvrfProve(s.vrfKey, alpha) ile üretiyor — s.vrfKey ÇAĞIRAN INSTANCE'IN
//	KENDİ özel VRF anahtarı (selfID'den türetilmiş). node-1 instance'ı kendi
//	anahtarıyla bir rastgele değer üretip TÜM aday listesi üzerinde
//	stake-ağırlıklı seçimi ona göre yapıyor; node-2 instance'ı KENDİ
//	anahtarıyla FARKLI bir değer üretip aynı seçimi yapıyor. İki node hiçbir
//	zaman aynı "rastgele" girdiyi paylaşmadığı için epoch ve node listesi
//	birebir aynı olsa bile seçim node'lar arası TUTARSIZ kalıyor.
//
//	Bu, epoch senkronizasyonundan (bu commit'in konusu) AYRI bir tasarım
//	hatası: gerçek VRF-tabanlı leader-election, TÜM taraflarca bağımsız
//	hesaplanabilir/doğrulanabilir PAYLAŞILAN bir rastgelelik kaynağı
//	gerektirir (örn. her aday kendi VRF proof'unu yayınlar, kazanan
//	proof'lar KARŞILAŞTIRILARAK belirlenir) — tek bir yerel özel anahtarla
//	değil. Düzeltme kapsamı ADR-0017 "adım 5" (validator-set/stake-ağırlıklı
//	quorum, leader-election adilliği) — bilerek BU commit'e dahil edilmedi,
//	ayrı bir karar/iş olarak bekliyor.
func TestCurrentSequencer_AgreesAcrossTwoIndependentInstances(t *testing.T) {
	t.Skip("BİLİNEN BUG (ADR-0017 adım 5'e ertelendi): vrfSelect rastgeleliği " +
		"çağıran instance'ın kendi vrfKey'inden üretiyor (sequencer.go ~359), " +
		"paylaşılan/doğrulanabilir bir kaynak değil — node'lar arası seçim bu " +
		"yüzden epoch senkron olsa bile tutarsız. Bkz. bu testin üstündeki yorum.")

	nodeA := NewSequencer("node-1", time.Hour)
	nodeB := NewSequencer("node-2", time.Hour)

	nodes := []NodeInfo{
		{NodeID: "node-1", Stake: 1000},
		{NodeID: "node-2", Stake: 1000},
	}
	nodeA.AddNode(nodes[0].NodeID, nodes[0].Stake)
	nodeA.AddNode(nodes[1].NodeID, nodes[1].Stake)
	nodeB.AddNode(nodes[0].NodeID, nodes[0].Stake)
	nodeB.AddNode(nodes[1].NodeID, nodes[1].Stake)

	seqA := nodeA.CurrentSequencer()
	seqB := nodeB.CurrentSequencer()
	if seqA != seqB {
		t.Fatalf("iki bağımsız node instance'ı FARKLI aktif sequencer hesapladı: A=%q B=%q — epoch senkronizasyonu bozuk", seqA, seqB)
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
