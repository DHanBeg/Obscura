import { readFileSync } from "fs";
import { join } from "path";
import { x25519 } from "@noble/curves/ed25519.js";
import {
  RatchetState,
  kdfRk,
  kdfCk,
  headerToBytes,
  headerFromBytes,
  encryptedMessageToBytes,
  encryptedMessageFromBytes,
  MessageHeader,
} from "../ratchet";
import { hexToU8, u8ToHex } from "../crypto";

const VECTORS_PATH = join(__dirname, "..", "..", "..", "crypto", "test-vectors", "x3dh_ratchet_vectors.json");
const vectors = JSON.parse(readFileSync(VECTORS_PATH, "utf8"));

describe("ratchet.ts — KDF vektörleri (Rust crypto/src/ratchet.rs ile birebir)", () => {
  test("kdf_ck_vector: chain_key → message_key + new_chain_key", () => {
    const v = vectors.ratchet_kdf_ck_vector;
    const { chainKey, messageKey } = kdfCk(hexToU8(v.input.chain_key));
    expect(u8ToHex(messageKey)).toBe(v.output.message_key_hex);
    expect(u8ToHex(chainKey)).toBe(v.output.new_chain_key_hex);
  });

  test("kdf_rk_vector: (root_key, dh_output) → chain_key + new_root_key", () => {
    const v = vectors.ratchet_kdf_rk_vector;
    const { rootKey, chainKey } = kdfRk(hexToU8(v.input.root_key), hexToU8(v.input.dh_output));
    expect(u8ToHex(chainKey)).toBe(v.output.chain_key_hex);
    expect(u8ToHex(rootKey)).toBe(v.output.new_root_key_hex);
  });
});

describe("ratchet.ts — session vektörü (init_sender → encrypt → Bob decrypt)", () => {
  const v = vectors.ratchet_session_vector;

  test("init_sender_with_dhs sonrası chain_key/root_key vector ile birebir eşleşiyor", () => {
    // RatchetState private state tutuyor (kasıtlı — encapsulation), bu yüzden
    // initSender'ın İÇİNDE çağırdığı AYNI primitifi (DH + kdfRk) burada
    // bağımsızca tekrar hesaplayıp vector ile karşılaştırıyoruz.
    const aliceDhsPriv = hexToU8(v.input.alice_dhs_priv);
    const bobRatchetPub = hexToU8(v.input.bob_ratchet_pub);
    const sharedKey = hexToU8(v.input.shared_key);

    const dhOut = x25519.getSharedSecret(aliceDhsPriv, bobRatchetPub);
    const { rootKey, chainKey } = kdfRk(sharedKey, dhOut);

    expect(u8ToHex(chainKey)).toBe(v.output.alice_chain_key_after_init_hex);
    expect(u8ToHex(rootKey)).toBe(v.output.alice_root_key_after_init_hex);
  });

  test("bu chain_key'den ilk encrypt'in message_key'i vector ile eşleşiyor", () => {
    const { chainKey } = kdfRk(hexToU8(v.input.shared_key), x25519.getSharedSecret(hexToU8(v.input.alice_dhs_priv), hexToU8(v.input.bob_ratchet_pub)));
    const { messageKey } = kdfCk(chainKey);
    expect(u8ToHex(messageKey)).toBe(v.output.message_key_hex);
  });

  test("gerçek RatchetState.initSender + encrypt() → header_hex vector ile birebir, Bob roundtrip başarılı", () => {
    const alice = RatchetState.initSender(
      hexToU8(v.input.shared_key),
      hexToU8(v.input.bob_ratchet_pub),
      hexToU8(v.input.alice_dhs_priv)
    );
    const bob = RatchetState.initReceiver(hexToU8(v.input.shared_key), hexToU8(v.input.bob_ratchet_priv));

    const ad = new TextEncoder().encode(v.input.ad);
    const plaintext = new TextEncoder().encode(v.input.plaintext);

    const enc = alice.encrypt(plaintext, ad);
    expect(u8ToHex(headerToBytes(enc.header))).toBe(v.output.header_hex);

    const dec = bob.decrypt(enc, ad);
    expect(new TextDecoder().decode(dec)).toBe(v.output.decrypted_plaintext);
    expect(v.output.roundtrip_ok).toBe(true);
  });
});

