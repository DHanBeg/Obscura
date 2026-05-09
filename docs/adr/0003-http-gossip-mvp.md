# ADR 0003: HTTP gossip for FAZ 1, libp2p later

Date: 2026-05-02
Status: Accepted
Decider: project lead
Spec ref: Bölüm 2.1 MODUL A (libp2p host), Bölüm 3.2 (mesaj yönlendirme)

## Context

Spec mandates libp2p with DHT, GossipSub, QUIC, NAT traversal for inter-node communication. This is a 2-3 month integration effort even for an experienced libp2p engineer.

For FAZ 1 MVP (5 whitelisted nodes), we don't need the full P2P stack — we know all peer addresses ahead of time and trust them.

## Options considered

### Option A: Full libp2p-go from day one
- Pros: Spec-compliant, zero migration debt
- Cons: 6-8 weeks engineering, blocks all other FAZ 1 work
- Effort: XL
- Risk: High (libp2p has steep learning curve)

### Option B: Simple HTTP relay (chosen)
- Each node runs `POST /v1/internal/relay` endpoint
- Sender posts to all peers in `NODE_PEERS` env
- Auth via shared `INTERNAL_SECRET` HMAC header
- Pros: 200 lines of Go, ships in 1 day
- Cons: Not horizontally scalable past ~20 nodes, no NAT traversal, no DHT
- Effort: S
- Risk: Low

### Option C: NATS / Redis pub-sub
- Pros: Battle-tested message bus
- Cons: External dependency violates "zero SaaS / minimal external services" principle
- Effort: M

## Decision

**Option B** for FAZ 1 (5 whitelisted nodes). Migrate to libp2p in FAZ 3 (Federasyon) when permissionless nodes need DHT discovery.

## Rationale

- 5 whitelisted nodes is FAZ 1 spec target. We know addresses, we trust them.
- libp2p complexity is unjustified for trusted small mesh.
- HTTP/JSON is debuggable with curl, libp2p needs special tools.
- Migration path is clean: keep HTTP relay alive, add libp2p alongside, switch readers, retire HTTP.

## Consequences

- **Positive**: Ships fast, easy to debug, no libp2p learning curve blocking FAZ 1
- **Negative**: Won't scale past ~20 nodes; need migration before FAZ 3
- **Tech debt**: ~4-6 weeks libp2p migration in FAZ 3 (tracked in p2p-engineer agent's plan)

## Migration plan (toward FAZ 3)

1. Add libp2p-go host alongside HTTP relay (dual-write)
2. Add GossipSub topic `/obscura/messages/v1`
3. Subscribe + verify against HTTP relay (audit phase)
4. Switch reader to libp2p, keep HTTP for debugging
5. Add Kademlia DHT for peer discovery
6. Open node admission (permissionless) — requires libp2p
7. Retire HTTP relay endpoint

## References

- libp2p-go: https://github.com/libp2p/go-libp2p
- GossipSub spec: https://github.com/libp2p/specs/tree/master/pubsub/gossipsub
- Spec: docs/spec/obscura_spec_v3.txt Bölüm 2-3
