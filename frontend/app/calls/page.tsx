"use client";

import { useState, useEffect, useRef } from "react";
import { useSearchParams } from "next/navigation";
import { Phone, PhoneOff, Video, Mic, MicOff, VideoOff, Volume2 } from "lucide-react";
import { cn } from "@/lib/cn";
import { AppShell } from "@/components/AppShell";
import { Avatar } from "@/components/ui/Avatar";
import { useStore } from "@/lib/store";
import { api, apiFetch } from "@/lib/api";

type CallState = "idle" | "outgoing" | "incoming" | "connected";

export default function CallsPage() {
  const searchParams = useSearchParams();
  const peerDID = searchParams.get("peer");
  const { conversations, onlineUsers } = useStore();
  const [callState, setCallState] = useState<CallState>("idle");
  const [muted, setMuted] = useState(false);
  const [videoOff, setVideoOff] = useState(false);
  const [speaker, setSpeaker] = useState(true);
  const [duration, setDuration] = useState(0);
  const durationRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const [turnCreds, setTurnCreds] = useState<{ username: string; password: string } | null>(null);

  const peer = peerDID
    ? conversations.find((c) => c.peer_did === peerDID)
    : null;
  const peerName = peer?.peer_name || peer?.name || "Bilinmiyor";

  // Get TURN credentials
  useEffect(() => {
    apiFetch("/v1/webrtc/turn-credentials")
      .then((d: any) => setTurnCreds(d))
      .catch(() => {});
  }, []);

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
    setCallState("idle");
    stopDuration();
  };

  const answer = () => {
    setCallState("connected");
    startDuration();
  };

  // Idle state — show recent calls placeholder
  if (callState === "idle") {
    return (
      <AppShell>
        <div className="flex flex-col h-full">
          <div className="px-5 pt-5 pb-3 flex-shrink-0">
            <h1 className="text-2xl font-semibold text-head">Aramalar</h1>
            <p className="text-sm text-sub mt-0.5">Uçtan uca şifreli ses ve görüntü</p>
          </div>

          <div className="flex-1 flex flex-col items-center justify-center pb-32 text-center px-6">
            <div className="w-20 h-20 rounded-full bg-raised border border-border flex items-center justify-center mb-4 animate-pulse-soft">
              <Phone size={32} className="text-dim" />
            </div>
            <p className="text-body font-medium mb-1">Arama geçmişi yok</p>
            <p className="text-dim text-sm max-w-[220px]">
              Sohbet ekranından bir kişiyi arayabilirsiniz
            </p>
          </div>
        </div>
      </AppShell>
    );
  }

  // Active call UI
  return (
    <div className="fixed inset-0 void-bg flex flex-col items-center justify-between pt-16 pb-12 pb-safe">
      {/* Ambient */}
      <div className="absolute inset-0 pointer-events-none">
        <div className={cn(
          "absolute top-1/4 left-1/2 -translate-x-1/2 -translate-y-1/2",
          "w-64 h-64 rounded-full blur-3xl transition-all duration-1000",
          callState === "connected" ? "bg-accent/8 scale-110" : "bg-accent/4"
        )} />
      </div>

      {/* Call info */}
      <div className="flex flex-col items-center gap-4 z-10 animate-slide-up">
        <Avatar name={peerName} size="xl" />
        <div className="text-center">
          <h2 className="text-2xl font-semibold text-head">{peerName}</h2>
          <p className={cn("text-sm mt-1", callState === "connected" ? "text-accent" : "text-sub")}>
            {callState === "outgoing" && "Bekleniyor..."}
            {callState === "incoming" && "Arıyor..."}
            {callState === "connected" && formatDuration(duration)}
          </p>
        </div>

        {/* Outgoing pulse */}
        {callState === "outgoing" && (
          <div className="relative">
            <div className="absolute inset-0 rounded-full bg-accent/20 animate-ping" style={{ animationDuration: "2s" }} />
          </div>
        )}
      </div>

      {/* Controls */}
      <div className="flex flex-col items-center gap-6 z-10">
        {/* Call controls */}
        <div className="flex items-center gap-5">
          <button
            onClick={() => setMuted((v) => !v)}
            className={cn(
              "w-14 h-14 rounded-full flex items-center justify-center",
              "transition-all duration-200 border",
              muted
                ? "bg-red/15 border-red/30 text-red"
                : "bg-raised border-border text-sub hover:text-body"
            )}
          >
            {muted ? <MicOff size={22} /> : <Mic size={22} />}
          </button>

          <button
            onClick={() => setSpeaker((v) => !v)}
            className={cn(
              "w-14 h-14 rounded-full flex items-center justify-center border",
              "transition-all duration-200",
              speaker
                ? "bg-accent/10 border-accent/25 text-accent"
                : "bg-raised border-border text-sub hover:text-body"
            )}
          >
            <Volume2 size={22} />
          </button>

          <button
            onClick={() => setVideoOff((v) => !v)}
            className={cn(
              "w-14 h-14 rounded-full flex items-center justify-center border",
              "transition-all duration-200",
              videoOff
                ? "bg-red/15 border-red/30 text-red"
                : "bg-raised border-border text-sub hover:text-body"
            )}
          >
            {videoOff ? <VideoOff size={22} /> : <Video size={22} />}
          </button>
        </div>

        {/* Hang up + Answer */}
        <div className="flex items-center gap-8">
          <button
            onClick={hangup}
            className="w-16 h-16 rounded-full bg-red flex items-center justify-center text-white shadow-void-md hover:bg-red/90 active:scale-95 transition-all duration-200"
          >
            <PhoneOff size={24} />
          </button>

          {callState === "incoming" && (
            <button
              onClick={answer}
              className="w-16 h-16 rounded-full bg-accent flex items-center justify-center text-void shadow-accent-glow hover:bg-accent/90 active:scale-95 transition-all duration-200"
            >
              <Phone size={24} />
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
