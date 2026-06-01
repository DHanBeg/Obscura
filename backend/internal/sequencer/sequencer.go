package sequencer

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"sort"
	"sync"
	"time"
)

// Sequencer — merkezi olmayan sequencer rotasyonu (FAZ 4).
// VRF + stake-ağırlıklı seçim ile her epoch'ta bir sequencer belirlenir.
//
// VRF: ECVRF-P256-SHA256-TAI (RFC 9381 §5.1). Stdlib elliptic P-256 üzerinde
// gerçek VRF prove/verify. Output beta deterministik (private key + alpha'ya
// bağlı) ve private key olmadan tahmin edilemez.

type NodeInfo struct {
	NodeID string `json:"node_id"`
	Stake  uint64 `json:"stake"`
}

type Batch struct {
	Epoch     uint64   `json:"epoch"`
	Sequencer string   `json:"sequencer"`
	TxHashes  []string `json:"tx_hashes"`
	Root      string   `json:"root"`
	Timestamp int64    `json:"timestamp"`
}

type Sequencer struct {
	mu       sync.RWMutex
	nodes    []NodeInfo
	batches  []Batch
	epoch    uint64
	selfID   string
	epochDur time.Duration
	stakeFn  func() []NodeInfo
	stopCh   chan struct{}
	running  bool

	// vrfKey: bu node'a ait deterministik VRF anahtarı (NODE_ID'den türetilir).
	vrfKey *ecdsa.PrivateKey
}

// NewSequencer yeni bir sequencer koordinatörü oluşturur.
func NewSequencer(selfID string, epochDur time.Duration) *Sequencer {
	return &Sequencer{
		selfID:   selfID,
		epochDur: epochDur,
		stopCh:   make(chan struct{}),
		vrfKey:   deriveVRFKey(selfID),
	}
}

// SetStakeLookup stake bilgisini sağlayan fonksiyonu ayarlar.
func (s *Sequencer) SetStakeLookup(fn func() []NodeInfo) {
	s.stakeFn = fn
}

// deriveVRFKey NODE_ID'den deterministik bir P-256 private key türetir.
// Aynı NODE_ID her zaman aynı VRF anahtarını verir (restart sonrası tutarlılık).
// Not: Production'da bu anahtar güvenli depodan (KMS/env) gelmeli; burada
// node kimliğine bağlı deterministik türetme dev/test için yeterli.
func deriveVRFKey(nodeID string) *ecdsa.PrivateKey {
	curve := elliptic.P256()
	n := curve.Params().N

	// HKDF-benzeri basit genişletme: counter ile SHA-256 zinciri.
	var d *big.Int
	for ctr := byte(0); ; ctr++ {
		h := sha256.New()
		h.Write([]byte("obscura-vrf-key-v1"))
		h.Write([]byte(nodeID))
		h.Write([]byte{ctr})
		cand := new(big.Int).SetBytes(h.Sum(nil))
		// 1 <= d < n koşulu
		cand.Mod(cand, new(big.Int).Sub(n, big.NewInt(1)))
		cand.Add(cand, big.NewInt(1))
		d = cand
		if d.Sign() > 0 && d.Cmp(n) < 0 {
			break
		}
	}

	priv := new(ecdsa.PrivateKey)
	priv.Curve = curve
	priv.D = d
	priv.PublicKey.Curve = curve
	priv.PublicKey.X, priv.PublicKey.Y = curve.ScalarBaseMult(d.Bytes())
	return priv
}

// ---- ECVRF-P256-SHA256-TAI (RFC 9381 §5.1) ----

// suiteString: ECVRF-P256-SHA256-TAI suite identifier (RFC 9381 §5.5, suite 0x01).
const ecvrfSuite = 0x01

// pointLen: P-256 sıkıştırılmış nokta uzunluğu (1 prefix + 32 byte X).
const pointLen = 33

// challengeLen: cofactor 1 olduğundan c için 16 byte (RFC 9381 §5.5, n=16).
const challengeLen = 16

