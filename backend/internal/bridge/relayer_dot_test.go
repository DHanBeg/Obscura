package bridge

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"testing"
)

// fakeDotTransferer — network'e dokunmayan, çağrı sayısını izleyen sahte
// dotTransferer. Idempotency testleri için: prepare/submit KAÇ KEZ
// çağrıldığını sayarak "aynı event iki kez işlenmedi" iddiasını kanıtlar.
type fakeDotTransferer struct {
	decimalsVal  uint32
	prepareCalls int
	submitCalls  int
	prepareErr   error
	submitErr    error
	submitStatus map[string]interface{}
	rawOut       []byte
	summaryOut   *ExtrinsicSummary
}

func (f *fakeDotTransferer) decimals(ctx context.Context) (uint32, error) {
	return f.decimalsVal, nil
}

func (f *fakeDotTransferer) prepare(ctx context.Context, destSS58 string, planckAmount *big.Int) ([]byte, *ExtrinsicSummary, error) {
	f.prepareCalls++
	if f.prepareErr != nil {
		return nil, nil, f.prepareErr
	}
	raw := f.rawOut
	summary := f.summaryOut
	if summary == nil {
		summary = &ExtrinsicSummary{DestSS58: destSS58, AmountPlanck: planckAmount}
	}
	return raw, summary, nil
}

func (f *fakeDotTransferer) submit(ctx context.Context, raw []byte, onUpdate func(map[string]interface{})) (map[string]interface{}, error) {
	f.submitCalls++
	if f.submitErr != nil {
		return f.submitStatus, f.submitErr
	}
	return map[string]interface{}{"finalized": "0xfakefinalized"}, nil
}

func mustAbiEncodeAndTopic(t *testing.T, amount *big.Int, destChain, recipient string, nonce *big.Int, senderHex string) ethLog {
	t.Helper()
	data := abiEncodeLockedData(amount, destChain, recipient, nonce)
	userTopic := "0x" + strings.Repeat("0", 24) + senderHex
	return ethLog{Topics: []string{lockEventTopic, userTopic}, Data: data}
}

// TestProcessLockEvent_Idempotent_SameEventTwiceOnlyProcessedOnce, aynı
// Locked event'in (aynı tx_hash+log_index) İKİ KEZ beslendiğinde DOT
// tarafının yalnızca BİR KEZ çağrıldığını kanıtlar (relayer restart / ETH
// RPC'nin aynı bloğu tekrar döndürmesi senaryosu — çift-gönderim = para
// kaybı riski).
func TestProcessLockEvent_Idempotent_SameEventTwiceOnlyProcessedOnce(t *testing.T) {
	sqlDB := setupTestDB(t)
	fake := &fakeDotTransferer{decimalsVal: 10}
	r := &Relayer{db: sqlDB, dot: fake, dotAutoSubmit: true}

	amount := big.NewInt(1_000_000_000_000_000_000) // 1 OBS
	recipient := "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY"
	ev := mustAbiEncodeAndTopic(t, amount, "dot", recipient, big.NewInt(1), "abcdef1234567890abcdef1234567890abcdef12")
	ev.TransactionHash = "0xsametx"
	ev.LogIndex = "0x2"

	if err := r.processLockEvent(context.Background(), ev); err != nil {
		t.Fatalf("first processLockEvent: %v", err)
	}
	if err := r.processLockEvent(context.Background(), ev); err != nil {
		t.Fatalf("second processLockEvent: %v", err)
	}

	if fake.prepareCalls != 1 {
		t.Errorf("prepare called %d times, want 1 (idempotency violated — çift işleme riski)", fake.prepareCalls)
	}
	if fake.submitCalls != 1 {
		t.Errorf("submit called %d times, want 1 (idempotency violated — ÇİFT GÖNDERİM riski, para kaybı)", fake.submitCalls)
	}
	t.Logf("KANIT: aynı event 2 kez beslendi, dot.prepare=%d kez dot.submit=%d kez çağrıldı (2. çağrı SKIPPED)", fake.prepareCalls, fake.submitCalls)

	id := syntheticID("0xsametx", "0x2")
	var status string
	if err := sqlDB.QueryRow("SELECT status FROM bridge_transfers WHERE id=?", id).Scan(&status); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "confirmed" {
		t.Errorf("status = %q, want %q", status, "confirmed")
	}
}

