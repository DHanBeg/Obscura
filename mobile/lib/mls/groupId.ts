// L2 Tuğla 5b-2 Parça A — grup kurarken client'ın ürettiği MLS group_id.
//
// URL-SAFE base64 ZORUNLU, standart base64 DEĞİL: group_id backend'de bir
// URL path segmenti olarak taşınıyor (POST /v1/mls/group/{id}/add) —
// standart base64'ün '/' karakteri path'i kırar (bkz.
// mls_relay_golden_test.go:175, aynı kanıt). '+' de query-string'de farklı
// yorumlanabildiği için o da dışlandı, '=' padding'i URL'de gereksiz.
import "react-native-get-random-values";

const GROUP_ID_BYTES = 32;

function base64UrlEncode(bytes: Uint8Array): string {
  const std = Buffer.from(bytes).toString("base64");
  return std.replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export interface GeneratedGroupId {
  /** ts-mls createGroup/createGroupWithMember'ın beklediği ham byte'lar. */
  bytes: Uint8Array;
  /** Backend'e (mls_group_id, URL path segmenti) ve mlsApi.* çağrılarına giden kimlik. */
  b64: string;
}

/** Rastgele, benzersiz bir MLS group_id üretir (32 byte, kriptografik RNG). */
export function generateGroupId(): GeneratedGroupId {
  const bytes = new Uint8Array(GROUP_ID_BYTES);
  crypto.getRandomValues(bytes);
  return { bytes, b64: base64UrlEncode(bytes) };
}
