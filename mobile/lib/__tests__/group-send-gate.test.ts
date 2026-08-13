import { resolveSendTarget, shouldFetchConversationHistory, isGroupSendBlocked, classifyConv, SendGateConversation } from "../group-send-gate";

describe("classifyConv", () => {
  test("is_group true ise 'group'", () => {
    expect(classifyConv({ id: "c1", is_group: true })).toBe("group");
  });

  test("is_group false ise 'direct'", () => {
    expect(classifyConv({ id: "c1", is_group: false, peer_did: "did:obscura:peer" })).toBe("direct");
  });

  test("is_group undefined ise 'unknown'", () => {
    expect(classifyConv({ id: "c1", is_group: undefined })).toBe("unknown");
  });

  test("is_group null ise 'unknown'", () => {
    const conv = { id: "c1", is_group: null } as unknown as SendGateConversation;
    expect(classifyConv(conv)).toBe("unknown");
  });

  test("conv yoksa 'unknown'", () => {
    expect(classifyConv(undefined)).toBe("unknown");
  });
});

describe("isGroupSendBlocked", () => {
  test("grup konuşmasında gönderim bloklanır", () => {
    expect(isGroupSendBlocked({ id: "c1", is_group: true })).toBe(true);
  });

  test("1:1 konuşmada gönderime izin verilir", () => {
    expect(isGroupSendBlocked({ id: "c1", is_group: false, peer_did: "did:obscura:peer" })).toBe(false);
  });

  test("is_group undefined ise bloklanır (fail-closed)", () => {
    expect(isGroupSendBlocked({ id: "c1", is_group: undefined })).toBe(true);
  });

  test("is_group null ise bloklanır", () => {
    const conv = { id: "c1", is_group: null } as unknown as SendGateConversation;
    expect(isGroupSendBlocked(conv)).toBe(true);
  });

  test("conv yoksa bloklanır", () => {
    expect(isGroupSendBlocked(undefined)).toBe(true);
  });
});

describe("resolveSendTarget", () => {
  test("grup konuşmasında hedef conv.id (peer_did yok, önemli değil)", () => {
    const conv: SendGateConversation = { id: "conv-group-1", is_group: true };
    expect(resolveSendTarget(conv)).toBe("conv-group-1");
  });

  test("1:1 konuşmada hedef peer_did", () => {
    const conv: SendGateConversation = { id: "conv-dm-1", is_group: false, peer_did: "did:obscura:peer" };
    expect(resolveSendTarget(conv)).toBe("did:obscura:peer");
  });

  test("1:1 konuşmada peer_did boşsa null (gönderim yapılamaz)", () => {
    const conv: SendGateConversation = { id: "conv-dm-2", is_group: false };
    expect(resolveSendTarget(conv)).toBeNull();
  });

  test("conv undefined ise null", () => {
    expect(resolveSendTarget(undefined)).toBeNull();
  });

  test("is_group belirsiz (undefined) ise hata fırlatır — 1:1'e TAHMİNLE düşmez", () => {
    const conv: SendGateConversation = { id: "conv-unknown-1", is_group: undefined, peer_did: "did:obscura:peer" };
    expect(() => resolveSendTarget(conv)).toThrow(/is_group belirsiz/);
  });

  test("is_group belirsiz (null) ise hata fırlatır", () => {
    const conv = { id: "conv-unknown-2", is_group: null } as unknown as SendGateConversation;
    expect(() => resolveSendTarget(conv)).toThrow(/is_group belirsiz/);
  });
});

describe("shouldFetchConversationHistory", () => {
  test("grup konuşmasında (peer_did YOK) fetch yine de yapılmalı", () => {
    const conv: SendGateConversation = { id: "conv-group-1", is_group: true };
    expect(shouldFetchConversationHistory("conv-group-1", conv)).toBe(true);
  });

  test("1:1 konuşmada (peer_did VAR) fetch yapılmalı", () => {
    const conv: SendGateConversation = { id: "conv-dm-1", is_group: false, peer_did: "did:obscura:peer" };
    expect(shouldFetchConversationHistory("conv-dm-1", conv)).toBe(true);
  });

  test("convId yoksa fetch yapılmamalı", () => {
    const conv: SendGateConversation = { id: "conv-group-1", is_group: true };
    expect(shouldFetchConversationHistory(undefined, conv)).toBe(false);
  });

  test("conv henüz yüklenmemişse (store'da yok) fetch yapılmamalı", () => {
    expect(shouldFetchConversationHistory("conv-not-loaded-yet", undefined)).toBe(false);
  });
});
