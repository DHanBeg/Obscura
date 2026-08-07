// Package bridge — ETH→DOT event relayer (spec Bölüm 12.3)
//
// Her 30s'de Ethereum'u eth_getLogs ile polling yapar (CGO=0, raw HTTP JSON-RPC).
// Yeni Locked event'i bulunca bridge_transfers tablosuna pending kaydı yazar,
// ardından DOT tarafında karşılık extrinsic'ini hazırlar (Bridge madde 4,
// PARÇA 3 — bkz. dotTransferer, processDotSide).
//
// GÜVENLİK: DOT_RELAYER_AUTO_SUBMIT açıkça "true" olmadıkça extrinsic
// SADECE hazırlanır/imzalanır/decode ile doğrulanır — author_submitExtrinsic
// hiç ÇAĞRILMAZ (status='ready_to_submit' olarak DB'de bekler). Bu parçanın
// (PARÇA 3) kapsamı budur; gerçek otomatik gönderim PARÇA 4'te kullanıcı
// onayıyla açılır.
package bridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Locked(address,uint256,string,string,uint256) event topic (keccak256 precomputed)
// keccak256("Locked(address,uint256,string,string,uint256)")
const lockEventTopic = "0xa5d128e0362bc8d53e55961fe2b57e89e4bdac2b0dcfdcd915db0c3c0044d2ac"

// maxDotTransferRetries, bir event için DOT tarafı hazırlama/gönderme
// denemesinin üst sınırıdır — aşılınca status='failed' KALICI olur, retry
// sweep bir daha denemez (sonsuz döngü YOK, bkz. paket dokümantasyonu).
const maxDotTransferRetries = 5

// dotSS58Prefix, Paseo/generic Substrate SS58 ağ prefix'idir (bkz.
// extrinsic.go/dot.go — bu paket boyunca sabit 42 kullanılıyor).
const dotSS58Prefix = 42

// permanentError, retry ETMENİN anlamsız olduğu hataları sarmalar (geçersiz
// SS58 adresi, sıfıra yuvarlanan miktar, decode uyuşmazlığı) — retryOrFail
// yerine doğrudan failPermanently'e yönlendirilir.
type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

func isPermanentErr(err error) bool {
	var pe *permanentError
	return errors.As(err, &pe)
}

// dotTransferer, DOT tarafında extrinsic hazırlama/gönderme davranışını
// soyutlar. Üretimde gerçek RPC'ye bağlanan liveDotTransferer kullanılır;
// testler network'e dokunmadan bir fixture implementasyonuyla (gerçek SCALE
// encoding + gerçek sr25519 imza, sabit chain verisiyle) çalışabilir.
type dotTransferer interface {
	decimals(ctx context.Context) (uint32, error)
	prepare(ctx context.Context, destSS58 string, planckAmount *big.Int) (raw []byte, summary *ExtrinsicSummary, err error)
	submit(ctx context.Context, raw []byte, onUpdate func(status map[string]interface{})) (finalStatus map[string]interface{}, err error)
}

// liveDotTransferer, gerçek Paseo RPC'sine karşı çalışan dotTransferer
// implementasyonudur (bkz. dot_submit.go/extrinsic.go/watch.go).
type liveDotTransferer struct {
	client         *Client
	keypair        *Sr25519Keypair
	cachedDecimals uint32 // 0 = henüz çekilmedi
}

func (l *liveDotTransferer) decimals(ctx context.Context) (uint32, error) {
	if l.cachedDecimals != 0 {
		return l.cachedDecimals, nil
	}
	d, err := l.client.FetchTokenDecimals(ctx)
	if err != nil {
		return 0, err
	}
	l.cachedDecimals = d
	return d, nil
}

