// L2 Tuğla 2 (Kısım B) — RFC 9420 resmi passive-client-welcome vektörü.
// Fizibilite bulgusu: key-schedule/treekem gibi vektörler iç-secret istiyor,
// ts-mls'in public API'siyle okunamaz. passive-client-welcome TEK istisna —
// sadece Welcome işleyip pasif üye olarak katılmak (public API) yeterli,
// doğrulama noktası joinGroup SONRASI state.keySchedule.epochAuthenticator'ın
// vektörün initial_epoch_authenticator'ıyla eşleşmesi.
//
// Vektör dosyası mlswg/mls-implementations (RFC 9420 resmi interop suite)
// reposundan indirilip repoya kalıcı olarak kondu — test-time fetch YOK,
// offline çalışır: lib/mls/fixtures/rfc-vectors/passive-client-welcome.json
import * as fs from "fs";
import * as path from "path";
import { joinFromRfcVector, type PassiveClientWelcomeVector } from "../mls/rfcVector";
import { getMlsCiphersuiteImpl } from "../mls/group";

const VECTOR_PATH = path.join(__dirname, "../mls/fixtures/rfc-vectors/passive-client-welcome.json");
const TARGET_SUITE = 1; // MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519

describe("RFC 9420 passive-client-welcome vektörü — suite 1", () => {
  const allVectors: PassiveClientWelcomeVector[] = JSON.parse(fs.readFileSync(VECTOR_PATH, "utf-8"));
  const suite1Vectors = allVectors.filter((v) => v.cipher_suite === TARGET_SUITE);

  test("dosyada suite-1 vektörü var", () => {
    expect(suite1Vectors.length).toBeGreaterThan(0);
  });

  test.each(suite1Vectors.map((v, i) => [i, v] as const))(
    "vektör #%i: ts-mls Welcome'ı işleyip resmi epoch_authenticator'a ulaşıyor",
    async (_i, vector) => {
      const cs = await getMlsCiphersuiteImpl();
      const state = await joinFromRfcVector(vector, cs);
      const actualHex = Buffer.from(state.keySchedule.epochAuthenticator).toString("hex");
      expect(actualHex).toBe(vector.initial_epoch_authenticator);
    }
  );
});
