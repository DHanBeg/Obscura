// L2 Tuğla 5c — mls-invites.tsx ekranının getWelcomes + acceptMlsWelcome'a
// bağlı olduğunu doğrular. new-group-mls-wiring.test.ts ile AYNI desen ve
// AYNI gerekçe: proje genelinde app/ ekranları için RTL/react-test-renderer
// harness'ı kurulu değil, koordinasyon mantığının tamamı (sıra, local-önce,
// kısmi-başarısızlık) mls-joinGroupFlow.test.ts'te mock'larla kapsanıyor —
// burada test edilen tek şey: ekran doğru modülü çağırıyor mu.
import * as fs from "fs";
import * as path from "path";

const SCREEN_PATH = path.join(__dirname, "../../app/(main)/mls-invites.tsx");

function readScreenSource(): string {
  return fs.readFileSync(SCREEN_PATH, "utf-8");
}

describe("mls-invites.tsx — welcome kabul akışına bağlı (Tuğla 5c)", () => {
  test("ekran var, getWelcomes ve acceptMlsWelcome'ı import edip çağırıyor", () => {
    const src = readScreenSource();
    expect(src).toMatch(/getWelcomes/);
    expect(src).toMatch(/acceptMlsWelcome/);
    expect(src).toMatch(/from ["']@\/lib\/mls\/joinGroupFlow["']/);
  });
});