func (l *liveDotTransferer) prepare(ctx context.Context, destSS58 string, planckAmount *big.Int) ([]byte, *ExtrinsicSummary, error) {
	destPub, destPrefix, err := DecodeSS58(destSS58)
	if err != nil {
		return nil, nil, &permanentError{fmt.Errorf("recipient SS58 decode: %w", err)}
	}
	if destPrefix != dotSS58Prefix {
		return nil, nil, &permanentError{fmt.Errorf("recipient ss58 prefix %d, beklenen %d değil", destPrefix, dotSS58Prefix)}
	}

	md, err := l.client.FetchMetadata(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("metadata: %w", err)
	}
	rv, err := l.client.FetchRuntimeVersion(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime version: %w", err)
	}
	genesis, err := l.client.FetchGenesisHash(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("genesis hash: %w", err)
	}
	signerSS58, err := EncodeSS58(l.keypair.Public, dotSS58Prefix)
	if err != nil {
		return nil, nil, fmt.Errorf("relayer adresi encode: %w", err)
	}
	nonce, err := l.client.FetchAccountNonce(ctx, signerSS58)
	if err != nil {
		return nil, nil, fmt.Errorf("nonce: %w", err)
	}

	params := SignedExtraParams{
		SpecVersion: rv.SpecVersion, TxVersion: rv.TransactionVersion,
		GenesisHash: genesis, EraCheckpoint: genesis, Era: EraImmortal(),
		Nonce: nonce, Tip: big.NewInt(0),
	}
	raw, err := BuildSignedExtrinsic(md, l.keypair, destPub, planckAmount, params)
	if err != nil {
		return nil, nil, fmt.Errorf("extrinsic kurma/imzalama: %w", err)
	}

	summary, err := DecodeExtrinsicForReview(md, raw, dotSS58Prefix)
	if err != nil {
		return nil, nil, fmt.Errorf("decode-for-review: %w", err)
	}
	if summary.SignerSS58 != signerSS58 || summary.DestSS58 != destSS58 || summary.AmountPlanck.Cmp(planckAmount) != 0 {
		return nil, nil, &permanentError{fmt.Errorf(
			"son decode kontrolü uyuşmuyor (signer/dest/amount beklenenle eşleşmiyor) — DUR")}
	}
	return raw, summary, nil
}

func (l *liveDotTransferer) submit(ctx context.Context, raw []byte, onUpdate func(map[string]interface{})) (map[string]interface{}, error) {
	return l.client.SubmitAndWatchExtrinsic(ctx, fmt.Sprintf("0x%x", raw), onUpdate)
}

// parseRelayerSeed, DOT_RELAYER_SEED ortam değişkenini bir sr25519 anahtar
// çiftine çevirir. İki biçim desteklenir:
//   - "//Name" — Substrate dev derivation kısayolu (SADECE test/kanıt amaçlı,
//     bkz. dot.go Sr25519DevKeypair dokümantasyonu — "//Alice" gibi).
//   - 32-baytlık ham hex seed (64 hex karakter, "0x" opsiyonel) — gerçek
//     relayer hesabı için.
//
// Mnemonic (BIP39) parse desteklenmiyor (YAGNI — ihtiyaç olduğunda eklenir).
func parseRelayerSeed(seed string) (*Sr25519Keypair, error) {
	seed = strings.TrimSpace(seed)
	if seed == "" {
		return nil, fmt.Errorf("DOT_RELAYER_SEED boş")
	}
	if strings.HasPrefix(seed, "//") {
		return Sr25519DevKeypair(strings.TrimPrefix(seed, "//"))
	}
	h := strings.TrimPrefix(seed, "0x")
	if len(h) == 64 {
		b, err := hex.DecodeString(h)
		if err != nil {
			return nil, fmt.Errorf("hex seed decode: %w", err)
		}
		var arr [32]byte
		copy(arr[:], b)
		return Sr25519KeypairFromSeed(arr)
	}
	return nil, fmt.Errorf("desteklenmeyen DOT_RELAYER_SEED biçimi (ne \"//Name\" dev kısayolu ne 32-bayt hex seed)")
}

func parseBoolEnv(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v == "1" || v == "true" || v == "yes"
}

