"use client";

// ZK QR format: obscura://zk/{commitment}/{public_params_b64}
// Spec Bölüm 11.3 — kimlik açıklanmadan ZK proof paylaşımı

export interface ZKQRPayload {
  commitment: string;
  publicParams: Record<string, string | number>;
  circuitId: string;
  timestamp: number;
}

export function encodeZKQR(payload: ZKQRPayload): string {
  const paramsB64 = btoa(JSON.stringify({
    circuit: payload.circuitId,
    params: payload.publicParams,
    ts: payload.timestamp,
  }));
  return `obscura://zk/${payload.commitment}/${paramsB64}`;
}

export function decodeZKQR(url: string): ZKQRPayload | null {
  try {
    if (!url.startsWith("obscura://zk/")) return null;
    const rest = url.slice("obscura://zk/".length);
    const slashIdx = rest.indexOf("/");
    if (slashIdx === -1) return null;
    const commitment = rest.slice(0, slashIdx);
    const paramsRaw = JSON.parse(atob(rest.slice(slashIdx + 1)));
    return {
      commitment,
      circuitId: paramsRaw.circuit ?? "unknown",
      publicParams: paramsRaw.params ?? {},
      timestamp: paramsRaw.ts ?? 0,
    };
  } catch {
    return null;
  }
}

// Profile sharing QR: obscura://add/{did}/{pubkey_b64}
export function encodeProfileQR(did: string, pubkeyB64: string): string {
  return `obscura://add/${did}/${pubkeyB64}`;
}

// Device pairing QR: obscura://device/{did}/{device_id}/{challenge_b64}
export function encodeDeviceQR(did: string, deviceId: string, challenge: Uint8Array): string {
  const challengeB64 = btoa(Array.from(challenge, c => String.fromCharCode(c)).join(""));
  return `obscura://device/${did}/${deviceId}/${challengeB64}`;
}

export function decodeObscuraQR(url: string): { type: string; payload: unknown } | null {
  if (url.startsWith("obscura://zk/")) {
    const p = decodeZKQR(url);
    return p ? { type: "zk", payload: p } : null;
  }
  if (url.startsWith("obscura://add/")) {
    const parts = url.slice("obscura://add/".length).split("/");
    if (parts.length < 2) return null;
    return { type: "profile", payload: { did: parts[0], pubkey: parts[1] } };
  }
  if (url.startsWith("obscura://device/")) {
    const parts = url.slice("obscura://device/".length).split("/");
    if (parts.length < 3) return null;
    return { type: "device", payload: { did: parts[0], deviceId: parts[1], challenge: parts[2] } };
  }
  return null;
}
