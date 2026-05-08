# Skill: Git Worktree Birleştir

## Tetikleyici
"Branch'leri birleştir", "worktree merge", "feature'ları birleştir" istendiğinde

## Adımlar

1. **Aktif worktree'leri listele**
   ```bash
   git worktree list
   ```

2. **Her branch'teki değişiklikleri özetle**
   ```bash
   git diff main...[branch-adı] --stat
   ```

3. **Bağımlılık sırasını belirle** — bağımsız olanları önce merge et

4. **Her branch için merge**
   ```bash
   git checkout main
   git merge --no-ff [branch-adı] -m "feat: [ne yapıldı]"
   ```

5. **Conflict varsa** — feature branch logic'ini tercih et, kullanıcıya bildir

6. **Test koş**
   ```bash
   cd backend && go test ./...
   cd frontend && npm run build
   ```

7. **Worktree'leri temizle**
   ```bash
   git worktree remove ../obscura-[feature]
   git branch -d [branch-adı]  # merge edildiyse
   ```

8. **Çıktı tablosu**
   | Branch | Değiştirilen Dosyalar | Durum |
   |---|---|---|
   | feature-x | 5 | ✅ Merged |

## Obscura için Worktree Önerilen Yapı

```bash
# Her büyük özellik için ayrı worktree
git worktree add -b feat/mls ../obscura-mls
git worktree add -b feat/libp2p ../obscura-libp2p
git worktree add -b feat/token ../obscura-token
git worktree add -b feat/mini-app ../obscura-miniapp
```

```
obscura/           ← main branch (stabil)
obscura-mls/       ← MLS grup şifreleme
obscura-libp2p/    ← P2P network katmanı
obscura-token/     ← OBS token ekonomisi
obscura-miniapp/   ← Mini app motoru
```