// TestProcessLockEvent_Idempotent_AcrossRelayerRestart, bir önceki testten
// FARKLI olarak aynı *Relayer* örneğini yeniden kullanmaz — ikinci
// processLockEvent çağrısı TAMAMEN YENİ bir Relayer struct'ı ve TAMAMEN YENİ
// bir dotTransferer ile yapılır (in-memory state SIFIRLANMIŞ), sadece SQLite
// DB paylaşılır. Bu, gerçek bir "relayer process restart" senaryosunu
// birebir simüle eder: idempotency in-memory bir cache'e değil, DB'deki
// kalıcı duruma dayanıyor mu?
func TestProcessLockEvent_Idempotent_AcrossRelayerRestart(t *testing.T) {
	sqlDB := setupTestDB(t)

	amount := big.NewInt(1_000_000_000_000_000_000)
	recipient := "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY"
	ev := mustAbiEncodeAndTopic(t, amount, "dot", recipient, big.NewInt(1), "abcdef1234567890abcdef1234567890abcdef12")
	ev.TransactionHash = "0xrestarttx"
	ev.LogIndex = "0x0"

	// --- "İlk süreç ömrü": relayer event'i görür, işler, süreç sonra ölür. ---
	fake1 := &fakeDotTransferer{decimalsVal: 10}
	r1 := &Relayer{db: sqlDB, dot: fake1, dotAutoSubmit: true}
	if err := r1.processLockEvent(context.Background(), ev); err != nil {
		t.Fatalf("r1.processLockEvent: %v", err)
	}
	if fake1.prepareCalls != 1 || fake1.submitCalls != 1 {
		t.Fatalf("r1: prepare=%d submit=%d, want 1/1", fake1.prepareCalls, fake1.submitCalls)
	}

	// --- "Restart": yepyeni Relayer + yepyeni dotTransferer, aynı ETH RPC'nin
	// aynı bloğu tekrar döndürdüğü senaryo (ör. lastBlock kayboldu). ---
	fake2 := &fakeDotTransferer{decimalsVal: 10}
	r2 := &Relayer{db: sqlDB, dot: fake2, dotAutoSubmit: true}
	if err := r2.processLockEvent(context.Background(), ev); err != nil {
		t.Fatalf("r2.processLockEvent (restart sonrası): %v", err)
	}

	if fake2.prepareCalls != 0 {
		t.Errorf("restart sonrası yeni relayer instance'ında prepare %d kez çağrıldı, want 0 (DB'deki 'confirmed' durumu görülmeliydi)", fake2.prepareCalls)
	}
	if fake2.submitCalls != 0 {
		t.Errorf("restart sonrası yeni relayer instance'ında submit %d kez çağrıldı, want 0 — ÇİFT GÖNDERİM (para kaybı) riski", fake2.submitCalls)
	}
	t.Logf("KANIT (restart): r1 (eski süreç) prepare=%d submit=%d | r2 (yeni süreç/instance, restart sonrası) prepare=%d submit=%d — idempotency DB'ye dayanıyor, process ömrüne değil",
		fake1.prepareCalls, fake1.submitCalls, fake2.prepareCalls, fake2.submitCalls)
}