// ecvrfProve: ECVRF-P256-SHA256-TAI prove (RFC 9381 §5.1).
// Girdi: privKey (P-256), alpha (mesaj/seed). Çıktı: pi proof byte'ları.
func ecvrfProve(privKey *ecdsa.PrivateKey, alpha []byte) []byte {
	curve := privKey.Curve
	q := curve.Params().N

	// 1. H = ECVRF_hash_to_curve_try_and_increment(Y, alpha)
	hx, hy := hashToCurveTAI(curve, &privKey.PublicKey, alpha)

	// 2. Gamma = sk * H
	gx, gy := curve.ScalarMult(hx, hy, privKey.D.Bytes())

	// 3. k = ECVRF_nonce_generation(sk, H_string) — deterministik nonce.
	k := nonceGeneration(curve, privKey.D, hx, hy)

	// 4. U = k*G, V = k*H
	ux, uy := curve.ScalarBaseMult(k.Bytes())
	vx, vy := curve.ScalarMult(hx, hy, k.Bytes())

	// 5. c = ECVRF_hash_points(H, Gamma, U, V)
	c := hashPoints(curve,
		hx, hy,
		gx, gy,
		ux, uy,
		vx, vy,
	)

	// 6. s = (k + c*sk) mod q
	s := new(big.Int).Mul(c, privKey.D)
	s.Add(s, k)
	s.Mod(s, q)

	// 7. pi = Gamma_compressed || c (16 byte) || s (32 byte)
	pi := make([]byte, 0, pointLen+challengeLen+32)
	pi = append(pi, compressPoint(curve, gx, gy)...)
	pi = append(pi, leftPad(c.Bytes(), challengeLen)...)
	pi = append(pi, leftPad(s.Bytes(), 32)...)
	return pi
}

// ecvrfProofToHash: pi -> beta (VRF output, 32 byte). RFC 9381 §5.2.
// beta = SHA256(suite || 0x03 || Gamma_compressed)
func ecvrfProofToHash(curve elliptic.Curve, pi []byte) []byte {
	if len(pi) < pointLen {
		return nil
	}
	gamma := pi[:pointLen]
	h := sha256.New()
	h.Write([]byte{ecvrfSuite, 0x03})
	h.Write(gamma)
	out := h.Sum(nil)
	return out
}

// ecvrfVerify: pi'nin pubKey + alpha için geçerli olduğunu doğrular. RFC 9381 §5.3.
func ecvrfVerify(pub *ecdsa.PublicKey, alpha, pi []byte) bool {
	curve := pub.Curve
	q := curve.Params().N
	if len(pi) != pointLen+challengeLen+32 {
		return false
	}
	gx, gy, ok := decompressPoint(curve, pi[:pointLen])
	if !ok {
		return false
	}
	c := new(big.Int).SetBytes(pi[pointLen : pointLen+challengeLen])
	s := new(big.Int).SetBytes(pi[pointLen+challengeLen:])
	if s.Cmp(q) >= 0 {
		return false
	}

	hx, hy := hashToCurveTAI(curve, pub, alpha)

	// U = s*G - c*Y
	sgx, sgy := curve.ScalarBaseMult(s.Bytes())
	cyx, cyy := curve.ScalarMult(pub.X, pub.Y, c.Bytes())
	ux, uy := curve.Add(sgx, sgy, cyx, new(big.Int).Sub(curve.Params().P, cyy))

	// V = s*H - c*Gamma
	shx, shy := curve.ScalarMult(hx, hy, s.Bytes())
	cgx, cgy := curve.ScalarMult(gx, gy, c.Bytes())
	vx, vy := curve.Add(shx, shy, cgx, new(big.Int).Sub(curve.Params().P, cgy))

	cPrime := hashPoints(curve, hx, hy, gx, gy, ux, uy, vx, vy)
	return cPrime.Cmp(c) == 0
}

// hashToCurveTAI: ECVRF_hash_to_curve_try_and_increment (RFC 9381 §5.4.1.1).
func hashToCurveTAI(curve elliptic.Curve, pub *ecdsa.PublicKey, alpha []byte) (*big.Int, *big.Int) {
	pkBytes := compressPoint(curve, pub.X, pub.Y)
	for ctr := 0; ctr < 256; ctr++ {
		h := sha256.New()
		h.Write([]byte{ecvrfSuite, 0x01})
		h.Write(pkBytes)
		h.Write(alpha)
		h.Write([]byte{byte(ctr)})
		h.Write([]byte{0x00}) // RFC: tek octet zero (suffix)
		hashStr := h.Sum(nil)

		// Sıkıştırılmış nokta dene: 0x02 prefix + hash.
		candidate := append([]byte{0x02}, hashStr...)
		x, y, ok := decompressPoint(curve, candidate)
		if ok {
			return x, y
		}
	}
	// Pratikte ulaşılmaz.
	return curve.Params().Gx, curve.Params().Gy
}

