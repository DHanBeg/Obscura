"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { api, createWS } from "@/lib/api";
import { useStore } from "@/lib/store";
import { loadIdentity } from "@/lib/e2ee";
import { getToken, onTauriEvent, showNotification, requestWebPushPermission } from "@/lib/tauri";
import { api } from "@/lib/api";
import { GravityWell } from "./GravityWell";
import { NewChatSheet } from "./NewChatSheet";

interface AppShellProps {
  children: React.ReactNode;
  showBack?: boolean;
  title?: string;
  hideGravityWell?: boolean;
}

export function AppShell({ children, showBack, title, hideGravityWell }: AppShellProps) {
  const router = useRouter();
  const { setUser, setConversations, addMessage, updateMsgStatus, setOnline, setWS, setIdentity } = useStore();
  const [newChatOpen, setNewChatOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const bootstrapped = useRef(false);

  const bootstrap = useCallback(async () => {
    if (bootstrapped.current) return;
    bootstrapped.current = true;
    const token = await getToken();
    if (!token) { router.replace("/login"); return; }

    try {
      const [me, convs] = await Promise.all([api.getMe(), api.getConversations()]);
      setUser(me);
      setConversations(convs || []);

      // E2EE: Kayıtlı kimliği yükle
      try {
        const phone = me?.phone || me?.username || "unknown";
        const passphrase = `obscura_${phone}_v1`;
        const identity = await loadIdentity(passphrase);
        if (identity) setIdentity(identity);
      } catch {}

      // Push bildirim izni iste ve token kaydet (web)
      try {
        const pushToken = await requestWebPushPermission();
        if (pushToken) {
          await api.registerDevice?.("fcm", pushToken);
        }
      } catch {}
    } catch {
      localStorage.removeItem("obscura_token");
      router.replace("/login");
      return;
    }

    // WebSocket
    const token2 = (await getToken())!;
    const ws = createWS(token2, (msg) => {
      switch (msg.type) {
        case "new_message":
          addMessage(msg.data);
          // Native bildirim — uygulama arka plandaysa göster
          if (typeof document !== "undefined" && document.hidden) {
            showNotification("Yeni mesaj", msg.data.ciphertext?.slice(0, 60) ?? "").catch(() => {});
          }
          break;
        case "message_delivered":
        case "message_read":
          updateMsgStatus(msg.data.msg_id, msg.type === "message_read" ? "read" : "delivered");
          break;
        case "user_online":
          setOnline(msg.data.did, true);
          break;
        case "user_offline":
          setOnline(msg.data.did, false);
          break;
      }
    });
    wsRef.current = ws;
    setWS(ws);
  }, [router, setUser, setConversations, addMessage, updateMsgStatus, setOnline, setWS]);

  useEffect(() => {
    bootstrap();
    return () => {
      wsRef.current?.close();
    };
  }, [bootstrap]);

  return (
    <div className="fixed inset-0 flex flex-col void-bg">
      {/* Page content */}
      <main className="flex-1 overflow-hidden relative">
        {children}
      </main>

      {/* Navigation */}
      {!hideGravityWell && (
        <GravityWell
          showBack={showBack}
          title={title}
          onNewChat={() => setNewChatOpen(true)}
          onSearch={() => setSearchOpen(true)}
          onBack={() => router.back()}
        />
      )}

      {/* New Chat Sheet */}
      <NewChatSheet
        open={newChatOpen}
        onClose={() => setNewChatOpen(false)}
      />
    </div>
  );
}
