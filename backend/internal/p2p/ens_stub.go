// Package p2p — ENS (Ethereum Name Service) çözümleme stub'ı
//
// Gerçek ENS çözümlemesi için bir Ethereum RPC endpoint'i gereklidir.
// OBSCURA_ENS_RPC_URL env değişkeni tanımlandığında ilerleyen fazlarda
// go-ethereum ethclient ile on-chain resolver çağrılacak.
// Şu anda: env boşsa önceden bilinen IP adresi döner.
package p2p

import (
	"fmt"
	"log"
	"os"
)

// ensRecord — bilinen ENS isimlerine karşılık gelen hardcoded fallback
// bootstrap adresleri. ENS çözümleme başarısız olduğunda veya RPC
// yapılandırılmamış olduğunda bu değerler kullanılır.
var ensRecord = map[string]string{
	// ENS RPC ile çözümlenmeden önce kullanılacak bootstrap adresi.
	// Prodüksiyonda bu adres ENS kaydından dinamik olarak okunur.
	"obscura.eth": "/ip4/95.216.100.1/tcp/9000",
}

// resolveENS verilen ENS ismini çözümleyip libp2p multiaddr stringi döner.
//
// Çözümleme öncelik sırası:
//  1. OBSCURA_ENS_RPC_URL tanımlıysa Ethereum RPC'ye bağlanmayı dene
//     (TODO: FAZ 3 — go-ethereum/ethclient ile gerçek on-chain çözümleme)
//  2. Bilinen isimler için hardcoded fallback adres döndür
//  3. İkisi de yoksa hata döner
//
// Gerçek ENS entegrasyonu için gerekli adımlar (TODO, FAZ 3+):
//
//	client, err := ethclient.Dial(os.Getenv("OBSCURA_ENS_RPC_URL"))
//	resolver, err := ens.NewResolver(client, name)
//	addr, err := resolver.Address()  // EIP-137 resolver
//	// addr → hex Ethereum adresi → DNS TXT kaydından multiaddr oku
func resolveENS(name string) (string, error) {
	rpcURL := os.Getenv("OBSCURA_ENS_RPC_URL")
	if rpcURL != "" {
		// ENS provider yapılandırılmış — gerçek çözümleme henüz implement edilmedi.
		// FAZ 3'te go-ethereum/ethclient + ENS resolver buraya eklenecek.
		// Şimdilik uyarı ver ve fallback kullan.
		log.Printf("P2P ENS: OBSCURA_ENS_RPC_URL=%s tanımlandı ancak gerçek ENS çözümleme henüz desteklenmiyor — fallback kullanılıyor", rpcURL)
	}

	if addr, ok := ensRecord[name]; ok {
		log.Printf("P2P ENS: '%s' için hardcoded fallback adresi kullanılıyor: %s", name, addr)
		return addr, nil
	}

	return "", fmt.Errorf("ENS çözümleme başarısız: '%s' için kayıt bulunamadı (ENS provider gerekli)", name)
}