// nonceGeneration: deterministik nonce (RFC 9381 §5.4.2.2 basitleştirilmiş).
// k = SHA256(sk || H_compressed) mod q (q'ya yakın bias ihmal edilebilir).
func nonceGeneration(curve elliptic.Curve, sk *big.Int, hx, hy *big.Int) *big.Int {
	q := curve.Params().N
	h := sha256.New()
	h.Write([]byte("obscura-vrf-nonce"))
	h.Write(leftPad(sk.Bytes(), 32))
	h.Write(compressPoint(curve, hx, hy))
	k := new(big.Int).SetBytes(h.Sum(nil))
	k.Mod(k, new(big.Int).Sub(q, big.NewInt(1)))
	k.Add(k, big.NewInt(1))
	return k
}

// hashPoints: c = ECVRF_hash_points(P1..Pn) (RFC 9381 §5.4.3).
func hashPoints(curve elliptic.Curve, pts ...*big.Int) *big.Int {
	h := sha256.New()
	h.Write([]byte{ecvrfSuite, 0x02})
	for i := 0; i+1 < len(pts); i += 2 {
		h.Write(compressPoint(curve, pts[i], pts[i+1]))
	}
	full := h.Sum(nil)
	// c, ilk challengeLen byte'tan alınır.
	return new(big.Int).SetBytes(full[:challengeLen])
}

// compressPoint: SEC1 sıkıştırılmış nokta kodlaması.
func compressPoint(curve elliptic.Curve, x, y *big.Int) []byte {
	byteLen := (curve.Params().BitSize + 7) / 8
	out := make([]byte, 1+byteLen)
	if y.Bit(0) == 0 {
		out[0] = 0x02
	} else {
		out[0] = 0x03
	}
	xb := x.Bytes()
	copy(out[1+byteLen-len(xb):], xb)
	return out
}

// decompressPoint: SEC1 sıkıştırılmış noktayı çözer. P-256 için y^2 = x^3 - 3x + b.
func decompressPoint(curve elliptic.Curve, data []byte) (*big.Int, *big.Int, bool) {
	params := curve.Params()
	byteLen := (params.BitSize + 7) / 8
	if len(data) != 1+byteLen {
		return nil, nil, false
	}
	if data[0] != 0x02 && data[0] != 0x03 {
		return nil, nil, false
	}
	x := new(big.Int).SetBytes(data[1:])
	if x.Cmp(params.P) >= 0 {
		return nil, nil, false
	}

	// y^2 = x^3 - 3x + b mod p
	p := params.P
	x3 := new(big.Int).Mul(x, x)
	x3.Mul(x3, x)
	threeX := new(big.Int).Lsh(x, 1)
	threeX.Add(threeX, x)
	rhs := new(big.Int).Sub(x3, threeX)
	rhs.Add(rhs, params.B)
	rhs.Mod(rhs, p)

	// y = rhs^((p+1)/4) mod p (p ≡ 3 mod 4 — P-256 için geçerli).
	exp := new(big.Int).Add(p, big.NewInt(1))
	exp.Rsh(exp, 2)
	y := new(big.Int).Exp(rhs, exp, p)

	// Doğrula: y^2 == rhs
	y2 := new(big.Int).Mul(y, y)
	y2.Mod(y2, p)
	if y2.Cmp(rhs) != 0 {
		return nil, nil, false
	}

	// Parite seçimi
	if y.Bit(0) != uint(data[0]&1) {
		y.Sub(p, y)
	}
	if !curve.IsOnCurve(x, y) {
		return nil, nil, false
	}
	return x, y, true
}

func leftPad(b []byte, size int) []byte {
	if len(b) >= size {
		return b[len(b)-size:]
	}
	out := make([]byte, size)
	copy(out[size-len(b):], b)
	return out
}