// Relayer polls Ethereum for OBSBridge Locked events and (if DOT tarafı
// yapılandırılmışsa) karşılık gelen DOT extrinsic'ini hazırlar/gönderir.
type Relayer struct {
	EthRPCURL    string
	DotRPCURL    string
	ContractAddr string
	PollInterval time.Duration
	db           *sql.DB
	httpClient   *http.Client
	lastBlock    uint64

	dot           dotTransferer // nil = DOT tarafı yapılandırılmamış, event'ler 'pending' kalır
	dotAutoSubmit bool
}

// NewRelayer reads config from env and returns a Relayer.
func NewRelayer(database *sql.DB) *Relayer {
	r := &Relayer{
		EthRPCURL:     os.Getenv("ETH_RPC_URL"),
		DotRPCURL:     os.Getenv("DOT_RPC_URL"),
		ContractAddr:  os.Getenv("ETH_BRIDGE_CONTRACT"),
		PollInterval:  30 * time.Second,
		db:            database,
		httpClient:    &http.Client{Timeout: 15 * time.Second},
		dotAutoSubmit: parseBoolEnv("DOT_RELAYER_AUTO_SUBMIT"),
	}

	seed := os.Getenv("DOT_RELAYER_SEED")
	switch {
	case seed == "" || r.DotRPCURL == "":
		log.Println("[relayer] DOT_RELAYER_SEED veya DOT_RPC_URL yok — DOT tarafı devre dışı, event'ler 'pending' bekleyecek")
	default:
		kp, err := parseRelayerSeed(seed)
		if err != nil {
			log.Printf("[relayer] DOT_RELAYER_SEED çözülemedi, DOT tarafı devre dışı: %v", err)
			break
		}
		client := New(Config{PolkadotRPC: r.DotRPCURL, RPCTimeout: 15 * time.Second, RPCMaxRetries: 3})
		r.dot = &liveDotTransferer{client: client, keypair: kp}
		relayerSS58, _ := EncodeSS58(kp.Public, dotSS58Prefix)
		log.Printf("[relayer] DOT tarafı hazır, relayer adresi=%s auto-submit=%v", relayerSS58, r.dotAutoSubmit)
	}

	return r
}

// StartRelayer spawns background polling goroutine.
func StartRelayer(ctx context.Context, database *sql.DB) {
	r := NewRelayer(database)
	go r.run(ctx)
}

func (r *Relayer) run(ctx context.Context) {
	if r.EthRPCURL == "" || r.ContractAddr == "" {
		log.Println("[relayer] bridge not configured — set ETH_RPC_URL + ETH_BRIDGE_CONTRACT")
		return
	}
	log.Printf("[relayer] started — contract=%s interval=%s", r.ContractAddr, r.PollInterval)

	if head, err := r.ethBlockNumber(ctx); err == nil {
		r.lastBlock = head
		log.Printf("[relayer] initialised at block %d", head)
	}

	ticker := time.NewTicker(r.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("[relayer] stopped")
			return
		case <-ticker.C:
			if err := r.poll(ctx); err != nil {
				log.Printf("[relayer] poll error: %v", err)
			}
			// Yeni ETH bloklarından bağımsız: DOT tarafı önceden çökük olduğu
			// için 'pending' kalmış (ya da retry hakkı kalan) event'leri
			// tekrar dener — bkz. paket dokümantasyonu ("ETH event'i geldi
			// ama DOT tarafı çökük → event kaybolmasın").
			r.retrySweepPendingDotTransfers(ctx)
		}
	}
}

