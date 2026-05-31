"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { api, createWS, AuthError } from "@/lib/api";
import { useStore } from "@/lib/store";
import { loadIdentity } from "@/lib/e2ee";
import { getToken, onTauriEvent, showNotification, requestWebPushPermission } from "@/lib/tauri";
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
    } catch (err) {
      if (err instanceof AuthError) {
        // Token geçersiz veya süresi dolmuş — çıkış yap
        localStorage.removeItem("obscura_token");
        router.replace("/login");
      }
      // Diğer hatalar (ağ hatası, 5xx) — sessizce devam et, kullanıcıyı çıkarma
      return;
    }

    // WebSocket
    const token2 = (await getToken())!;
    const ws = createWS(token2, (msg) => {
      // Backend WSMessage: { type: string, payload: any }
      // Bazı eski handler'lar msg.data kullanıyor (backward compat).
      const p = msg.payload ?? msg.data ?? {};
      switch (msg.type) {
        case "new_message":
          addMessage(p);
          // Native bildirim — uygulama arka plandaysa göster
          if (typeof document !== "undefined" && document.hidden) {
            showNotification("Yeni mesaj", p.ciphertext?.slice(0, 60) ?? "").catch(() => {});
          }
          break;
        // Mesaj durum sistemi (Spec Bölüm 6.4)
        case "delivery_ack":
          // Gönderenin mesajı "delivered" olarak işaretlendi
          updateMsgStatus(p.msg_id, p.status ?? "delivered");
          break;
        case "read_receipt":
          // Alıcı mesajı okudu — gönderene iletildi
          updateMsgStatus(p.msg_id, "read");
          break;
        // Geriye dönük uyumluluk
        case "message_delivered":
          updateMsgStatus(p.msg_id, "delivered");
          break;
        case "message_read":
          updateMsgStatus(p.msg_id, "read");
          break;
        case "user_online":
          setOnline(p.did, true);
          break;
        case "user_offline":
          setOnline(p.did, false);
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
    <div className="fixed inset-0 void-bg flex justify-center">
      {/* Center content on desktop — max 480px wide */}
      <div className="flex flex-col h-full w-full" style={{ maxWidth: 480 }}>
        {/* Page content */}
        <main className="flex-1 overflow-hidden relative scroll-area">
          {children}
        </main>
      </div>

      {/* Navigation — fixed, already centered via justify-center */}
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