// TestProcessLockEvent_DifferentLogIndexSameTx_BothProcessed, tek bir ETH
// tx'inde birden fazla Locked event'i olabileceğini (contract batch/loop
// senaryosu) ve bunların tx_hash AYNI, log_index FARKLI olduğu için
// birbirini YANLIŞLIKLA "duplicate" saymadığını kanıtlar — eski kodun
// (yalnızca tx_hash kontrol eden) bug'ının regresyon testi.
func TestProcessLockEvent_DifferentLogIndexSameTx_BothProcessed(t *testing.T) {
	sqlDB := setupTestDB(t)
	fake := &fakeDotTransferer{decimalsVal: 10}
	r := &Relayer{db: sqlDB, dot: fake, dotAutoSubmit: true}

	amount := big.NewInt(1_000_000_000_000_000_000)
	recipient := "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY"
	sender := "abcdef1234567890abcdef1234567890abcdef12"

	ev0 := mustAbiEncodeAndTopic(t, amount, "dot", recipient, big.NewInt(1), sender)
	ev0.TransactionHash = "0xbatchtx"
	ev0.LogIndex = "0x0"

	ev1 := mustAbiEncodeAndTopic(t, amount, "dot", recipient, big.NewInt(2), sender)
	ev1.TransactionHash = "0xbatchtx" // AYNI tx_hash
	ev1.LogIndex = "0x1"              // FARKLI log_index

	if err := r.processLockEvent(context.Background(), ev0); err != nil {
		t.Fatalf("ev0: %v", err)
	}
	if err := r.processLockEvent(context.Background(), ev1); err != nil {
		t.Fatalf("ev1: %v", err)
	}

	if fake.prepareCalls != 2 {
		t.Errorf("prepare called %d times, want 2 (aynı tx içindeki iki farklı log_index İKİSİ DE işlenmeli)", fake.prepareCalls)
	}

	var count int
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM bridge_transfers WHERE tx_hash='0xbatchtx'").Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 2 {
		t.Errorf("bridge_transfers satır sayısı = %d, want 2", count)
	}
}

// TestProcessLockEvent_PermanentlyFailed_NotRetried, retry hakkı tükenmiş
// (status='failed', retry_count>=max) bir event'in tekrar geldiğinde DOT
// tarafının BİR DAHA çağrılmadığını kanıtlar (sonsuz döngü / kalıcı hata
// sonrası sessiz atlama).
func TestProcessLockEvent_PermanentlyFailed_NotRetried(t *testing.T) {
	sqlDB := setupTestDB(t)
	fake := &fakeDotTransferer{decimalsVal: 10, prepareErr: fmt.Errorf("kalıcı RPC hatası simülasyonu")}
	r := &Relayer{db: sqlDB, dot: fake, dotAutoSubmit: true}

	amount := big.NewInt(1_000_000_000_000_000_000)
	recipient := "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY"
	ev := mustAbiEncodeAndTopic(t, amount, "dot", recipient, big.NewInt(1), "abcdef1234567890abcdef1234567890abcdef12")
	ev.TransactionHash = "0xfailtx"
	ev.LogIndex = "0x0"

	// maxDotTransferRetries kez besle — her seferinde retry_count artar.
	for i := 0; i < maxDotTransferRetries; i++ {
		if err := r.processLockEvent(context.Background(), ev); err != nil {
			t.Fatalf("processLockEvent iterasyon %d: %v", i, err)
		}
	}
	callsAfterExhaustion := fake.prepareCalls
	if callsAfterExhaustion != maxDotTransferRetries {
		t.Fatalf("prepare called %d times, want %d (retry cap'e kadar her denemede çağrılmalı)", callsAfterExhaustion, maxDotTransferRetries)
	}

	// Bir kez daha besle — artık retry hakkı tükenmiş olmalı, prepare
	// ÇAĞRILMAMALI.
	if err := r.processLockEvent(context.Background(), ev); err != nil {
		t.Fatalf("processLockEvent (retry tükendikten sonra): %v", err)
	}
	if fake.prepareCalls != callsAfterExhaustion {
		t.Errorf("prepare called %d times after exhaustion, want %d (sonsuz döngü — retry cap'i ihlal edildi)", fake.prepareCalls, callsAfterExhaustion)
	}

	id := syntheticID("0xfailtx", "0x0")
	var status string
	var retryCount int
	if err := sqlDB.QueryRow("SELECT status, retry_count FROM bridge_transfers WHERE id=?", id).Scan(&status, &retryCount); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "failed" {
		t.Errorf("status = %q, want %q", status, "failed")
	}
	if retryCount < maxDotTransferRetries {
		t.Errorf("retry_count = %d, want >= %d", retryCount, maxDotTransferRetries)
	}
}

