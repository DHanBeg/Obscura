package api

import (
	"encoding/json"
	"net/http"

	"obscura.network/core/internal/identity"
)

// HandleGenerateMnemonic — POST /v1/identity/mnemonic/generate
// Yeni 12 kelimeli BIP39 mnemonic üret.
// NOT: Bu endpoint istemci tarafında çalışmalıdır idealde.
// Sunucu tarafı üretim: test/onboarding için kabul edilebilir.
func HandleGenerateMnemonic(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	phrase, err := identity.GenerateMnemonic()
	if err != nil {
		respond(w, 500, nil, "Mnemonic üretilemedi")
		return
	}
	respond(w, 200, map[string]string{
		"mnemonic": phrase,
		"words":    "12",
		"warning":  "Mnemonici güvenli saklayın. Sunucuya tekrar göndermeyiniz.",
	}, "")
}

// HandleValidateMnemonic — POST /v1/identity/mnemonic/validate
func HandleValidateMnemonic(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mnemonic string `json:"mnemonic"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	valid := identity.ValidateMnemonic(req.Mnemonic)
	respond(w, 200, map[string]bool{"valid": valid}, "")
}

// HandleDeriveIdentity — POST /v1/identity/derive
// Mnemonic'ten DID türet (passphrase isteğe bağlı).
// GÜVENLIK: production'da bu istemci tarafında yapılmalıdır.
func HandleDeriveIdentity(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	var req struct {
		Mnemonic   string `json:"mnemonic"`
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	if !identity.ValidateMnemonic(req.Mnemonic) {
		respond(w, 400, nil, "Geçersiz mnemonic")
		return
	}
	did, err := identity.DIDFromMnemonic(req.Mnemonic, req.Passphrase)
	if err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	respond(w, 200, map[string]string{"did": did}, "")
}

// HandleShamirSplit — POST /v1/identity/shamir/split
// Mnemonic'i k-of-n Shamir paylarına böl.
func HandleShamirSplit(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	var req struct {
		Mnemonic string `json:"mnemonic"`
		K        int    `json:"k"` // minimum pay sayısı
		N        int    `json:"n"` // toplam pay sayısı
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	if req.K == 0 {
		req.K = 3
	}
	if req.N == 0 {
		req.N = 5
	}
	shares, err := identity.SplitMnemonic(req.Mnemonic, req.K, req.N)
	if err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	respond(w, 200, map[string]interface{}{
		"shares":  shares,
		"k":       req.K,
		"n":       req.N,
		"warning": "Her payı farklı güvenilir kişiye verin. k payıyla hesabı kurtarabilirsiniz.",
	}, "")
}

// HandleShamirCombine — POST /v1/identity/shamir/combine
// k adet Shamir payından mnemonic'i kurtar.
func HandleShamirCombine(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Shares []identity.ShamirShare `json:"shares"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	if len(req.Shares) < 2 {
		respond(w, 400, nil, "En az 2 pay gerekli")
		return
	}
	phrase, err := identity.CombineShares(req.Shares)
	if err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	respond(w, 200, map[string]string{
		"mnemonic": phrase,
		"warning":  "Mnemonici hemen güvenli depolamaya aktarın.",
	}, "")
}
