// Package p2p — GossipSub ↔ WebSocket hub köprüsü
//
// StartHubBridge çağrıldığında /obscura/messages/1.0.0 topic'ini dinler;
// gelen her RelayEnvelope'u deliver fonksiyonu üzerinden yerel hub'a iletir.
// PublishMessage, mesaj gönderimini GossipSub ağına yayar.
package p2p

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

// MessageTopic — tüm Obscura node'larının mesaj relay için kullandığı topic
const MessageTopic = "/obscura/messages/1.0.0"

// RelayEnvelope — GossipSub üzerinden taşınan mesaj zarfı.
// gossip.RelayMessage ile birebir uyumlu (bağımlılık olmadan).
type RelayEnvelope struct {
	TargetDID string      `json:"target_did"`
	MsgType   string      `json:"msg_type"`
	Payload   interface{} `json:"payload"`
	SentAt    int64       `json:"sent_at"`
	NodeID    string      `json:"node_id"`
}

// StartHubBridge, GossipSub mesaj topic'ini dinleyip gelen zarfları
// deliver fonksiyonu aracılığıyla yerel WebSocket hub'a yönlendirir.
//
// Kullanım (main.go):
//
//	p2p.StartHubBridge(ctx, func(did, typ string, payload interface{}) bool {
//	    return messaging.GlobalHub.SendTo(did, typ, payload)
//	})
func StartHubBridge(ctx context.Context, deliver func(targetDID, msgType string, payload interface{}) bool) error {
	ch := make(chan []byte, 512)
	if err := Subscribe(MessageTopic, ch); err != nil {
		return err
	}
	log.Printf("P2P bridge: GossipSub ↔ hub aktif — topic: %s", MessageTopic)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case raw, ok := <-ch:
				if !ok {
					return
				}
				var env RelayEnvelope
				if err := json.Unmarshal(raw, &env); err != nil {
					log.Printf("P2P bridge: parse hatası: %v", err)
					continue
				}
				// Kendi yayınladığımız mesajı tekrar işleme
				if env.NodeID == SelfID() {
					continue
				}
				deliver(env.TargetDID, env.MsgType, env.Payload)
			}
		}
	}()
	return nil
}

// PublishMessage, hedef DID'e yönelik mesajı GossipSub ağına yayar.
// P2P başlatılmamışsa hata döner — caller HTTP gossip fallback yapabilir.
func PublishMessage(targetDID, msgType string, payload interface{}) error {
	env := RelayEnvelope{
		TargetDID: targetDID,
		MsgType:   msgType,
		Payload:   payload,
		SentAt:    time.Now().UnixMilli(),
		NodeID:    SelfID(),
	}
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return Publish(MessageTopic, data)
}