// fixtureDotTransferer, gerçek Paseo V14 metadata fixture'ı + gerçek sr25519
// imza kullanarak, AMA canlı RPC'ye hiç dokunmadan (sabit spec/tx version,
// sabit genesis, sabit nonce) DOT extrinsic'i hazırlar. Bu, relayer akışının
// gerçek SCALE encoding + gerçek kriptografiyle uçtan uca (ETH event ->
// extrinsic bytes) doğru çalıştığını, hiçbir ağa bağımlı olmadan kanıtlamak
// için var.
type fixtureDotTransferer struct {
	md          *Metadata
	keypair     *Sr25519Keypair
	genesis     [32]byte
	nonce       uint64
	submitCalls int // author_submitExtrinsic çağrılıp çağrılmadığının KANITI
}

func newFixtureDotTransferer(t *testing.T) *fixtureDotTransferer {
	t.Helper()
	hexStr := loadPaseoMetadataFixture(t)
	md, err := DecodeMetadataV14(hexStr)
	if err != nil {
		t.Fatalf("DecodeMetadataV14: %v", err)
	}
	kp, err := Sr25519DevKeypair("Alice")
	if err != nil {
		t.Fatalf("Sr25519DevKeypair: %v", err)
	}
	var genesis [32]byte
	for i := range genesis {
		genesis[i] = byte(i) // sabit/deterministik, gerçek zincirle eşleşmesi gerekmiyor (offline test)
	}
	return &fixtureDotTransferer{md: md, keypair: kp, genesis: genesis, nonce: 7}
}

func (f *fixtureDotTransferer) decimals(ctx context.Context) (uint32, error) { return 10, nil }

func (f *fixtureDotTransferer) prepare(ctx context.Context, destSS58 string, planckAmount *big.Int) ([]byte, *ExtrinsicSummary, error) {
	destPub, prefix, err := DecodeSS58(destSS58)
	if err != nil {
		return nil, nil, &permanentError{err}
	}
	if prefix != dotSS58Prefix {
		return nil, nil, &permanentError{fmt.Errorf("beklenmeyen ss58 prefix %d", prefix)}
	}
	signerSS58, err := EncodeSS58(f.keypair.Public, dotSS58Prefix)
	if err != nil {
		return nil, nil, err
	}
	params := SignedExtraParams{
		SpecVersion: 2003001, TxVersion: 26,
		GenesisHash: f.genesis, EraCheckpoint: f.genesis, Era: EraImmortal(),
		Nonce: f.nonce, Tip: big.NewInt(0),
	}
	raw, err := BuildSignedExtrinsic(f.md, f.keypair, destPub, planckAmount, params)
	if err != nil {
		return nil, nil, err
	}
	summary, err := DecodeExtrinsicForReview(f.md, raw, dotSS58Prefix)
	if err != nil {
		return nil, nil, err
	}
	if summary.SignerSS58 != signerSS58 || summary.DestSS58 != destSS58 || summary.AmountPlanck.Cmp(planckAmount) != 0 {
		return nil, nil, &permanentError{fmt.Errorf("decode kontrolü uyuşmuyor")}
	}
	return raw, summary, nil
}

func (f *fixtureDotTransferer) submit(ctx context.Context, raw []byte, onUpdate func(map[string]interface{})) (map[string]interface{}, error) {
	f.submitCalls++ // bu sayaç 0 KALMALI — testin asıl iddiası bu
	return nil, fmt.Errorf("bu testte submit ÇAĞRILMAMALI — sadece hazırlama/decode test ediliyor")
}

