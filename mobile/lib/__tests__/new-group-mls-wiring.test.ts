// L2 Tuğla 5b-2 Parça D — new-group.tsx'in artık MLS akışına (createGroupFlow.ts)
// bağlı olduğunu, eski düz-plaintext createConversation({type:"group"}) yolunun
// KALMADIĞINI doğrular (Ders-4: plaintext-capable yol var olmamalı).
//
// Bu ekranın render/etkileşim testi yok (proje genelinde hiçbir app/ ekranı
// için RTL/react-test-renderer harness'ı kurulu değil — mls-createGroupFlow.test.ts
// zaten koordinasyon mantığının TAMAMINI (sıra, local-önce, kısmi-başarısızlık)
// mock'larla kapsıyor). Burada test edilen tek şey: ekranın submit'i doğru
// modülü çağırıyor mu, yoksa eski plaintext çağrıyı mı koruyor — kaynak
// metni üzerinden, mimari-uyum kontrolü olarak.
import * as fs from "fs";
import * as path from "path";

const SCREEN_PATH = path.join(__dirname, "../../app/(main)/new-group.tsx");

function readScreenSource(): string {
  return fs.readFileSync(SCREEN_PATH, "utf-8");
}

describe("new-group.tsx — MLS akışına bağlı (Tuğla 5b-2 Parça D)", () => {
  test("createMlsGroupConversation'ı import ediyor ve submit'te çağırıyor", () => {
    const src = readScreenSource();
    expect(src).toMatch(/createMlsGroupConversation/);
    expect(src).toMatch(/from ["']@\/lib\/mls\/createGroupFlow["']/);
  });

  test("eski plaintext grup-oluşturma çağrısı (api.createConversation({ type: \"group\" ...)) KALMADI", () => {
    const src = readScreenSource();
    // Eski akışın imzası: api.createConversation çağrısına DOĞRUDAN
    // type: "group" geçilmesi. MLS akışına taşındıktan sonra bu literal
    // kalıp ekranda görünmemeli (api.createConversation artık sadece
    // createGroupFlow.ts'in İÇİNDE çağrılıyor, ekranda değil).
    expect(src).not.toMatch(/api\.createConversation\(\s*\{\s*[\s\S]{0,40}type:\s*["']group["']/);
  });
});
