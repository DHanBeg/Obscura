// L2 Tuğla 5c bölüm 2 — ensureInvitable'ın (1) app açılışında (_layout.tsx,
// sessiz/best-effort — Bob'un MLS ekranını hiç açmadan davet-edilebilir
// olması için, aksi halde tavuk-yumurta: davet ekranı sadece davet
// ALINDIKTAN sonra ziyaret edilir) ve (2) mls-invites.tsx'te (görünür hata
// ile, kullanıcı MLS'e bakarken) çağrıldığını doğrular. Kaynak-seviyesi
// kontrol — new-group-mls-wiring.test.ts ile aynı gerekçe (RTL harness yok).
import * as fs from "fs";
import * as path from "path";

function readSource(rel: string): string {
  return fs.readFileSync(path.join(__dirname, "../..", rel), "utf-8");
}

describe("ensureInvitable bağlantıları (Tuğla 5c bölüm 2)", () => {
  test("app/_layout.tsx: girişten sonra sessiz/best-effort çağrı", () => {
    const src = readSource("app/_layout.tsx");
    expect(src).toMatch(/ensureInvitable/);
    expect(src).toMatch(/from ["']@\/lib\/mls\/inviteBootstrap["']/);
  });

  test("app/(main)/mls-invites.tsx: görünür hata ile çağrı", () => {
    const src = readSource("app/(main)/mls-invites.tsx");
    expect(src).toMatch(/ensureInvitable/);
    expect(src).toMatch(/from ["']@\/lib\/mls\/inviteBootstrap["']/);
  });
});
