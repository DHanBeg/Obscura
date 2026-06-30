/**
 * Module-level singleton — RTCPeerConnection persists across screen transitions.
 * Both caller and callee screens read/write this state.
 */
import {
  RTCPeerConnection,
  RTCSessionDescription,
  mediaDevices,
} from "react-native-webrtc";

export type RtcState = "idle" | "calling" | "ringing" | "connected" | "ended";

const STUN_ONLY = [
  { urls: "stun:stun.l.google.com:19302" },
  { urls: "stun:stun1.l.google.com:19302" },
  { urls: "stun:stun2.l.google.com:19302" },
];

let _iceServers: any[] = STUN_ONLY;

export function setTurnCredentials(creds: { urls: string[]; username: string; credential: string }) {
  _iceServers = [
    ...STUN_ONLY,
    { urls: creds.urls, username: creds.username, credential: creds.credential },
  ];
}

let _pc: any = null;
let _localStream: any = null;
let _remoteStream: any = null;
let _state: RtcState = "idle";
let _isMuted = false;
let _isCameraOff = false;

const _listeners = new Set<() => void>();

function notify() {
  _listeners.forEach((fn) => fn());
}

export function subscribe(fn: () => void): () => void {
  _listeners.add(fn);
  return () => _listeners.delete(fn);
}

export function getState() {
  return {
    state: _state,
    localStream: _localStream,
    remoteStream: _remoteStream,
    isMuted: _isMuted,
    isCameraOff: _isCameraOff,
  };
}

async function acquireMedia(callType: "audio" | "video") {
  const stream = await mediaDevices.getUserMedia({
    audio: true,
    video: callType === "video"
      ? { facingMode: "user", width: 640, height: 480 }
      : false,
  });
  _localStream = stream;
  notify();
  return stream;
}

function createPC() {
  cleanup();
  _pc = new RTCPeerConnection({ iceServers: _iceServers });

  _pc.ontrack = (e: any) => {
    const streams = e.streams;
    if (streams?.[0]) {
      _remoteStream = streams[0];
      notify();
    }
  };

  _pc.oniceconnectionstatechange = () => {
    const s = _pc?.iceConnectionState;
    if (s === "connected" || s === "completed") {
      _state = "connected";
      notify();
    } else if (s === "disconnected" || s === "failed" || s === "closed") {
      _state = "ended";
      notify();
    }
  };

  return _pc;
}

function waitForIce(connection: any): Promise<void> {
  return new Promise((resolve) => {
    if (connection.iceGatheringState === "complete") { resolve(); return; }
    const timeout = setTimeout(resolve, 4000);
    const prev = connection.onicecandidate;
    connection.onicecandidate = (e: any) => {
      prev?.(e);
      if (!e.candidate) { clearTimeout(timeout); resolve(); }
    };
  });
}

/** Caller: get local media + create offer SDP */
export async function initCaller(callType: "audio" | "video"): Promise<string> {
  _state = "calling";
  notify();

  const connection = createPC(); // cleanup old session first
  const stream = await acquireMedia(callType);
  (stream as any).getTracks().forEach((t: any) => connection.addTrack(t, stream));

  const offer = await connection.createOffer({});
  await connection.setLocalDescription(offer);
  await waitForIce(connection);

  return connection.localDescription?.sdp ?? "";
}

/** Callee: create answer SDP from incoming offer */
export async function initCallee(sdpOffer: string, callType: "audio" | "video"): Promise<string> {
  _state = "ringing";
  notify();

  const connection = createPC(); // cleanup old session first
  const stream = await acquireMedia(callType);
  (stream as any).getTracks().forEach((t: any) => connection.addTrack(t, stream));

  await connection.setRemoteDescription(
    new RTCSessionDescription({ type: "offer", sdp: sdpOffer })
  );
  const answer = await connection.createAnswer();
  await connection.setLocalDescription(answer);
  await waitForIce(connection);

  _state = "connected";
  notify();
  return connection.localDescription?.sdp ?? "";
}

/** Caller: apply the peer's answer */
export async function applyAnswer(sdpAnswer: string): Promise<void> {
  if (!_pc) return;
  await _pc.setRemoteDescription(
    new RTCSessionDescription({ type: "answer", sdp: sdpAnswer })
  );
  _state = "connected";
  notify();
}

export function toggleMute(): boolean {
  const audio = _localStream?.getAudioTracks()?.[0];
  if (!audio) return _isMuted;
  audio.enabled = !audio.enabled;
  _isMuted = !audio.enabled;
  notify();
  return _isMuted;
}

export function toggleCamera(): boolean {
  const video = _localStream?.getVideoTracks()?.[0];
  if (!video) return _isCameraOff;
  video.enabled = !video.enabled;
  _isCameraOff = !video.enabled;
  notify();
  return _isCameraOff;
}

export function cleanup() {
  _localStream?.getTracks()?.forEach((t: any) => t.stop());
  _remoteStream?.getTracks()?.forEach((t: any) => t.stop());
  _pc?.close();
  _pc = null;
  _localStream = null;
  _remoteStream = null;
  _isMuted = false;
  _isCameraOff = false;
}

export function markEnded() {
  cleanup();
  _state = "ended";
  notify();
  setTimeout(() => { _state = "idle"; notify(); }, 1000);
}
