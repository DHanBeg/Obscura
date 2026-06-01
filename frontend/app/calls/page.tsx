"use client";

import { useState, useEffect, useRef, Suspense } from "react";
import { useSearchParams } from "next/navigation";
import {
  Phone, PhoneOff, PhoneOutgoing, PhoneMissed,
  Video, Mic, MicOff, VideoOff, Volume2,
  Clock,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { AppShell } from "@/components/AppShell";
import { Avatar } from "@/components/ui/Avatar";
import { StatusPill } from "@/components/StatusPill";
import { useStore } from "@/lib/store";
import { api, apiFetch } from "@/lib/api";
import { ObscuraCall, type TurnCredentials } from "@/lib/webrtc";

type CallState = "idle" | "outgoing" | "incoming" | "connected";

// ── Mock recent calls type ────────────────────────────────────────────────────
interface RecentCall {
  id: string;
  name: string;
  date: string;
  direction: "incoming" | "outgoing" | "missed";
  transport: "p2p" | "turn";
  duration?: number;
}

// ── Recent call row ───────────────────────────────────────────────────────────
function CallRow({ call }: { call: RecentCall }) {
  const dirIcon = (() => {
    if (call.direction === "outgoing")
      return <PhoneOutgoing size={14} style={{ color: "var(--accent)" }} />;
    if (call.direction === "missed")
      return <PhoneMissed size={14} style={{ color: "var(--error)" }} />;
    return <Phone size={14} style={{ color: "var(--text-2)" }} />;
  })();

  return (
    <div
      className="conv-row"
      role="listitem"
    >
      <Avatar name={call.name} size="md" />

      <div className="flex-1 min-w-0">
        <div className="flex items-baseline justify-between mb-0.5">
          <span
            className="text-[15px] font-medium truncate"
            style={{
              fontFamily: "var(--font-display)",
              color: call.direction === "missed" ? "var(--error)" : "var(--text-1)",
            }}
          >
            {call.name}
          </span>
          <span className="text-[11px] flex-shrink-0 ml-2" style={{ color: "var(--text-3)" }}>
            {call.date}
          </span>
        </div>

        <div className="flex items-center gap-2">
          <span className="flex items-center gap-1" style={{ color: "var(--text-3)" }}>
            {dirIcon}
            <span className="text-[12px]">
              {call.direction === "incoming" && "Gelen"}
              {call.direction === "outgoing" && "Giden"}
              {call.direction === "missed" && "Cevapsız"}
            </span>
          </span>
          {call.duration !== undefined && (
            <span className="flex items-center gap-1 text-[11px]" style={{ color: "var(--text-3)" }}>
              <Clock size={10} />
              {Math.floor(call.duration / 60)}:{String(call.duration % 60).padStart(2, "0")}
            </span>
          )}
        </div>
      </div>

      {/* Call action buttons */}
      <div className="flex items-center gap-1.5 flex-shrink-0">
        <button
          className="w-9 h-9 rounded-full flex items-center justify-center transition-all duration-150 hover:bg-[rgba(0,229,160,0.12)] active:scale-90"
          style={{ color: "var(--accent)" }}
          aria-label={`${call.name} sesli ara`}
        >
          <Phone size={15} strokeWidth={2} />
        </button>
        <button
          className="w-9 h-9 rounded-full flex items-center justify-center transition-all duration-150 hover:bg-[rgba(77,168,255,0.12)] active:scale-90"
          style={{ color: "var(--signal)" }}
          aria-label={`${call.name} görüntülü ara`}
        >
          <Video size={15} strokeWidth={2} />
        </button>
      </div>
    </div>
  );
}

// ── Idle / History Screen ─────────────────────────────────────────────────────
function IdleScreen() {
  // Placeholder recent calls — real data would come from an API
  const recentCalls: RecentCall[] = [
    { id: "1", name: "Ahmet Yılmaz", date: "11:42", direction: "outgoing", transport: "p2p", duration: 183 },
    { id: "2", name: "Zeynep K.", date: "Dün", direction: "missed", transport: "turn" },
    { id: "3", name: "Mert Demir", date: "Dün", direction: "incoming", transport: "p2p", duration: 620 },
    { id: "4", name: "Selin A.", date: "Paz", direction: "outgoing", transport: "p2p", duration: 42 },
  ];

  return (
    <AppShell>
      <div className="flex flex-col h-full">

        {/* ── Page Header ───────────────────────────────────────────────── */}
        <div className="page-header flex-shrink-0 px-4">
          <h1 className="page-title">Aramalar</h1>
          <StatusPill />
        </div>

        <div className="flex-1 scroll-area pb-28">

          {/* ── Security info banner ──────────────────────────────────── */}
          <div
            className="mx-4 mt-4 mb-4 flex items-center gap-3 px-4 py-3 rounded-2xl"
            style={{ background: "rgba(0,229,160,0.04)", border: "1px solid rgba(0,229,160,0.10)" }}
          >
            <Phone size={16} style={{ color: "var(--accent)", flexShrink: 0 }} aria-hidden="true" />
            <p className="text-[12px] leading-relaxed" style={{ color: "var(--text-3)" }}>
              Aramalarınız şifreli ve doğrudan iletilir. Hiçbir kayıt tutulmaz.
            </p>
          </div>

          {/* ── New Call Card ──────────────────────────────────────────── */}
          <div className="px-4 mb-6">
            <div
              className="card-interactive flex items-center gap-4 p-5"
              role="button"
              tabIndex={0}
              aria-label="Yeni sesli arama başlat"
              onKeyDown={(e) => e.key === "Enter" && undefined}
            >
              <div
                className="w-14 h-14 rounded-2xl flex items-center justify-center flex-shrink-0"
                style={{ background: "rgba(0,229,160,0.08)", border: "1px solid rgba(0,229,160,0.14)" }}
              >
                <Phone size={24} style={{ color: "var(--accent)" }} />
              </div>

              <div className="flex-1 min-w-0">
                <p
                  className="text-[16px] font-semibold mb-0.5"
                  style={{ fontFamily: "var(--font-display)", color: "var(--text-1)" }}
                >
                  Yeni Sesli Arama
                </p>
                <p className="text-[13px]" style={{ color: "var(--text-3)" }}>
                  Uçtan uca şifreli · Doğrudan bağlantı
                </p>
              </div>

              <div
                className="w-8 h-8 rounded-full flex items-center justify-center flex-shrink-0"
                style={{ background: "var(--surface-3)", border: "1px solid var(--border-2)" }}
              >
                <PhoneOutgoing size={14} style={{ color: "var(--text-2)" }} />
              </div>
            </div>
          </div>

          {/* ── Section Label ──────────────────────────────────────────── */}
          <div className="px-5 mb-2">
            <span className="section-label">Son Aramalar</span>
          </div>

          {/* ── Recent Calls ───────────────────────────────────────────── */}
          <div
            className="mx-3 overflow-hidden"
            style={{
              background: "var(--surface-2)",
              border: "1px solid var(--border-1)",
              borderRadius: "20px",
            }}
            role="list"
            aria-label="Son aramalar"
          >
            {recentCalls.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-12 text-center px-6">
                <Phone size={28} className="mb-3" style={{ color: "var(--text-3)" }} />
                <p className="text-sm font-medium mb-1" style={{ color: "var(--text-2)" }}>
                  Arama geçmişi yok
                </p>
                <p className="text-xs" style={{ color: "var(--text-3)" }}>
                  Sohbet ekranından bir kişiyi arayabilirsiniz
                </p>
              </div>
            ) : (
              recentCalls.map((call) => (
                <div
                  key={call.id}
                  style={{ borderBottom: "1px solid var(--border-1)" }}
                  className="last:border-0"
                >
                  <CallRow call={call} />
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </AppShell>
  );
}

// ── Main Page ─────────────────────────────────────────────────────────────────
function CallsPageContent() {
  const searchParams = useSearchParams();
  const peerDID = searchParams.get("peer");
  const { conversations, onlineUsers } = useStore();
  const [callState, setCallState] = useState<CallState>("idle");
  const [muted, setMuted] = useState(false);
  const [videoOff, setVideoOff] = useState(false);
  const [speaker, setSpeaker] = useState(true);
  const [duration, setDuration] = useState(0);
  const durationRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const [turnCreds, setTurnCreds] = useState<TurnCredentials | null>(null);
  const callRef = useRef<ObscuraCall | null>(null);
  const remoteAudioRef = useRef<HTMLAudioElement | null>(null);

  const peer = peerDID
    ? conversations.find((c) => c.peer_did === peerDID)
    : null;
  const peerName = peer?.peer_name || peer?.name || "Bilinmiyor";

  // Fetch TURN credentials once on mount
  useEffect(() => {
    apiFetch("/v1/rtc/turn-credentials")
      .then((d: unknown) => setTurnCreds(d as TurnCredentials))
      .catch(() => {});
  }, []);

  // Initialise WebRTC when transitioning to outgoing/incoming
  useEffect(() => {
    if (callState === "idle") return;

    const call = new ObscuraCall(/* polite= */ callState === "incoming");
    callRef.current = call;

    call.init(turnCreds).then(async () => {
      if (callState === "outgoing") {
        await call.startLocalMedia(true, !videoOff);
      }
    });

    const off = call.on((evt) => {
      if (evt.type === "connected") {
        setCallState("connected");
        startDuration();
      } else if (evt.type === "disconnected" || evt.type === "failed") {
        hangup();
      } else if (evt.type === "track") {
        const { stream } = evt.data as { track: MediaStreamTrack; stream: MediaStream };
        if (remoteAudioRef.current) {
          remoteAudioRef.current.srcObject = stream;
          remoteAudioRef.current.play().catch(() => {});
        }
      }
    });

    return () => {
      off();
      call.close();
      callRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [callState === "idle" ? "idle" : "active"]);

  // Sync muted state into the live call
  useEffect(() => { callRef.current?.setMuted(muted); }, [muted]);

  // Sync video state into the live call
  useEffect(() => { callRef.current?.setVideoEnabled(!videoOff); }, [videoOff]);

  // Auto-start call if peerDID provided
  useEffect(() => {
    if (peerDID && callState === "idle") {
      setCallState("outgoing");
    }
  }, [peerDID, callState]);

  const startDuration = () => {
    setDuration(0);
    durationRef.current = setInterval(() => setDuration((d) => d + 1), 1000);
  };

  const stopDuration = () => {
    if (durationRef.current) clearInterval(durationRef.current);
  };

  const formatDuration = (s: number) => {
    const m = Math.floor(s / 60);
    const sec = s % 60;
    return `${m.toString().padStart(2, "0")}:${sec.toString().padStart(2, "0")}`;
  };

  const hangup = () => {
    callRef.current?.close();
    callRef.current = null;
    setCallState("idle");
    stopDuration();
  };

  const answer = async () => {
    if (callRef.current) {
      await callRef.current.startLocalMedia(true, !videoOff);
    }
    setCallState("connected");
    startDuration();
  };

  if (callState === "idle") {
    return <IdleScreen />;
  }

  // ── Active Call UI ──────────────────────────────────────────────────────────
  return (
    <>
    {/* Hidden audio sink for remote MediaStream */}
    <audio ref={remoteAudioRef} autoPlay playsInline style={{ display: "none" }} />
    <div
      className="fixed inset-0 flex flex-col items-center justify-between pt-16 pb-12 void-bg"
      style={{ paddingBottom: "max(48px, env(safe-area-inset-bottom, 12px))" }}
    >
      {/* Ambient glow */}
      <div className="absolute inset-0 pointer-events-none overflow-hidden">
        <div
          className={cn(
            "absolute top-1/4 left-1/2 -translate-x-1/2 -translate-y-1/2 rounded-full blur-3xl",
            "transition-all duration-1000",
            callState === "connected"
              ? "w-72 h-72 opacity-100"
              : "w-56 h-56 opacity-60"
          )}
          style={{ background: "radial-gradient(circle, rgba(0,229,160,0.08) 0%, transparent 70%)" }}
        />
      </div>

      {/* Call info */}
      <div className="flex flex-col items-center gap-5 z-10 animate-slide-up">
        {/* Avatar with pulse ring on outgoing */}
        <div className="relative">
          {callState === "outgoing" && (
            <>
              <div
                className="absolute inset-0 rounded-full animate-ping opacity-20"
                style={{
                  background: "var(--accent)",
                  animationDuration: "1.8s",
                  margin: "-12px",
                }}
              />
              <div
                className="absolute inset-0 rounded-full animate-ping opacity-10"
                style={{
                  background: "var(--accent)",
                  animationDuration: "2.4s",
                  margin: "-24px",
                }}
              />
            </>
          )}
          <Avatar name={peerName} size="xl" />
        </div>

        <div className="text-center">
          <h2
            className="text-[26px] font-bold mb-1"
            style={{ fontFamily: "var(--font-display)", color: "var(--text-1)", letterSpacing: "-0.025em" }}
          >
            {peerName}
          </h2>
          <p
            className={cn("text-sm font-medium transition-colors duration-300")}
            style={{
              color: callState === "connected" ? "var(--accent)" : "var(--text-2)",
            }}
          >
            {callState === "outgoing" && "Bekleniyor..."}
            {callState === "incoming" && "Arıyor..."}
            {callState === "connected" && formatDuration(duration)}
          </p>
          {/* Security badge */}
          {callState === "connected" && (
            <span
              className="inline-flex items-center mt-2 h-5 px-2.5 rounded-full text-[10px] font-semibold animate-fade-in"
              style={{ background: "rgba(0,229,160,0.08)", color: "var(--accent)" }}
            >
              Şifreli arama
            </span>
          )}
        </div>
      </div>

      {/* Controls */}
      <div className="flex flex-col items-center gap-7 z-10">
        {/* Secondary controls */}
        <div className="flex items-center gap-5">
          <button
            onClick={() => setMuted((v) => !v)}
            className="w-14 h-14 rounded-full flex items-center justify-center transition-all duration-200 active:scale-90"
            style={
              muted
                ? {
                    background: "rgba(255,64,88,0.12)",
                    border: "1px solid rgba(255,64,88,0.25)",
                    color: "var(--error)",
                  }
                : {
                    background: "var(--surface-2)",
                    border: "1px solid var(--border-2)",
                    color: "var(--text-2)",
                  }
            }
            aria-label={muted ? "Sessiz kaldır" : "Sessize al"}
            aria-pressed={muted}
          >
            {muted ? <MicOff size={21} /> : <Mic size={21} />}
          </button>

          <button
            onClick={() => setSpeaker((v) => !v)}
            className="w-14 h-14 rounded-full flex items-center justify-center transition-all duration-200 active:scale-90"
            style={
              speaker
                ? {
                    background: "rgba(0,229,160,0.08)",
                    border: "1px solid rgba(0,229,160,0.2)",
                    color: "var(--accent)",
                  }
                : {
                    background: "var(--surface-2)",
                    border: "1px solid var(--border-2)",
                    color: "var(--text-2)",
                  }
            }
            aria-label={speaker ? "Hoparlörü kapat" : "Hoparlörü aç"}
            aria-pressed={speaker}
          >
            <Volume2 size={21} />
          </button>

          <button
            onClick={() => setVideoOff((v) => !v)}
            className="w-14 h-14 rounded-full flex items-center justify-center transition-all duration-200 active:scale-90"
            style={
              videoOff
                ? {
                    background: "rgba(255,64,88,0.12)",
                    border: "1px solid rgba(255,64,88,0.25)",
                    color: "var(--error)",
                  }
                : {
                    background: "var(--surface-2)",
                    border: "1px solid var(--border-2)",
                    color: "var(--text-2)",
                  }
            }
            aria-label={videoOff ? "Kamerayı aç" : "Kamerayı kapat"}
            aria-pressed={videoOff}
          >
            {videoOff ? <VideoOff size={21} /> : <Video size={21} />}
          </button>
        </div>

        {/* Hang up + Answer */}
        <div className="flex items-center gap-10">
          {callState === "incoming" && (
            <button
              onClick={answer}
              className="w-16 h-16 rounded-full flex items-center justify-center transition-all duration-200 active:scale-90"
              style={{
                background: "var(--accent)",
                color: "var(--void)",
                boxShadow: "0 0 28px rgba(0,229,160,0.35), 0 4px 16px rgba(0,0,0,0.5)",
              }}
              aria-label="Aramayı kabul et"
            >
              <Phone size={24} />
            </button>
          )}

          <button
            onClick={hangup}
            className="w-16 h-16 rounded-full flex items-center justify-center transition-all duration-200 active:scale-90"
            style={{
              background: "var(--error)",
              color: "white",
              boxShadow: "0 4px 16px rgba(255,64,88,0.3), 0 2px 8px rgba(0,0,0,0.5)",
            }}
            aria-label="Aramayı sonlandır"
          >
            <PhoneOff size={24} />
          </button>
        </div>
      </div>
    </div>
    </>
  );
}

export default function CallsPage() {
  return (
    <Suspense>
      <CallsPageContent />
    </Suspense>
  );
}
