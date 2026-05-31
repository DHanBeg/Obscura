// Package consensus — transport adapter for the BFT engine.
//
// Engine.NewEngine bir publishFn/subscribeFn çifti bekler. Üretimde bunlar
// libp2p GossipSub'a (p2p.Publish / p2p.Subscribe) bağlanır. Ancak consensus
// paketinin p2p'ye doğrudan bağımlılığı IMPORT CYCLE ve sıkı kuplaj yaratır;
// bu yüzden transport main tarafından dışarıdan enjekte edilir (dependency
// inversion — aynı desen sequencer.SetStakeLookup ve p2p.SetNodeProofVerifier'da
// kullanılıyor).
//
// LocalTransport, GossipSub bağlanana kadar kullanılan in-process bir taşıyıcıdır.
// Tek-node geliştirme/test ortamında konsensüs döngüsünün uçtan uca çalışmasını
// sağlar (propose → prevote → precommit → commit). Çok-node üretim için main,
// p2p.Publish/Subscribe köprüsünü NewEngine'e geçirmelidir.
//
// TODO(FAZ3-BFT): main.go'da P2P aktifken LocalTransport yerine GossipSub
// publish/subscribe köprüsü enjekte et (p2p paketinin Publish/Subscribe API'si
// stabilleştiğinde).
package consensus

import "sync"

// LocalTransport — in-process publish/subscribe taşıyıcısı.
// Aynı süreç içindeki abonelere mesaj iletir. Thread-safe.
type LocalTransport struct {
	mu   sync.RWMutex
	subs map[string][]chan<- []byte
}

// NewLocalTransport — yeni in-process taşıyıcı.
func NewLocalTransport() *LocalTransport {
	return &LocalTransport{subs: make(map[string][]chan<- []byte)}
}

// Publish — topic'e mesaj yayınla (Engine.publishFn imzasıyla uyumlu).
// Abonelere bloklamadan (non-blocking) gönderir; dolu kanal mesajı düşürür.
func (t *LocalTransport) Publish(topic string, data []byte) error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, ch := range t.subs[topic] {
		select {
		case ch <- data:
		default:
			// abone kanalı dolu — düşür (BFT round timeout ile toparlanır)
		}
	}
	return nil
}

// Subscribe — topic'e abone ol (Engine.subscribeFn imzasıyla uyumlu).
func (t *LocalTransport) Subscribe(topic string, ch chan<- []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.subs[topic] = append(t.subs[topic], ch)
	return nil
}