// TestProcessLockEvent_PreparesRealDotExtrinsic_Offline, mock bir Locked
// event'inden (ETH tarafı, 18 decimal, 0.1 OBS — bu oturumda gerçekten
// gönderilen 0.1 PAS ile aynı miktar) başlayıp, GERÇEK SCALE encoding +
// GERÇEK sr25519 imza ile bir DOT extrinsic'inin hazırlandığını ve
// bağımsızca decode edilip doğrulanabildiğini kanıtlar — hiç GÖNDERİLMEDEN
// (dotAutoSubmit=false, fixtureDotTransferer.submit çağrılırsa test FAIL
// olur).
func TestProcessLockEvent_PreparesRealDotExtrinsic_Offline(t *testing.T) {
	sqlDB := setupTestDB(t)
	fixture := newFixtureDotTransferer(t)
	r := &Relayer{db: sqlDB, dot: fixture, dotAutoSubmit: false}

	weiAmount, ok := new(big.Int).SetString("100000000000000000", 10) // 0.1 OBS, 18 decimals
	if !ok {
		t.Fatal("bad test fixture")
	}
	destSS58 := "5GZVKNQhXGfWWmAYy5psxjm3E2g9J9aYTbGK86qWYG9LnxSC"
	ev := mustAbiEncodeAndTopic(t, weiAmount, "dot", destSS58, big.NewInt(99), "abcdef1234567890abcdef1234567890abcdef12")
	ev.TransactionHash = "0xoffline1"
	ev.LogIndex = "0x0"

	if err := r.processLockEvent(context.Background(), ev); err != nil {
		t.Fatalf("processLockEvent: %v", err)
	}

	id := syntheticID("0xoffline1", "0x0")
	var status, extrinsicHex, planckStr string
	if err := sqlDB.QueryRow(
		"SELECT status, dot_extrinsic_hex, dot_amount_planck FROM bridge_transfers WHERE id=?", id,
	).Scan(&status, &extrinsicHex, &planckStr); err != nil {
		t.Fatalf("query: %v", err)
	}

	if status != "ready_to_submit" {
		t.Errorf("status = %q, want %q (hazırlandı ama auto-submit kapalı olduğu için gönderilmemeli)", status, "ready_to_submit")
	}
	if planckStr != "1000000000" { // 0.1 OBS (18 dec) -> 0.1 PAS (10 dec) = 1e9 planck
		t.Errorf("dot_amount_planck = %q, want %q (18->10 decimal çevrimi yanlış)", planckStr, "1000000000")
	}
	if extrinsicHex == "" || !strings.HasPrefix(extrinsicHex, "0x") {
		t.Fatalf("dot_extrinsic_hex boş/biçimsiz: %q", extrinsicHex)
	}

	// Bağımsız decode: DB'ye yazılan hex'in GERÇEKTEN doğru signer/dest/amount
	// taşıdığını, relayer'ın iç mantığından ayrı olarak yeniden doğrula.
	rawBytes, err := hexDecodeNoPrefix(extrinsicHex)
	if err != nil {
		t.Fatalf("hex decode: %v", err)
	}
	summary, err := DecodeExtrinsicForReview(fixture.md, rawBytes, dotSS58Prefix)
	if err != nil {
		t.Fatalf("DecodeExtrinsicForReview: %v", err)
	}
	wantSigner, _ := EncodeSS58(fixture.keypair.Public, dotSS58Prefix)
	if summary.SignerSS58 != wantSigner {
		t.Errorf("signer = %q, want %q", summary.SignerSS58, wantSigner)
	}
	if summary.DestSS58 != destSS58 {
		t.Errorf("dest = %q, want %q", summary.DestSS58, destSS58)
	}
	if summary.AmountPlanck.String() != "1000000000" {
		t.Errorf("amount planck = %s, want 1000000000", summary.AmountPlanck.String())
	}

	// KANIT: author_submitExtrinsic (fixtureDotTransferer.submit) HİÇ
	// çağrılmadı — extrinsic yalnızca hazırlandı/imzalandı/decode edildi,
	// gönderilmedi.
	if fixture.submitCalls != 0 {
		t.Fatalf("submit %d kez çağrıldı, want 0 — bu parçada author_submitExtrinsic ÇAĞRILMAMALIYDI", fixture.submitCalls)
	}
	t.Logf("KANIT: dotAutoSubmit=false, fixture.submitCalls=%d — author_submitExtrinsic hiç çağrılmadı, extrinsic sadece hazırlandı+decode edildi", fixture.submitCalls)
}

func hexDecodeNoPrefix(s string) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(s, "0x"))
}