describe("ratchet.ts — header / wire format", () => {
  test("header 40 byte'a serialize/deserialize ediliyor (round-trip)", () => {
    const header: MessageHeader = { dhPub: new Uint8Array(32).fill(0xab), pn: 42, n: 7 };
    const bytes = headerToBytes(header);
    expect(bytes.length).toBe(40);
    const parsed = headerFromBytes(bytes);
    expect(u8ToHex(parsed.dhPub)).toBe(u8ToHex(header.dhPub));
    expect(parsed.pn).toBe(42);
    expect(parsed.n).toBe(7);
  });

  test("EncryptedMessage wire format round-trip (gerçek şifreleme ile)", () => {
    const sharedKey = new Uint8Array(32).fill(0x42);
    const bob = x25519.keygen();
    const alice = RatchetState.initSender(sharedKey, bob.publicKey);
    const bobState = RatchetState.initReceiver(sharedKey, bob.secretKey);

    const ad = new TextEncoder().encode("wire-test");
    const pt = new TextEncoder().encode("Wire format test mesaji");
    const enc = alice.encrypt(pt, ad);
    const wire = encryptedMessageToBytes(enc);
    const parsed = encryptedMessageFromBytes(wire);

    const dec = bobState.decrypt(parsed, ad);
    expect(new TextDecoder().decode(dec)).toBe("Wire format test mesaji");
  });
});

describe("ratchet.ts — temel gönder/al ve çift yönlü DH ratchet (Rust unit testleriyle paralel)", () => {
  function makeSession() {
    const sharedKey = new Uint8Array(32).fill(0x42);
    const bobRatchet = x25519.keygen();
    const alice = RatchetState.initSender(sharedKey, bobRatchet.publicKey);
    const bob = RatchetState.initReceiver(sharedKey, bobRatchet.secretKey);
    return { alice, bob };
  }

  test("basic_send_receive", () => {
    const { alice, bob } = makeSession();
    const ad = new TextEncoder().encode("conv-id-001");
    const pt = new TextEncoder().encode("Merhaba Bob!");
    const enc = alice.encrypt(pt, ad);
    const dec = bob.decrypt(enc, ad);
    expect(new TextDecoder().decode(dec)).toBe("Merhaba Bob!");
  });

  test("multi_message_alice_to_bob — 10 ardışık mesaj, her seferinde yeni chain key", () => {
    const { alice, bob } = makeSession();
    const ad = new TextEncoder().encode("conv-001");
    const seenHeaders = new Set<string>();
    for (let i = 0; i < 10; i++) {
      const pt = new Uint8Array(64).fill(i);
      const enc = alice.encrypt(pt, ad);
      // her mesajın n'i artmalı, aynı header iki kez üretilmemeli
      expect(enc.header.n).toBe(i);
      expect(seenHeaders.has(u8ToHex(headerToBytes(enc.header)))).toBe(false);
      seenHeaders.add(u8ToHex(headerToBytes(enc.header)));

      const dec = bob.decrypt(enc, ad);
      expect(u8ToHex(dec)).toBe(u8ToHex(pt));
    }
  });

  test("bidirectional_ratchet — Bob cevap verince DH ratchet tetiklenir, dh_pub değişir", () => {
    const { alice, bob } = makeSession();
    const ad = new TextEncoder().encode("conv-bidir");

    const m1 = alice.encrypt(new TextEncoder().encode("A-to-B message 1"), ad);
    const d1 = bob.decrypt(m1, ad);
    expect(new TextDecoder().decode(d1)).toBe("A-to-B message 1");

    const m2 = bob.encrypt(new TextEncoder().encode("B-to-A reply"), ad);
    // Bob'un DH ratchet public key'i Alice'in ilk gördüğü bob_ratchet_pub'dan FARKLI
    // olmalı — bob.encrypt() henüz kendi tarafında DH adımı yapmadı (init_sender/receiver
    // simetrik değil), ama Alice bunu decrypt ederken dh_ratchet tetiklenmeli.
    const d2 = alice.decrypt(m2, ad);
    expect(new TextDecoder().decode(d2)).toBe("B-to-A reply");

    const m3 = alice.encrypt(new TextEncoder().encode("A-to-B message 2"), ad);
    // Alice artık YENİ bir dh_pub kullanıyor olmalı (dh_ratchet sırasında üretildi)
    expect(u8ToHex(m3.header.dhPub)).not.toBe(u8ToHex(m1.header.dhPub));
    const d3 = bob.decrypt(m3, ad);
    expect(new TextDecoder().decode(d3)).toBe("A-to-B message 2");
  });
});

