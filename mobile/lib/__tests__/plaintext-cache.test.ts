import { cachePlaintext, getCachedPlaintext, purgePlaintext } from "../plaintext-cache";
import { createMemoryStore } from "../../test-utils/memoryStore";

describe("plaintext-cache.ts — purgePlaintext (self-destruct'ın kalbi)", () => {
  test("purge sonrası mesajın plaintext'i cache'ten OKUNAMAZ — backend ciphertext'i silse bile client'ta düz metin kalmamalı", async () => {
    const stores = { secure: createMemoryStore(), async: createMemoryStore() };
    await cachePlaintext("msg-1", "gizli mesaj içeriği", stores);

    // Purge öncesi: normal şekilde okunabiliyor (sağlık kontrolü)
    expect(await getCachedPlaintext("msg-1", stores)).toBe("gizli mesaj içeriği");

    await purgePlaintext("msg-1", stores);

    expect(await getCachedPlaintext("msg-1", stores)).toBeNull();
  });

  test("purge sadece hedef mesajı siler, diğer mesajların cache'i etkilenmez", async () => {
    const stores = { secure: createMemoryStore(), async: createMemoryStore() };
    await cachePlaintext("msg-a", "mesaj a", stores);
    await cachePlaintext("msg-b", "mesaj b", stores);

    await purgePlaintext("msg-a", stores);

    expect(await getCachedPlaintext("msg-a", stores)).toBeNull();
    expect(await getCachedPlaintext("msg-b", stores)).toBe("mesaj b");
  });

  test("var olmayan mesaj id'si için purge sessizce no-op (hata fırlatmaz)", async () => {
    const stores = { secure: createMemoryStore(), async: createMemoryStore() };
    await expect(purgePlaintext("no-such-message", stores)).resolves.not.toThrow();
  });
});
