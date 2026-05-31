// CRYSTALS-Dilithium — Post-quantum digital signatures (FAZ 4, spec 12.4)
//
// Kyber-768 (FAZ 3) ile birlikte tam hibrit post-quantum şifreleme sağlar:
//   - Kyber-768: anahtar değişimi (KEM)
//   - Dilithium3: dijital imza
//
// NIST PQC standardı: ML-DSA (Module-Lattice Digital Signature Algorithm)
package pqcrypto

import (
	"encoding/hex"
	"fmt"

	"github.com/cloudflare/circl/sign/dilithium/mode3"
)

// DilithiumKeyPair — Dilithium3 imza anahtar çifti
type DilithiumKeyPair struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"` // güvenli depolama zorunlu
}

// DilithiumSignature — Dilithium3 imzası
type DilithiumSignature struct {
	Message   string `json:"message"`   // hex
	Signature string `json:"signature"` // hex
	PublicKey string `json:"public_key"` // doğrulama için
}

// DilithiumGenerateKeyPair — yeni Dilithium3 anahtar çifti
func DilithiumGenerateKeyPair() (*DilithiumKeyPair, error) {
	pub, priv, err := mode3.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("dilithium keygen: %w", err)
	}

	pubBytes, err := pub.MarshalBinary()
	if err != nil {
		return nil, err
	}
	privBytes, err := priv.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return &DilithiumKeyPair{
		PublicKey:  hex.EncodeToString(pubBytes),
		PrivateKey: hex.EncodeToString(privBytes),
	}, nil
}

// DilithiumSign — mesajı Dilithium3 ile imzala
func DilithiumSign(privateKeyHex string, messageHex string) (*DilithiumSignature, error) {
	privBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("geçersiz private key: %w", err)
	}
	msgBytes, err := hex.DecodeString(messageHex)
	if err != nil {
		return nil, fmt.Errorf("geçersiz message hex: %w", err)
	}

	var priv mode3.PrivateKey
	if err := priv.UnmarshalBinary(privBytes); err != nil {
		return nil, fmt.Errorf("private key ayrıştırma: %w", err)
	}

	sig := make([]byte, mode3.SignatureSize)
	mode3.SignTo(&priv, msgBytes, sig)

	// PublicKey'i private key'den türet
	pub := priv.Public().(*mode3.PublicKey)
	pubBytes, _ := pub.MarshalBinary()

	return &DilithiumSignature{
		Message:   messageHex,
		Signature: hex.EncodeToString(sig),
		PublicKey: hex.EncodeToString(pubBytes),
	}, nil
}

// DilithiumVerify — Dilithium3 imzasını doğrula
func DilithiumVerify(publicKeyHex, messageHex, signatureHex string) (bool, error) {
	pubBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return false, fmt.Errorf("geçersiz public key: %w", err)
	}
	msgBytes, err := hex.DecodeString(messageHex)
	if err != nil {
		return false, fmt.Errorf("geçersiz message hex: %w", err)
	}
	sigBytes, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false, fmt.Errorf("geçersiz signature hex: %w", err)
	}

	var pub mode3.PublicKey
	if err := pub.UnmarshalBinary(pubBytes); err != nil {
		return false, fmt.Errorf("public key ayrıştırma: %w", err)
	}

	return mode3.Verify(&pub, msgBytes, sigBytes), nil
}
