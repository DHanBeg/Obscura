// Package pqcrypto — Post-quantum cryptography preparation (FAZ 3, spec 12.3)
//
// CRYSTALS-Kyber-768 anahtar kapsülleme mekanizması (KEM).
// cloudflare/circl kütüphanesi kullanılır.
// FAZ 4'te X3DH anahtar değişimini bu KEM ile hibrit modda sarmalayacak.
package pqcrypto

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/cloudflare/circl/kem/kyber/kyber768"
)

// KeyPair — Kyber-768 anahtar çifti (hex kodlu)
type KeyPair struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"` // güvenli depolama gerektirir
}

// EncapsulateResult — anahtar kapsülleme sonucu
type EncapsulateResult struct {
	Ciphertext  string `json:"ciphertext"`  // alıcıya gönderilir
	SharedSecret string `json:"shared_secret"` // yerel kullanım — sakla
}

// GenerateKeyPair — yeni Kyber-768 anahtar çifti üret
func GenerateKeyPair() (*KeyPair, error) {
	scheme := kyber768.Scheme()
	pub, priv, err := scheme.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("kyber keygen: %w", err)
	}

	pubBytes, err := pub.MarshalBinary()
	if err != nil {
		return nil, err
	}
	privBytes, err := priv.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return &KeyPair{
		PublicKey:  hex.EncodeToString(pubBytes),
		PrivateKey: hex.EncodeToString(privBytes),
	}, nil
}

// Encapsulate — alıcının public key'i ile shared secret kapsülle
// Gönderen taraf kullanır: ciphertext alıcıya, sharedSecret gönderenin elinde kalır
func Encapsulate(recipientPublicKeyHex string) (*EncapsulateResult, error) {
	pubBytes, err := hex.DecodeString(recipientPublicKeyHex)
	if err != nil {
		return nil, fmt.Errorf("geçersiz public key hex: %w", err)
	}

	scheme := kyber768.Scheme()
	pub, err := scheme.UnmarshalBinaryPublicKey(pubBytes)
	if err != nil {
		return nil, fmt.Errorf("public key çözümle: %w", err)
	}

	ct, ss, err := scheme.Encapsulate(pub)
	if err != nil {
		return nil, fmt.Errorf("kapsülleme: %w", err)
	}

	return &EncapsulateResult{
		Ciphertext:   hex.EncodeToString(ct),
		SharedSecret: hex.EncodeToString(ss),
	}, nil
}

// Decapsulate — alıcının private key'i ve ciphertext ile shared secret çıkar
func Decapsulate(privateKeyHex, ciphertextHex string) (string, error) {
	privBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("geçersiz private key hex: %w", err)
	}
	ctBytes, err := hex.DecodeString(ciphertextHex)
	if err != nil {
		return "", fmt.Errorf("geçersiz ciphertext hex: %w", err)
	}

	scheme := kyber768.Scheme()
	priv, err := scheme.UnmarshalBinaryPrivateKey(privBytes)
	if err != nil {
		return "", fmt.Errorf("private key çözümle: %w", err)
	}

	ss, err := scheme.Decapsulate(priv, ctBytes)
	if err != nil {
		return "", fmt.Errorf("çözümleme: %w", err)
	}

	return hex.EncodeToString(ss), nil
}

// HybridKey — X25519 (klasik) + Kyber-768 (post-quantum) hibrit anahtar
// İki shared secret XOR ile birleştirilir → 32 byte combined key
type HybridKey struct {
	Classical string `json:"x25519_pub"` // hex
	PQ        string `json:"kyber_pub"`  // hex
}

// RandomBytes — kriptografik rastgele bayt üretici
func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}
