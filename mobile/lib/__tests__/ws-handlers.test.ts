import { handleMessageExpired } from "../ws-handlers";
import { cachePlaintext, getCachedPlaintext } from "../plaintext-cache";
import { createMemoryStore } from "../../test-utils/memoryStore";

describe("handleMessageExpired — WS message_expired event işleyicisi", () => {
  test("mesaj status='expired' olarak güncellenir", async () => {
    const stores = { secure: createMemoryStore(), async: createMemoryStore() };
    const updateMsgStatus = jest.fn();

    await handleMessageExpired({ message_id: "msg-1", conv_id: "c1", expired_at: 123 }, updateMsgStatus, stores);

    expect(updateMsgStatus).toHaveBeenCalledWith("msg-1", "expired");
  });

  test("self-destruct'ın kalbi: plaintext-cache'teki çözülmüş metin PURGE edilir", async () => {
    const stores = { secure: createMemoryStore(), async: createMemoryStore() };
    await cachePlaintext("msg-2", "çözülmüş gizli metin", stores);
    expect(await getCachedPlaintext("msg-2", stores)).toBe("çözülmüş gizli metin");

    await handleMessageExpired({ message_id: "msg-2", conv_id: "c1", expired_at: 123 }, jest.fn(), stores);

    expect(await getCachedPlaintext("msg-2", stores)).toBeNull();
  });

  test("message_id eksikse no-op — updateMsgStatus çağrılmaz, hata fırlatmaz", async () => {
    const stores = { secure: createMemoryStore(), async: createMemoryStore() };
    const updateMsgStatus = jest.fn();

    await expect(handleMessageExpired({} as any, updateMsgStatus, stores)).resolves.not.toThrow();
    expect(updateMsgStatus).not.toHaveBeenCalled();
  });
});
