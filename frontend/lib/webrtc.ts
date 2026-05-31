"use client";

// WebRTC peer connection manager for Obscura voice/video calls.
// Uses TURN credentials from /v1/rtc/turn-credentials for relay fallback.
// Signaling (offer/answer/ICE) is exchanged via the WebSocket stream.

export interface TurnCredentials {
  username: string;
  password: string;
  url?: string;
}

export type CallEventType =
  | "connected"
  | "disconnected"
  | "failed"
  | "candidate"
  | "offer"
  | "answer"
  | "track";

export interface CallEvent {
  type: CallEventType;
  data?: unknown;
}

export type CallEventHandler = (event: CallEvent) => void;

const TURN_HOST =
  process.env.NEXT_PUBLIC_TURN_HOST || "turn.obscura.network";
const STUN_URL = `stun:${TURN_HOST}:3478`;
const TURN_URL_UDP = `turn:${TURN_HOST}:3478`;
const TURN_URL_TLS = `turns:${TURN_HOST}:5349`;

export class ObscuraCall {
  private pc: RTCPeerConnection | null = null;
  private localStream: MediaStream | null = null;
  private handlers: CallEventHandler[] = [];
  private polite: boolean; // polite peer defers on offer collision (RFC 8829)
  private makingOffer = false;
  private ignoreOffer = false;

  constructor(polite: boolean) {
    this.polite = polite;
  }

  // ── Lifecycle ────────────────────────────────────────────────────────────────

  async init(creds: TurnCredentials | null): Promise<void> {
    const iceServers: RTCIceServer[] = [
      { urls: STUN_URL },
    ];

    if (creds) {
      iceServers.push({
        urls: [TURN_URL_UDP, TURN_URL_TLS],
        username: creds.username,
        credential: creds.password,
      });
    }

    this.pc = new RTCPeerConnection({
      iceServers,
      bundlePolicy: "max-bundle",
      rtcpMuxPolicy: "require",
      iceCandidatePoolSize: 10,
    });

    this.pc.onicecandidate = ({ candidate }) => {
      if (candidate) {
        this.emit({ type: "candidate", data: candidate.toJSON() });
      }
    };

    this.pc.oniceconnectionstatechange = () => {
      const state = this.pc?.iceConnectionState;
      if (state === "connected" || state === "completed") {
        this.emit({ type: "connected" });
      } else if (state === "disconnected" || state === "closed") {
        this.emit({ type: "disconnected" });
      } else if (state === "failed") {
        this.pc?.restartIce();
        this.emit({ type: "failed" });
      }
    };

    this.pc.onnegotiationneeded = async () => {
      try {
        this.makingOffer = true;
        await this.pc!.setLocalDescription();
        this.emit({ type: "offer", data: this.pc!.localDescription });
      } catch (err) {
        console.error("[webrtc] negotiationneeded error", err);
      } finally {
        this.makingOffer = false;
      }
    };

    this.pc.ontrack = ({ track, streams }) => {
      this.emit({ type: "track", data: { track, stream: streams[0] } });
    };
  }

  async startLocalMedia(audio = true, video = false): Promise<MediaStream> {
    this.localStream = await navigator.mediaDevices.getUserMedia({ audio, video });
    for (const track of this.localStream.getTracks()) {
      this.pc!.addTrack(track, this.localStream);
    }
    return this.localStream;
  }

  stopLocalMedia(): void {
    this.localStream?.getTracks().forEach((t) => t.stop());
    this.localStream = null;
  }

  close(): void {
    this.stopLocalMedia();
    this.pc?.close();
    this.pc = null;
    this.handlers = [];
  }

  // ── Signaling handlers (called by WebSocket message dispatcher) ──────────────

  // handleOffer — received remote offer; create and send back answer.
  async handleOffer(description: RTCSessionDescriptionInit): Promise<void> {
    if (!this.pc) return;
    const offerCollision =
      description.type === "offer" &&
      (this.makingOffer || this.pc.signalingState !== "stable");

    this.ignoreOffer = !this.polite && offerCollision;
    if (this.ignoreOffer) return;

    await this.pc.setRemoteDescription(description);
    if (description.type === "offer") {
      await this.pc.setLocalDescription();
      this.emit({ type: "answer", data: this.pc.localDescription });
    }
  }

  // handleAnswer — received remote answer to our offer.
  async handleAnswer(description: RTCSessionDescriptionInit): Promise<void> {
    if (!this.pc) return;
    if (this.pc.signalingState === "have-local-offer") {
      await this.pc.setRemoteDescription(description);
    }
  }

  // handleCandidate — trickle ICE candidate from remote peer.
  async handleCandidate(candidateInit: RTCIceCandidateInit): Promise<void> {
    if (!this.pc) return;
    try {
      if (!this.ignoreOffer) {
        await this.pc.addIceCandidate(new RTCIceCandidate(candidateInit));
      }
    } catch (err) {
      console.warn("[webrtc] addIceCandidate error", err);
    }
  }

  // ── Controls ─────────────────────────────────────────────────────────────────

  setMuted(muted: boolean): void {
    this.localStream?.getAudioTracks().forEach((t) => {
      t.enabled = !muted;
    });
  }

  setVideoEnabled(enabled: boolean): void {
    this.localStream?.getVideoTracks().forEach((t) => {
      t.enabled = enabled;
    });
  }

  getConnectionState(): RTCPeerConnectionState | null {
    return this.pc?.connectionState ?? null;
  }

  // ── Event emitter ─────────────────────────────────────────────────────────────

  on(handler: CallEventHandler): () => void {
    this.handlers.push(handler);
    return () => {
      this.handlers = this.handlers.filter((h) => h !== handler);
    };
  }

  private emit(event: CallEvent): void {
    this.handlers.forEach((h) => h(event));
  }
}