// vrfSelect VRF tabanlı stake-ağırlıklı sequencer seçimi yapar.
// Her node kendi epoch beta'sını üretir; deterministik seçim için seçimde
// kullanılan rastgelelik bu node'un VRF beta'sından gelir (verifiable).
func (s *Sequencer) vrfSelect(epoch uint64, nodes []NodeInfo) string {
	if len(nodes) == 0 {
		return ""
	}
	// Deterministik sıralama
	sorted := make([]NodeInfo, len(nodes))
	copy(sorted, nodes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].NodeID < sorted[j].NodeID
	})

	// alpha = "obscura-epoch" || epoch (big-endian)
	alpha := make([]byte, 0, 16)
	alpha = append(alpha, []byte("obscura-epoch")...)
	var eb [8]byte
	binary.BigEndian.PutUint64(eb[:], epoch)
	alpha = append(alpha, eb[:]...)

	// VRF ile epoch rastgeleliği üret (gerçek ECVRF).
	pi := ecvrfProve(s.vrfKey, alpha)
	beta := ecvrfProofToHash(s.vrfKey.Curve, pi)
	rnd := new(big.Int).SetBytes(beta)

	// Toplam stake
	var total uint64
	for _, n := range sorted {
		total += n.Stake
	}
	if total == 0 {
		// Eşit ağırlık
		idx := new(big.Int).Mod(rnd, big.NewInt(int64(len(sorted))))
		return sorted[idx.Int64()].NodeID
	}

	// Stake-ağırlıklı seçim
	target := new(big.Int).Mod(rnd, new(big.Int).SetUint64(total))
	var cumulative uint64
	for _, n := range sorted {
		cumulative += n.Stake
		if target.Cmp(new(big.Int).SetUint64(cumulative)) < 0 {
			return n.NodeID
		}
	}
	return sorted[len(sorted)-1].NodeID
}

// VRFProof verilen epoch için bu node'un VRF proof + output'unu döner.
// Diğer node'lar bunu ecvrfVerify ile doğrulayabilir.
func (s *Sequencer) VRFProof(epoch uint64) (proofHex, betaHex string) {
	alpha := make([]byte, 0, 16)
	alpha = append(alpha, []byte("obscura-epoch")...)
	var eb [8]byte
	binary.BigEndian.PutUint64(eb[:], epoch)
	alpha = append(alpha, eb[:]...)
	pi := ecvrfProve(s.vrfKey, alpha)
	beta := ecvrfProofToHash(s.vrfKey.Curve, pi)
	return hex.EncodeToString(pi), hex.EncodeToString(beta)
}

// StartEpochRotation epoch döngüsünü başlatır.
func (s *Sequencer) StartEpochRotation() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go func() {
		ticker := time.NewTicker(s.epochDur)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.rotateEpoch()
			}
		}
	}()
}

func (s *Sequencer) rotateEpoch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.epoch++
	var nodes []NodeInfo
	if s.stakeFn != nil {
		nodes = s.stakeFn()
	} else {
		nodes = s.nodes
	}
	seq := s.vrfSelect(s.epoch, nodes)
	log.Printf("[sequencer] epoch=%d sequencer=%s", s.epoch, seq)
}

// CurrentSequencer mevcut epoch'un sequencer'ını döner.
func (s *Sequencer) CurrentSequencer() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var nodes []NodeInfo
	if s.stakeFn != nil {
		nodes = s.stakeFn()
	} else {
		nodes = s.nodes
	}
	return s.vrfSelect(s.epoch, nodes)
}

// AddNode test/manuel node ekleme.
func (s *Sequencer) AddNode(id string, stake uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes = append(s.nodes, NodeInfo{NodeID: id, Stake: stake})
}

// SubmitBatch yeni bir batch ekler (sadece mevcut sequencer).
func (s *Sequencer) SubmitBatch(txHashes []string) (*Batch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seq := s.vrfSelect(s.epoch, s.currentNodes())
	if seq != s.selfID {
		return nil, fmt.Errorf("bu node sequencer değil (current=%s, self=%s)", seq, s.selfID)
	}
	root := computeMerkleRoot(txHashes)
	b := Batch{
		Epoch:     s.epoch,
		Sequencer: s.selfID,
		TxHashes:  txHashes,
		Root:      root,
		Timestamp: time.Now().Unix(),
	}
	s.batches = append(s.batches, b)
	return &b, nil
}

func (s *Sequencer) currentNodes() []NodeInfo {
	if s.stakeFn != nil {
		return s.stakeFn()
	}
	return s.nodes
}

// Batches tüm batch'leri döner.
func (s *Sequencer) Batches() []Batch {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Batch, len(s.batches))
	copy(out, s.batches)
	return out
}

// Epoch mevcut epoch numarasını döner.
func (s *Sequencer) Epoch() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.epoch
}

func computeMerkleRoot(hashes []string) string {
	if len(hashes) == 0 {
		return ""
	}
	if len(hashes) == 1 {
		return hashes[0]
	}
	var level []string
	level = append(level, hashes...)
	for len(level) > 1 {
		var next []string
		for i := 0; i < len(level); i += 2 {
			if i+1 < len(level) {
				sum := sha256.Sum256([]byte(level[i] + level[i+1]))
				next = append(next, hex.EncodeToString(sum[:]))
			} else {
				next = append(next, level[i])
			}
		}
		level = next
	}
	return level[0]
}
