# Backend Domain (Go)

**Kapsam:** Go 1.22, gorilla/mux, gorilla/websocket, modernc.org/sqlite, JWT.

## Kod

`E:\obscura\backend\`
- `cmd/node/main.go` — entry, route registration
- `internal/api/` — HTTP handlers (auth, users, messages, credit, wallet, staking, governance, airdrop, MLS, shielded, mini app)
- `internal/db/` — SQLite + migrations (54+ migration)
- `internal/{token,staking,governance,airdrop,miniapp,moderation,blockchain,mls,zk}/` — domain logic
- `internal/{auth,gossip,push,media,sms,messaging,credit,models}/` — alt servisler

## Kurallar (sıkı)

- CGO_ENABLED=0 her zaman
- Tüm SQL parametreli (`?`), asla `fmt.Sprintf` ile concat
- Response envelope: `{success, data | error}`
- Multi-statement write → transaction
- `panic()` `main` dışında YOK

## Sub-agent

- [[../../../.claude/agents/backend-engineer|backend-engineer]]
- [[../../../.claude/agents/database-engineer|database-engineer]]
- [[../../../.claude/agents/network-engineer|network-engineer]]

## Skill

- [[../../../.claude/skills/go-backend-patterns/SKILL|go-backend-patterns]]
- [[../../../.claude/skills/sqlite-pure-go-migrations/SKILL|sqlite-pure-go-migrations]]
- [[../../../.claude/skills/minio-aws-sigv4/SKILL|minio-aws-sigv4]]
- [[../../../.claude/skills/push-fcm-apns-webpush/SKILL|push-fcm-apns-webpush]]
- [[../../../.claude/skills/openapi-contract/SKILL|openapi-contract]]

## Test paketleri (hepsi PASS)

- zk, token, staking, governance, airdrop, miniapp, moderation, blockchain, mls
