# Skill: Yeni ZK Devresi Ekle

## Tetikleyici
"ZK devresi ekle", "circom circuit", "yeni kanıt" istendiğinde

## Adımlar

1. **Spec'te bu devrenin tanımı var mı kontrol et** (CLAUDE.md YAPILMADI listesi)

2. **Circuit dosyasını yaz** — `circuits/[isim].circom`
   ```circom
   pragma circom 2.1.6;
   include "../node_modules/circomlib/circuits/poseidon.circom";
   include "../node_modules/circomlib/circuits/comparators.circom";

   template CircuitAdı() {
       // Private inputs
       signal input xxx;
       // Public inputs
       signal input yyy;
       // Output
       signal output commitment;

       // Constraints
       // ...
   }

   component main {public [yyy]} = CircuitAdı();
   ```

3. **Browser client'ı yaz** — `frontend/lib/zk.ts`'e ekle
   ```typescript
   export async function proveXxx(params: {...}): Promise<ZKProof> {
       const snarkjs = await getSnarkjs();
       const { proof, publicSignals } = await snarkjs.groth16.fullProve(
           input,
           "/zk/xxx.wasm",
           "/zk/xxx_final.zkey"
       );
       return { circuit: "xxx", proof, publicSignals, ... };
   }
   ```

4. **Backend doğrulama** — `/v1/zk/verify` endpoint zaten var
   - Yeni circuit_id eklenmesi gerekiyorsa `backend/internal/api/handlers.go`'daki verify handler'ı güncelle

5. **Build script'i güncelle** — `circuits/build.sh`'e yeni circuit'i ekle

6. **CLAUDE.md'yi güncelle** — devreyi YAPILDI listesine taşı

## Önemli Notlar
- Poseidon hash: circomlibjs ile (SHA-256 fallback sadece test için)
- BN254 field: max 254-bit integer
- Negatif sayılar için offset ekle (örn: score -20..100 → +20 ile 0..120 yap)
- `fullProve` sadece browser'da çalışır (SSR'da hata verir)
- .wasm ve .zkey dosyaları `frontend/public/zk/` altına koy
