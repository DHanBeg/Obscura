import React from "react";
import TestRenderer, { act } from "react-test-renderer";
import { TextInput } from "react-native";
import * as SecureStore from "expo-secure-store";
import * as cryptoMod from "../crypto";
import { api } from "../api";
import LoginScreen from "../../app/(auth)/login";

// login.tsx "@/..." alias'ı kullanıyor; jest'te bu alias tanımlı değil ve
// config'e dokunmuyoruz. Virtual mock, alias'ı GERÇEK modüle bağlar
// (requireActual) — sahte bir implementasyon değil, aynı modül örneği.
jest.mock("@/lib/theme", () => jest.requireActual("../theme"), { virtual: true });
jest.mock("@/lib/e2e", () => jest.requireActual("../e2e"), { virtual: true });
jest.mock("@/lib/keys-sync", () => jest.requireActual("../keys-sync"), { virtual: true });
jest.mock("@/lib/mnemonic", () => jest.requireActual("../mnemonic"), { virtual: true });
jest.mock("@/lib/api", () => jest.requireMock("../api"), { virtual: true });
jest.mock("@/assets/logo.jpeg", () => 1, { virtual: true });

// Yalnız native/ağ sınırları sahte: SecureStore (bellek-içi), router, haptics, api.
jest.mock("expo-secure-store", () => {
  const mockStore = new Map<string, string>();
  return {
    __store: mockStore,
    getItemAsync: jest.fn(async (k: string) => (mockStore.has(k) ? mockStore.get(k)! : null)),
    setItemAsync: jest.fn(async (k: string, v: string) => {
      mockStore.set(k, v);
    }),
    deleteItemAsync: jest.fn(async (k: string) => {
      mockStore.delete(k);
    }),
  };
});
jest.mock("expo-router", () => ({ router: { replace: jest.fn() } }));
jest.mock("expo-haptics", () => ({
  notificationAsync: jest.fn(async () => {}),
  NotificationFeedbackType: { Success: "success", Error: "error" },
}));
jest.mock("@expo/vector-icons", () => ({ Ionicons: "Ionicons" }));
jest.mock("../api", () => ({
  api: {
    requestOTP: jest.fn(async () => ({})),
    verifyOTP: jest.fn(async () => ({ token: "tok", is_new: true })),
    getOPKCount: jest.fn(async () => ({ count: 100, low: false, critical: false })),
    uploadPrekeys: jest.fn(async () => ({})),
    replenishOPK: jest.fn(async () => ({})),
  },
}));

(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;

const KEY_PRIV = "obscura_x25519_private";

function findByText(tree: TestRenderer.ReactTestRenderer, text: string) {
  const nodes = tree.root.findAll(
    (n) => typeof n.props?.onPress === "function" && n.findAll((c) => c.props?.children === text).length > 0
  );
  if (nodes.length === 0) throw new Error(`"${text}" düğmesi bulunamadı`);
  return nodes[0];
}

describe("mobile login: OTP auto-submit (80ms) + manuel buton yarışı", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    (SecureStore as any).__store.clear();
    jest.clearAllMocks();
  });
  afterEach(() => {
    jest.useRealTimers();
  });

  // Web'de Kat.1'de kapatılan sınıfın mobile karşılığı: cihazda henüz kimlik yokken
  // iki verifyOTP çağrısı yarışırsa her biri getOrCreateKeyPair'de "yok" görüp FARKLI
  // X25519 kimliği üretebilir. verifyingRef kilidi ikinci çağrıyı sessizce düşürmeli.
  it("son hane yazıldıktan sonra 80ms dolmadan 'Doğrula'ya basılırsa TEK kimlik üretilir, sunucuya TEK identity_key gider", async () => {
    const genSpy = jest.spyOn(cryptoMod, "generateX25519KeyPair");
    let tree!: TestRenderer.ReactTestRenderer;

    await act(async () => {
      tree = TestRenderer.create(<LoginScreen />);
    });

    // Telefon adımı → OTP adımı
    await act(async () => {
      tree.root.findByType(TextInput).props.onChangeText("+905551234567");
    });
    await act(async () => {
      await findByText(tree, "Kod gönder").props.onPress();
    });
    expect(tree.root.findAllByType(TextInput)).toHaveLength(6);

    // İlk 5 haneyi yaz (her state güncellemesinden sonra taze closure'lar için yeniden bul)
    for (let i = 0; i < 5; i++) {
      await act(async () => {
        tree.root.findAllByType(TextInput)[i].props.onChangeText(String(i + 1));
      });
    }

    // Son hane: 80ms'lik auto-submit zamanlayıcısı kurulur. Timer dolmadan kullanıcı butona basar.
    await act(async () => {
      tree.root.findAllByType(TextInput)[5].props.onChangeText("6");
    });
    const verifyBtn = findByText(tree, "Doğrula");
    expect(verifyBtn.props.disabled).toBe(false); // gerçek kullanıcı da basabilirdi

    await act(async () => {
      verifyBtn.props.onPress(); // manuel (login.tsx:217)
      jest.advanceTimersByTime(80); // auto (login.tsx:122) — ilki hâlâ uçuştayken
    });
    // Akışın tamamlanması için kalan async işleri boşalt.
    await act(async () => {
      await Promise.resolve();
      jest.advanceTimersByTime(1000);
    });
    await act(async () => {
      for (let i = 0; i < 20; i++) await Promise.resolve();
    });

    const sentKeys = (api.verifyOTP as jest.Mock).mock.calls.map((c) => c[0].identity_key as string);

    expect(genSpy).toHaveBeenCalledTimes(1);
    expect(api.verifyOTP).toHaveBeenCalledTimes(1);
    expect(new Set(sentKeys).size).toBe(1);

    // Sunucuya giden anahtar = cihazda kalıcı saklanan anahtar (ayrışma yok).
    const storedPrivHex = (SecureStore as any).__store.get(KEY_PRIV) as string;
    expect(storedPrivHex).toBeTruthy();
    const storedPub = cryptoMod.x25519PublicKey(cryptoMod.hexToU8(storedPrivHex));
    expect(sentKeys[0]).toBe(cryptoMod.u8ToBase64(storedPub));

    await act(async () => {
      tree.unmount();
    });
  }, 60000); // soğuk jest transform önbelleğinde RN modüllerinin ilk yüklemesi 5 sn'yi aşabiliyor
});