describe("ratchet.ts — out-of-order ve skipped messages (KRİTİK)", () => {
  function makeSession() {
    const sharedKey = new Uint8Array(32).fill(0x77);
    const bobRatchet = x25519.keygen();
    const alice = RatchetState.initSender(sharedKey, bobRatchet.publicKey);
    const bob = RatchetState.initReceiver(sharedKey, bobRatchet.secretKey);
    return { alice, bob };
  }

  test("sırasız gelen mesajlar (2,0,1) doğru çözülüyor", () => {
    const { alice, bob } = makeSession();
    const ad = new TextEncoder().encode("ooo-conv");
    const msgs = [0, 1, 2].map((i) => alice.encrypt(new TextEncoder().encode(`msg-${i}`), ad));

    // ters sırayla teslim: 2, sonra 0, sonra 1
    const d2 = bob.decrypt(msgs[2], ad);
    expect(new TextDecoder().decode(d2)).toBe("msg-2");
    const d0 = bob.decrypt(msgs[0], ad);
    expect(new TextDecoder().decode(d0)).toBe("msg-0");
    const d1 = bob.decrypt(msgs[1], ad);
    expect(new TextDecoder().decode(d1)).toBe("msg-1");
  });

  test("kaybolan bir mesaj (1) sonra geç gelirse — 0,2,3 teslim edilir, sonra 1 geç gelir, hepsi çözülür", () => {
    const { alice, bob } = makeSession();
    const ad = new TextEncoder().encode("late-msg-conv");
    const msgs = [0, 1, 2, 3].map((i) => alice.encrypt(new TextEncoder().encode(`m${i}`), ad));

    expect(new TextDecoder().decode(bob.decrypt(msgs[0], ad))).toBe("m0");
    // msg[1] KAYIP — direkt msg[2]'ye atla, bu Bob'un ckr'ini ilerletirken
    // msg[1]'in anahtarını mk_skipped'e saklamalı
    expect(new TextDecoder().decode(bob.decrypt(msgs[2], ad))).toBe("m2");
    expect(bob.skippedCount).toBe(1);
    expect(new TextDecoder().decode(bob.decrypt(msgs[3], ad))).toBe("m3");
    expect(bob.skippedCount).toBe(1);

    // msg[1] GEÇ gelir — hâlâ skipped buffer'dan çözülebilmeli
    expect(new TextDecoder().decode(bob.decrypt(msgs[1], ad))).toBe("m1");
    expect(bob.skippedCount).toBe(0); // kullanılan skipped key silindi
  });

  test("DH ratchet SONRASI eski zincirden atlanan mesaj hâlâ çözülebiliyor (çapraz-zincir skip)", () => {
    const { alice, bob } = makeSession();
    const ad = new TextEncoder().encode("cross-chain-conv");

    const a0 = alice.encrypt(new TextEncoder().encode("a0"), ad);
    const a1 = alice.encrypt(new TextEncoder().encode("a1"), ad); // Bob bunu hiç almayacak (kayıp)
    expect(new TextDecoder().decode(bob.decrypt(a0, ad))).toBe("a0");

    // Bob cevap verir → DH ratchet tetiklenir, Alice'in eski zincirinden a1 artık "eski dh_pub" zincirine ait
    const b0 = bob.encrypt(new TextEncoder().encode("b0"), ad);
    expect(new TextDecoder().decode(alice.decrypt(b0, ad))).toBe("b0");

    // Alice yeni zincirde devam eder
    const a2 = alice.encrypt(new TextEncoder().encode("a2"), ad);
    expect(new TextDecoder().decode(bob.decrypt(a2, ad))).toBe("a2");

    // a1 GEÇ gelir — Bob'un eski zincirden sakladığı skipped key ile hâlâ çözülmeli
    expect(new TextDecoder().decode(bob.decrypt(a1, ad))).toBe("a1");
  });

  test("MAX_SKIP (1000) aşılırsa reddedilir — sınırsız saklama DoS riski", () => {
    const { alice, bob } = makeSession();
    const ad = new TextEncoder().encode("dos-conv");

    let last;
    for (let i = 0; i < 1002; i++) {
      last = alice.encrypt(new TextEncoder().encode(`x${i}`), ad);
    }
    // Bob HİÇBİRİNİ almadı, direkt son mesajı (n=1001) çözmeye çalışıyor —
    // nr(0) + MAX_SKIP(1000) < 1001 → reddedilmeli
    expect(() => bob.decrypt(last!, ad)).toThrow(/Çok fazla mesaj atlandı/);
  });
});

