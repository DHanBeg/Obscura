// Package zk — multi-node proof verification.
//
// Spec Bölüm 19.3: "Her proof en az 2 node tarafından doğrulanır".
// MultiVerify, NODE_PEERS ortam değişkeninden alınan peer listesine paralel
// olarak POST /v1/zk/verify isteği gönderir ve en az minVerifiers kadar
// onay gerektirir. Sonuçlar çelişiyorsa (true/false karışımı) anomaly
// bayrağı set edilir.
package zk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// VerifyResult — tek bir peer'dan dönen doğrulama sonucu.
type VerifyResult struct {
	NodeID  string
	Valid   bool
	Latency time.Duration
	Error   error
}

// multiVerifyRequest — peer'a gönderilen istek gövdesi.
// HandleVerifyZKProof ile aynı format: proof_json + circuit_id + public_inputs.
type multiVerifyRequest struct {
	ProofJSON    string   `json:"proof_json"`
	CircuitID    string   `json:"circuit_id"`
	PublicInputs []string `json:"public_inputs"`
}

// multiVerifyResponse — peer'dan gelen yanıt zarfı.
type multiVerifyResponse struct {
	Success bool `json:"success"`
	// data ve error alanları dinamik — sadece success bayrağı yeterli.
}

// MultiVerify, proof'u NODE_PEERS listesindeki node'lara paralel olarak
// gönderir ve en az minVerifiers kadar onay beklenir.
//
// Dönüş değerleri:
//   - ok:      minVerifiers onay alındıysa true
//   - results: her peer'ın bireysel sonucu (loglama/audit için)
//   - err:     node listesi boşsa veya hiç peer erişilemiyorsa non-nil
//
// ctx iptal edildiğinde uçuştaki tüm HTTP istekleri kesilir.
func MultiVerify(ctx context.Context, proof ProofData, circuitID CircuitID, minVerifiers int) (bool, []VerifyResult, error) {
	peers := parsePeers()
	if len(peers) == 0 {
		return false, nil, fmt.Errorf("multiverify: NODE_PEERS tanımlı değil — en az 1 peer gerekli")
	}

	reqBody, err := json.Marshal(multiVerifyRequest{
		ProofJSON:    proof.ProofJSON,
		CircuitID:    string(circuitID),
		PublicInputs: proof.PublicInputs,
	})
	if err != nil {
		return false, nil, fmt.Errorf("multiverify: istek serileştirilemedi: %w", err)
	}

	type peerResult struct {
		nodeID  string
		valid   bool
		latency time.Duration
		err     error
	}

	ch := make(chan peerResult, len(peers))
	var wg sync.WaitGroup

	for _, peer := range peers {
		wg.Add(1)
		go func(nodeAddr string) {
			defer wg.Done()
			start := time.Now()
			valid, callErr := callPeerVerify(ctx, nodeAddr, reqBody)
			ch <- peerResult{
				nodeID:  nodeAddr,
				valid:   valid,
				latency: time.Since(start),
				err:     callErr,
			}
		}(peer)
	}

	// Tüm goroutine'ler bitince kanalı kapat.
	go func() {
		wg.Wait()
		close(ch)
	}()

	results := make([]VerifyResult, 0, len(peers))
	approvals := 0
	denials := 0

	for pr := range ch {
		vr := VerifyResult{
			NodeID:  pr.nodeID,
			Valid:   pr.valid,
			Latency: pr.latency,
			Error:   pr.err,
		}
		results = append(results, vr)

		if pr.err != nil {
			log.Printf("[MultiVerify] peer=%s hata: %v", pr.nodeID, pr.err)
			continue
		}
		if pr.valid {
			approvals++
		} else {
			denials++
		}
	}

	anomaly := approvals > 0 && denials > 0
	if anomaly {
		log.Printf("[MultiVerify] ANOMALY: circuit=%s approvals=%d denials=%d — peer tutarsızlığı",
			circuitID, approvals, denials)
	}

	ok := approvals >= minVerifiers
	if !ok {
		log.Printf("[MultiVerify] yetersiz onay: circuit=%s approvals=%d min=%d",
			circuitID, approvals, minVerifiers)
	}

	return ok, results, nil
}

// callPeerVerify — tek bir peer'a HTTP isteği gönderir ve proof geçerliyse
// true döner. Hata durumunda false + hata döner.
func callPeerVerify(ctx context.Context, peerAddr string, body []byte) (bool, error) {
	url := fmt.Sprintf("http://%s/v1/zk/verify", peerAddr)

	// Her peer isteği için 10 saniye timeout. Context iptal edilirse de kesilir.
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("istek oluşturulamadı: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// INTERNAL_SECRET — gossip relay ile aynı header'ı kullan.
	if secret := os.Getenv("INTERNAL_SECRET"); secret != "" {
		httpReq.Header.Set("X-Internal-Secret", secret)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return false, fmt.Errorf("peer erişilemedi: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("peer hata kodu: %d", resp.StatusCode)
	}

	var parsed multiVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return false, fmt.Errorf("yanıt ayrıştırılamadı: %w", err)
	}

	return parsed.Success, nil
}

// parsePeers — NODE_PEERS env değişkenini "host:port,host:port" formatında ayrıştırır.
// Boş veya yanlış formatlar atlanır.
func parsePeers() []string {
	raw := os.Getenv("NODE_PEERS")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	peers := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			peers = append(peers, p)
		}
	}
	return peers
}

// ProofData — MultiVerify ve DualVerify için proof taşıyıcı.
// HandleVerifyZKProof ile aynı alanlar.
type ProofData struct {
	ProofJSON    string   // snarkjs Groth16 proof JSON (ham veya kapsayıcı)
	PublicInputs []string // publicSignals dizisi
}
