# Skill: Yeni Backend Handler Ekle

## Tetikleyici
"Yeni endpoint ekle", "yeni handler yaz", "API ekle" istendiğinde

## Adımlar

1. **CLAUDE.md'deki mevcut endpoint listesini kontrol et**
   - Bu endpoint zaten var mı?
   - Benzer bir şey var mı?

2. **Doğru dosyayı belirle**
   - Temel CRUD → `backend/internal/api/handlers.go`
   - Yeni özellik grubu → `backend/internal/api/extra_handlers.go`
   - Yeni modül → `backend/internal/api/[modül].go`

3. **Handler fonksiyonu yaz**
   ```go
   func HandleXxx(w http.ResponseWriter, r *http.Request) {
       // 1. Auth: userDID, _ := r.Context().Value("userDID").(string)
       // 2. Input parse: json.NewDecoder(r.Body).Decode(&req)
       // 3. Validation
       // 4. DB işlemi
       // 5. Response: writeJSON(w, http.StatusOK, map[string]interface{}{...})
       // veya hata: writeError(w, http.StatusBadRequest, "mesaj")
   }
   ```

4. **Route'u kaydet** — `backend/cmd/node/main.go`
   ```go
   r.Handle("/v1/xxx", middleware.RequireAuth(http.HandlerFunc(api.HandleXxx))).Methods("GET")
   ```

5. **CLAUDE.md'yi güncelle** — endpoint listesine ekle, YAPILDI listesini güncelle

6. **Entegrasyon testi yaz** — `backend/internal/api/integration_test.go`
   ```go
   func TestXxx(t *testing.T) {
       // setup, request, assert
   }
   ```

7. **code-reviewer agent'ı çağır** — yazdığın handler'ı incelet

## Örnek Response Formatları

```go
// Başarılı
writeJSON(w, http.StatusOK, map[string]interface{}{
    "success": true,
    "data": result,
})

// Hata
writeError(w, http.StatusBadRequest, "geçersiz istek")
writeError(w, http.StatusUnauthorized, "yetki yok")
writeError(w, http.StatusNotFound, "bulunamadı")
writeError(w, http.StatusInternalServerError, "sunucu hatası")
```
