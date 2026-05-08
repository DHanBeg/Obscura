# Skill: Yeni Özelliğe Başla

## Tetikleyici
Her yeni özellik/modül geliştirmesine başlamadan önce bu skill'i uygula

## Adımlar (Plan Mode — kod yazmadan önce)

1. **CLAUDE.md'yi oku** — tam olarak, özetine güvenme

2. **Bu özellik nerede?**
   - YAPILDI listesinde → zaten var, tekrar yazma
   - YAPILMADI listesinde → hangi fazda, sırası geldi mi?
   - Listede yok → spec'te var mı kontrol et

3. **spec-checker agent'ı çağır**
   - "Bu özelliğin spec'teki tanımını bul ve mevcut kodla karşılaştır"

4. **Etkilenecek dosyaları belirle**
   ```
   Backend değişikliği: handlers.go veya yeni dosya + main.go (route) + integration_test.go
   Frontend değişikliği: lib/api.ts + ilgili page/component
   Mobile değişikliği: mobile/lib/api.ts + ilgili screen
   ZK değişikliği: circuits/ + frontend/lib/zk.ts + backend verify handler
   ```

5. **Planı kullanıcıya sun, onay bekle**
   ```
   Yapacaklarım:
   1. [dosya] — [ne değişecek]
   2. [dosya] — [ne değişecek]
   
   Yapmayacaklarım (kapsam dışı):
   - [ilgili ama şimdi değil]
   
   Onaylıyor musun?
   ```

6. **Onay geldikten sonra** kodu yaz

7. **Bitince:**
   - code-reviewer agent'ı çağır
   - CLAUDE.md'yi güncelle (YAPILMADI → YAPILDI)
   - git commit at