func (r *Relayer) poll(ctx context.Context) error {
	head, err := r.ethBlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("eth_blockNumber: %w", err)
	}
	if head <= r.lastBlock {
		return nil
	}

	from := fmt.Sprintf("0x%x", r.lastBlock+1)
	to := fmt.Sprintf("0x%x", head)
	events, err := r.ethGetLogs(ctx, from, to)
	if err != nil {
		return fmt.Errorf("eth_getLogs: %w", err)
	}
	log.Printf("[relayer] blocks %d→%d: %d event(s)", r.lastBlock+1, head, len(events))
	for _, ev := range events {
		if err := r.processLockEvent(ctx, ev); err != nil {
			log.Printf("[relayer] processLockEvent tx=%s: %v", ev.TransactionHash, err)
		}
	}
	r.lastBlock = head
	return nil
}

type ethLog struct {
	Topics          []string `json:"topics"`
	Data            string   `json:"data"`
	TransactionHash string   `json:"transactionHash"`
	LogIndex        string   `json:"logIndex"`
}

func (r *Relayer) ethGetLogs(ctx context.Context, from, to string) ([]ethLog, error) {
	body := map[string]interface{}{
		"jsonrpc": "2.0", "method": "eth_getLogs", "id": 1,
		"params": []interface{}{map[string]interface{}{
			"fromBlock": from, "toBlock": to,
			"address": r.ContractAddr,
			"topics":  []string{lockEventTopic},
		}},
	}
	raw, err := r.rpcCall(ctx, r.EthRPCURL, body)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(raw["result"])
	if err != nil {
		return nil, fmt.Errorf("marshal logs: %w", err)
	}
	var logs []ethLog
	if err := json.Unmarshal(b, &logs); err != nil {
		return nil, fmt.Errorf("unmarshal logs: %w", err)
	}
	return logs, nil
}

func (r *Relayer) ethBlockNumber(ctx context.Context) (uint64, error) {
	raw, err := r.rpcCall(ctx, r.EthRPCURL, map[string]interface{}{
		"jsonrpc": "2.0", "method": "eth_blockNumber", "params": []interface{}{}, "id": 1,
	})
	if err != nil {
		return 0, err
	}
	s := strings.TrimPrefix(raw["result"].(string), "0x")
	return strconv.ParseUint(s, 16, 64)
}

