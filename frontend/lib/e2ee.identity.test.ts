import { describe, it, expect } from "vitest";
import {
  generateIdentity,
  saveIdentity,
  loadIdentity,
  createRunOnceGuard,
  getOrCreateIdentity,
} from "./e2ee";

describe("web kimlik üretimi — idempotency (login/page.tsx doVerify deseni)", () => {
  it("ardışık iki eşzamanlı çağrı guard ile tek kimlik üretir, ikincisi sessizce düşer", async () => {
    const passphrase = `race_test_${Date.now()}_${Math.random()}`;
    const guard = createRunOnceGuard();

    async function doVerifyLike() {
      if (!guard.tryEnter()) return null;
      try {
        return await getOrCreateIdentity(passphrase);
      } finally {
        guard.exit();
      }
    }

    const [first, second] = await Promise.all([doVerifyLike(), doVerifyLike()]);

    expect([first, second].filter((r) => r !== null)).toHaveLength(1);

    const winner = (first ?? second)!;
    const persisted = await loadIdentity(passphrase);
    expect(persisted).not.toBeNull();
    expect(persisted!.did).toBe(winner.did);
  });

  it("kimlik zaten varken getOrCreateIdentity yeniden üretmez, mevcut kimliği döndürür", async () => {
    const passphrase = `existing_test_${Date.now()}_${Math.random()}`;
    const original = await generateIdentity();
    await saveIdentity(original, passphrase);

    const resolved = await getOrCreateIdentity(passphrase);

    expect(resolved.did).toBe(original.did);
    expect(Array.from(resolved.dhKeyPair.publicKeyBytes)).toEqual(
      Array.from(original.dhKeyPair.publicKeyBytes)
    );
  });
});