describe("ratchet.ts — forward secrecy: kullanılan mesaj anahtarı geri kullanılamaz", () => {
  test("skipped key bir kez tüketildikten sonra aynı mesaj TEKRAR çözülemez", () => {
    const sharedKey = new Uint8Array(32).fill(0x99);
    const bobRatchet = x25519.keygen();
    const alice = RatchetState.initSender(sharedKey, bobRatchet.publicKey);
    const bob = RatchetState.initReceiver(sharedKey, bobRatchet.secretKey);
    const ad = new TextEncoder().encode("replay-conv");

    const m0 = alice.encrypt(new TextEncoder().encode("m0"), ad);
    const m1 = alice.encrypt(new TextEncoder().encode("m1"), ad);

    bob.decrypt(m1, ad); // m0 atlanır, skipped'e kaydedilir
    expect(bob.skippedCount).toBe(1);
    bob.decrypt(m0, ad); // skipped key tüketilir, silinir
    expect(bob.skippedCount).toBe(0);

    // AYNI mesajı tekrar göndermeye çalış — skipped map'te artık yok,
    // normal zincirden de çözülemez (nr zaten ilerlemiş) → hata fırlatmalı
    expect(() => bob.decrypt(m0, ad)).toThrow();
  });

  test("destroy() sonrası RatchetState kullanılamaz hale geliyor", () => {
    const sharedKey = new Uint8Array(32).fill(0x11);
    const bobRatchet = x25519.keygen();
    const alice = RatchetState.initSender(sharedKey, bobRatchet.publicKey);
    alice.destroy();
    expect(() => alice.encrypt(new TextEncoder().encode("x"), new Uint8Array(0))).toThrow(/destroy/);
  });
});

describe("ratchet.ts — bütünlük: yanlış AD veya bozulmuş ciphertext reddedilir", () => {
  test("yanlış additional data ile decrypt başarısız olur (AEAD bütünlüğü)", () => {
    const sharedKey = new Uint8Array(32).fill(0x22);
    const bobRatchet = x25519.keygen();
    const alice = RatchetState.initSender(sharedKey, bobRatchet.publicKey);
    const bob = RatchetState.initReceiver(sharedKey, bobRatchet.secretKey);

    const enc = alice.encrypt(new TextEncoder().encode("gizli"), new TextEncoder().encode("conv-a"));
    expect(() => bob.decrypt(enc, new TextEncoder().encode("conv-b"))).toThrow();
  });

  test("bozulmuş ciphertext decrypt başarısız olur", () => {
    const sharedKey = new Uint8Array(32).fill(0x33);
    const bobRatchet = x25519.keygen();
    const alice = RatchetState.initSender(sharedKey, bobRatchet.publicKey);
    const bob = RatchetState.initReceiver(sharedKey, bobRatchet.secretKey);
    const ad = new TextEncoder().encode("tamper-conv");

    const enc = alice.encrypt(new TextEncoder().encode("gizli mesaj"), ad);
    const tampered = enc.ciphertext.slice();
    tampered[tampered.length - 1] ^= 0xff; // son byte'ı (auth tag içinde) boz
    expect(() => bob.decrypt({ header: enc.header, ciphertext: tampered }, ad)).toThrow();
  });
});
