// Package p2p — Ed25519 identity key persist/load
package p2p

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// LoadOrCreatePrivateKey verilen dosya yolundan Ed25519 private key yükler.
// Dosya yoksa yeni anahtar üretilip kaydedilir (PeerID tutarlılığı için).
// path boşsa her seferinde yeni anahtar üretilir (dev/test modu).
func LoadOrCreatePrivateKey(path string) (crypto.PrivKey, error) {
	if path == "" {
		// Dev modu: ephemeral key, her başlatmada farklı PeerID
		priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("ephemeral key üretimi: %w", err)
		}
		log.Println("P2P: geçici identity key kullanılıyor (dev modu — P2P_KEY_PATH tanımlanmamış)")
		return priv, nil
	}

	// Dizin oluştur (DATA_DIR altında çalışır)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("key dizini oluşturulamadı: %w", err)
	}

	// Mevcut key dosyasını yükle
	if data, err := os.ReadFile(path); err == nil {
		raw, err := hex.DecodeString(string(data))
		if err != nil {
			return nil, fmt.Errorf("key hex decode: %w", err)
		}
		priv, err := crypto.UnmarshalPrivateKey(raw)
		if err != nil {
			return nil, fmt.Errorf("key unmarshal: %w", err)
		}
		log.Printf("P2P: mevcut identity key yüklendi: %s", path)
		return priv, nil
	}

	// Yeni key üret ve kaydet
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("identity key üretimi: %w", err)
	}

	raw, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("key marshal: %w", err)
	}

	encoded := hex.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(encoded), 0600); err != nil {
		// Yazma hatası kritik değil — ephemeral olarak devam et
		log.Printf("P2P uyarı: identity key kaydedilemedi (%s): %v", path, err)
	} else {
		log.Printf("P2P: yeni identity key oluşturuldu ve kaydedildi: %s", path)
	}

	return priv, nil
}