// processLockEvent, ETH tarafında yakalanan bir Locked event'ini işler:
// idempotency kontrolü (tx_hash+log_index kimliği), DB'ye kayıt, ve DOT
// tarafında karşılık extrinsic'in hazırlanması/gönderilmesi.
//
// IDEMPOTENCY: aynı event (aynı id = sha256(tx_hash+":"+log_index)) iki kez
// gelirse — relayer restart, ETH RPC'nin aynı bloğu tekrar döndürmesi, vs. —
// status zaten 'confirmed' ya da retry hakkı tükenmiş 'failed' ise HİÇBİR
// ŞEY yapılmadan döner (bkz. dedup sorgusu aşağıda, id ÜZERİNDEN — eski kod
// yalnızca tx_hash kontrol ediyordu, bu YANLIŞTI: aynı tx içindeki farklı
// log_index'li iki event'i de tek event sayardı).
func (r *Relayer) processLockEvent(ctx context.Context, ev ethLog) error {
	sender := topicToAddress(safeIdx(ev.Topics, 1))
	amountWei, _, recipient, _, err := decodeLockedEventData(ev.Data)
	if err != nil {
		return fmt.Errorf("decode Locked event data: %w", err)
	}
	txHash := ev.TransactionHash
	logIndex := ev.LogIndex
	id := syntheticID(txHash, logIndex)

	var status string
	var retryCount int
	err = r.db.QueryRowContext(ctx,
		"SELECT status, retry_count FROM bridge_transfers WHERE id = ?", id,
	).Scan(&status, &retryCount)

	switch {
	case err == nil:
		if status == "confirmed" {
			return nil // idempotent: zaten tamamlanmış
		}
		if status == "failed" && retryCount >= maxDotTransferRetries {
			return nil // idempotent: retry hakkı tükenmiş, kalıcı hata
		}
		// pending/ready_to_submit/retry hakkı kalan failed -> devam et
	case errors.Is(err, sql.ErrNoRows):
		now := time.Now().UTC().Format(time.RFC3339)
		if _, insErr := r.db.ExecContext(ctx, `
			INSERT INTO bridge_transfers
				(id,chain_from,chain_to,sender_address,recipient_address,amount,tx_hash,log_index,status,created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			id, "eth", "dot", sender, recipient, amountWei, txHash, logIndex, "pending", now,
		); insErr != nil {
			return fmt.Errorf("insert: %w", insErr)
		}
		log.Printf("[relayer] lock id=%s %s→%s %s wei kayıt altına alındı", id, sender, recipient, amountWei)
		retryCount = 0
	default:
		return fmt.Errorf("idempotency kontrolü: %w", err)
	}

	return r.processDotSide(ctx, id, recipient, amountWei, retryCount)
}

// processDotSide, bir bridge_transfers satırı için DOT tarafı extrinsic'ini
// hazırlar (ve DOT_RELAYER_AUTO_SUBMIT açıksa gönderir). ETH tarafından
// gelen canlı event'lerden VE retrySweepPendingDotTransfers'tan (DB'den
// kurtarma) ortak çağrılır — decimal çevrimi + hazırlama + hata yönetimi tek
// yerde.
func (r *Relayer) processDotSide(ctx context.Context, id, recipientSS58, amountWeiStr string, retryCount int) error {
	if r.dot == nil {
		// DOT tarafı yapılandırılmamış/çökük — bu event'in HATASI değil,
		// altyapı eksik. 'pending' kalır, retry sweep tekrar dener (retry
		// sayacı ARTMAZ — sonsuz döngü kısıtı yalnızca gerçek gönderme
		// denemeleri için geçerli, altyapı bekleme durumu için değil).
		_, err := r.db.ExecContext(ctx,
			"UPDATE bridge_transfers SET status='pending', error_message=? WHERE id=?",
			"DOT tarafı yapılandırılmamış (DOT_RELAYER_SEED/DOT_RPC_URL eksik)", id)
		return err
	}

	weiAmount, ok := new(big.Int).SetString(amountWeiStr, 10)
	if !ok {
		return r.failPermanently(ctx, id, fmt.Errorf("amount parse edilemedi: %q", amountWeiStr))
	}

	decimals, err := r.dot.decimals(ctx)
	if err != nil {
		return r.retryOrFail(ctx, id, retryCount, fmt.Errorf("dot token decimals: %w", err))
	}

	planckAmount, dust, err := EthWeiToDotPlanck(weiAmount, decimals)
	if err != nil {
		return r.failPermanently(ctx, id, fmt.Errorf("decimal çevrimi: %w", err))
	}
	if dust.Sign() != 0 {
		log.Printf("[relayer] id=%s: %s wei dust kayboluyor (eth %d dec -> dot %d dec)", id, dust.String(), EthTokenDecimals, decimals)
	}

	raw, summary, err := r.dot.prepare(ctx, recipientSS58, planckAmount)
	if err != nil {
		if isPermanentErr(err) {
			return r.failPermanently(ctx, id, err)
		}
		return r.retryOrFail(ctx, id, retryCount, err)
	}
	rawHex := fmt.Sprintf("0x%x", raw)

	if _, err := r.db.ExecContext(ctx,
		"UPDATE bridge_transfers SET dot_extrinsic_hex=?, dot_amount_planck=? WHERE id=?",
		rawHex, summary.AmountPlanck.String(), id,
	); err != nil {
		return fmt.Errorf("extrinsic hex kaydı: %w", err)
	}

	if !r.dotAutoSubmit {
		log.Printf("[relayer] id=%s DOT extrinsic HAZIR (auto-submit kapalı, GÖNDERİLMEDİ) dest=%s amount=%s planck",
			id, summary.DestSS58, summary.AmountPlanck.String())
		_, err := r.db.ExecContext(ctx,
			"UPDATE bridge_transfers SET status='ready_to_submit', error_message='' WHERE id=?", id)
		return err
	}

	finalStatus, err := r.dot.submit(ctx, raw, func(s map[string]interface{}) {
		log.Printf("[relayer] id=%s dot durum: %+v", id, s)
	})
	if err != nil {
		if _, sawInBlock := finalStatus["inBlock"]; sawInBlock {
			// KRİTİK: extrinsic muhtemelen zincire GİTTİ (inBlock görüldü)
			// ama finalize beklerken bağlantı/hata koptu — bu, PARÇA 2'deki
			// websocket bug'ının sistematik hali. Nonce muhtemelen tükendi;
			// TEKRAR GÖNDERMEK ÇİFT-HARCAMA riski taşır. Kalıcı 'failed' +
			// MANUEL inceleme gerektir (dot_extrinsic_hex ve inBlock hash'i
			// error_message'da saklı — operatör chain_getBlock ile teyit
			// edip elle 'confirmed'a çekebilir).
			msg := fmt.Sprintf("submit sonrası inBlock görüldü (%v) ama finalize beklerken hata — muhtemelen ZATEN GÖNDERİLDİ, retry YAPILMAYACAK, manuel kontrol gerekli: %v", finalStatus["inBlock"], err)
			log.Printf("[relayer] id=%s %s", id, msg)
			_, dbErr := r.db.ExecContext(ctx,
				"UPDATE bridge_transfers SET status='failed', retry_count=?, error_message=? WHERE id=?",
				maxDotTransferRetries, msg, id)
			return dbErr
		}
		// Broadcast öncesi/sırasında hata — extrinsic zincire hiç girmedi,
		// güvenle retry edilebilir.
		return r.retryOrFail(ctx, id, retryCount, fmt.Errorf("submit/watch: %w", err))
	}

	txHashHex := fmt.Sprintf("0x%x", ExtrinsicHash(raw))
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = r.db.ExecContext(ctx,
		"UPDATE bridge_transfers SET status='confirmed', dot_tx_hash=?, confirmed_at=?, error_message='' WHERE id=?",
		txHashHex, now, id)
	return err
}

// retryOrFail, retry edilebilir bir DOT tarafı hatasını kaydeder. Deneme
// sayısı maxDotTransferRetries'e ulaşınca status='failed' KALICI olur —
// retry sweep bir daha bu id'yi denemez (sonsuz döngü yok).
func (r *Relayer) retryOrFail(ctx context.Context, id string, retryCount int, cause error) error {
	newCount := retryCount + 1
	status := "pending"
	if newCount >= maxDotTransferRetries {
		status = "failed"
	}
	log.Printf("[relayer] id=%s DOT hazırlama/gönderme hatası (deneme %d/%d): %v", id, newCount, maxDotTransferRetries, cause)
	_, dbErr := r.db.ExecContext(ctx,
		"UPDATE bridge_transfers SET status=?, retry_count=?, error_message=? WHERE id=?",
		status, newCount, cause.Error(), id)
	return dbErr
}

// failPermanently, retry etmenin anlamsız olduğu bir hatayı kalıcı olarak
// işaretler (geçersiz adres, sıfıra yuvarlanan miktar, decode uyuşmazlığı) —
// retry_count doğrudan cap'e sabitlenir ki sweep bir daha denemesin.
func (r *Relayer) failPermanently(ctx context.Context, id string, cause error) error {
	log.Printf("[relayer] id=%s KALICI hata (retry edilmeyecek): %v", id, cause)
	_, dbErr := r.db.ExecContext(ctx,
		"UPDATE bridge_transfers SET status='failed', retry_count=?, error_message=? WHERE id=?",
		maxDotTransferRetries, cause.Error(), id)
	return dbErr
}

// retrySweepPendingDotTransfers, ETH tarafından yeni event beklemeden
// 'pending' durumdaki (DOT tarafı önceden çökük olduğu için bekleyen ya da
// retry hakkı kalan) satırları DB'den okuyup tekrar dener.
func (r *Relayer) retrySweepPendingDotTransfers(ctx context.Context) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, recipient_address, amount, retry_count FROM bridge_transfers WHERE chain_to='dot' AND status='pending'")
	if err != nil {
		log.Printf("[relayer] retry sweep sorgu hatası: %v", err)
		return
	}
	type pendingItem struct {
		id, recipient, amount string
		retryCount            int
	}
	var items []pendingItem
	for rows.Next() {
		var it pendingItem
		if scanErr := rows.Scan(&it.id, &it.recipient, &it.amount, &it.retryCount); scanErr != nil {
			log.Printf("[relayer] retry sweep scan hatası: %v", scanErr)
			continue
		}
		items = append(items, it)
	}
	rows.Close()

	for _, it := range items {
		if err := r.processDotSide(ctx, it.id, it.recipient, it.amount, it.retryCount); err != nil {
			log.Printf("[relayer] retry sweep id=%s: %v", it.id, err)
		}
	}
}

func (r *Relayer) rpcCall(ctx context.Context, url string, body interface{}) (map[string]interface{}, error) {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, err
	}
	if errVal, ok := result["error"]; ok && errVal != nil {
		return nil, fmt.Errorf("rpc error: %v", errVal)
	}
	return result, nil
}

func topicToAddress(topic string) string {
	topic = strings.TrimPrefix(topic, "0x")
	if len(topic) < 64 {
		return "0x" + topic
	}
	return "0x" + topic[24:]
}

// decodeLockedEventData ABI-decodes the non-indexed fields of
// Locked(address indexed user, uint256 amount, string destChain,
//
//	string recipient, uint256 nonce).
//
// Head = 4 words (amount, destChain-offset, recipient-offset, nonce);
// dynamic strings live in the tail at their respective offsets.
func decodeLockedEventData(data string) (amount, destChain, recipient, nonce string, err error) {
	raw := strings.TrimPrefix(data, "0x")
	b, hexErr := hex.DecodeString(raw)
	if hexErr != nil {
		return "", "", "", "", fmt.Errorf("hex decode: %w", hexErr)
	}
	const wordSize = 32
	const headWords = 4
	if len(b) < headWords*wordSize {
		return "", "", "", "", fmt.Errorf("event data too short: %d bytes", len(b))
	}

	word := func(i int) []byte { return b[i*wordSize : (i+1)*wordSize] }

	amount = new(big.Int).SetBytes(word(0)).String()
	nonce = new(big.Int).SetBytes(word(3)).String()

	destChainOff := new(big.Int).SetBytes(word(1)).Int64()
	recipientOff := new(big.Int).SetBytes(word(2)).Int64()

	destChain, err = decodeABIDynamicString(b, destChainOff)
	if err != nil {
		return "", "", "", "", fmt.Errorf("destChain: %w", err)
	}
	recipient, err = decodeABIDynamicString(b, recipientOff)
	if err != nil {
		return "", "", "", "", fmt.Errorf("recipient: %w", err)
	}
	return amount, destChain, recipient, nonce, nil
}

// decodeABIDynamicString reads a dynamic ABI string at byte offset `offset`
// within `b`: a 32-byte length word followed by the (unpadded) UTF-8 bytes.
func decodeABIDynamicString(b []byte, offset int64) (string, error) {
	const wordSize = int64(32)
	if offset < 0 || offset+wordSize > int64(len(b)) {
		return "", fmt.Errorf("offset %d out of range (data len %d)", offset, len(b))
	}
	length := new(big.Int).SetBytes(b[offset : offset+wordSize]).Int64()
	start := offset + wordSize
	end := start + length
	if length < 0 || end > int64(len(b)) {
		return "", fmt.Errorf("string length %d out of range at offset %d", length, offset)
	}
	return string(b[start:end]), nil
}

func syntheticID(txHash, logIndex string) string {
	h := sha256.Sum256([]byte(txHash + ":" + logIndex))
	return hex.EncodeToString(h[:16])
}

func safeIdx(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}
