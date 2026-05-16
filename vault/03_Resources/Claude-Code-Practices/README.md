# Claude Code Practices (kullanıcının ilk verdiği rehber)

Kullanıcı projeye başlarken bu rehberi paylaştı. Bu vault, Claude Code'un kendi self-improvement materyali.

## 10 ana başlık

1. **Git Worktrees** — paralel geliştirme (Obscura için uygulanmadı, tek branch)
2. **Plan Mode** — kod yazmadan önce planlama; ADR'lar bu rolü oynuyor
3. **CLAUDE.md iterasyonu** — proje hafızası ✅ (CLAUDE.md sürekli güncellenir)
4. **Skills** — `.claude/skills/` ✅ kuruldu (15+ Obscura-özel + 379 external)
5. **Hata kodlarıyla bug fix** — kullanıcı hata mesajı yapıştırdığında full stack ver
6. **Zarif çözüm promptu** — kirli iterasyondan sonra temiz reimplementasyon iste
7. **GhostTTY** — terminal yönetimi (kullanıcının terminal tercihi)
8. **Sub Agents** — `.claude/agents/` ✅ kuruldu (30 ajan), audit + paralel iş için kullanılıyor
9. **SQL/veri analizi** — Claude'a sor (Obscura'da kullanılmıyor, SQLite migration runner yeterli)
10. **Açıklayıcı çıktı stili** — output styles (Explanatory / Learning)

## Obscura'da uygulananlar

- ✅ CLAUDE.md (root) — her oturumun ilk okuması
- ✅ Sub-agents — code-reviewer, security-auditor, spec-checker, crypto-engineer, mls-engineer, vb. 30 ajan
- ✅ Skills — Obscura özel (`circom-zk-circuits`, `go-backend-patterns`, `tauri-2x-patterns`, `motion-principles-obscura`, vb.)
- ✅ External skills — anthropics, vercel, expo, pbakaus-impeccable, leonxlnx-taste, vb.
- ✅ ADR sistem (docs/adr/) — Plan Mode'un sürdürülebilir hali
- ✅ Session log (docs/sessions/) — oturumdan oturuma süreklilik
- ✅ Vault (Obsidian) — knowledge graph

## Uygulanmayanlar

- ❌ Git worktrees — tek branch
- ❌ GhostTTY — kullanıcı PowerShell + diğer terminal
- ❌ Slack MCP — kullanılmıyor

## Process dersi (ADR-0008 → 0009'dan)

> "Sub-agent team 6 critical bug buldu. Bundan sonra her major commit öncesi code-reviewer + security-auditor + spec-checker zorunlu."

Bu kural CLAUDE.md'de yazılı.
