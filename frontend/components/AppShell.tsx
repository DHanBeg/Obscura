"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { api, createWS, AuthError } from "@/lib/api";
import { useStore } from "@/lib/store";
import { loadIdentity } from "@/lib/e2ee";
import { ensurePreKeysUploaded } from "@/lib/prekeys";
import { decryptIncoming, setActiveAccountDid } from "@/lib/e2ee-session";
import { getToken, onTauriEvent, showNotification, requestWebPushPermission } from "@/lib/tauri";
import { GravityWell } from "./GravityWell";
import { NewChatSheet } from "./NewChatSheet";

interface AppShellProps {
  children: React.ReactNode;
  showBack?: boolean;
  title?: string;
  hideGravityWell?: boolean;
}

// Double ratchet sirali ilerler -- ayni conv icin art arda gelen WS
// "new_message" olaylari birbirini beklemeden decryptIncoming baslatirsa
// (ikisi de ayni eski state'i okuyup celisen state yazar) chain key
// bozulur ve ikinci mesaj hep "cozulemedi" doner. Conv basina zincirleme
// kuyruk bunu engeller.
const decryptQueues: Record<string, Promise<unknown>> = {};
function queueDecrypt<T>(convId: string, fn: () => Promise<T>): Promise<T> {
  const prev = decryptQueues[convId] || Promise.resolve();
  const next = prev.then(fn, fn);
  decryptQueues[convId] = next.catch(() => {});
  return next;
}

export function AppShell({ children, showBack, title, hideGravityWell }: AppShellProps) {
  const router = useRouter();
  const { user: storeUser, ws: storeWS, setUser, setConversations, addMessage, updateMsgStatus, setOnline, setWS, setIdentity, setPrekeyStore } = useStore();
  const [newChatOpen, setNewChatOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const bootstrapped = useRef(false);

  const bootstrap = useCallback(async () => {
    if (bootstrapped.current) return;
    bootstrapped.current = true;

    // Kullanıcı ve WS zaten aktifse tekrar bootstrap etme (settings sub-page navigation)
    if (storeUser && storeWS && storeWS.readyState === WebSocket.OPEN) return;

    const token = await getToken();
    if (!token) { router.replace("/login"); return; }

    try {
      const [me, convs] = await Promise.all([api.getMe(), api.getConversations()]);
      setUser(me);
      setConversations(convs || []);

      // B10 Faz 1 — mobile/app/_layout.tsx:ensureInvitable ile AYNI ilke:
      // kendi MLS KeyPackage'ını üretir/yükler ki başkaları (mobil) bizi bir
      // gruba EKLEYEBİLSİN (web kendi grup kuramaz, ama davet edilebilir
      // olması gerekir). Dynamic import — ts-mls'i (46kB) her sayfanın
      // paylaşılan bundle'ına sokmamak için (AppShell tüm sayfaları sarar).
      import("@/lib/mls/inviteBootstrap")
        .then((m) => m.ensureInvitable(me.did))
        .catch(() => {});

      // E2EE: Kayıtlı kimliği yükle
      try {
        // DID kullanilir -- me.phone /v1/users/me yanitinda genelde yok
        // (gizlilik), telefon tabanli passphrase reload sonrasi hep
        // uyusmuyordu (login/page.tsx kayitta ham phone kullaniyor, burada
        // phone yoksa username'e duserdi -- iki farkli passphrase, hep fail).
        const passphrase = `obscura_${me.did}_v1`;
        const identity = await loadIdentity(passphrase);
        if (identity) {
          setIdentity(identity);
          setActiveAccountDid(identity.did);
          const prekeyStore = await ensurePreKeysUploaded(identity, passphrase);
          if (prekeyStore) setPrekeyStore(prekeyStore);
        }
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
        case "new_message": {
          const s = useStore.getState();
          queueDecrypt(p.conv_id, () => decryptIncoming(p.conv_id, p.ciphertext, s.identity, s.prekeyStore, s.setRatchet)).then((plaintext) => {
            addMessage({ ...p, ciphertext: plaintext });
            // Native bildirim — uygulama arka plandaysa göster
            if (typeof document !== "undefined" && document.hidden) {
              showNotification("Yeni mesaj", plaintext.slice(0, 60)).catch(() => {});
            }
          });
          break;
        }
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
  }, [router, storeUser, storeWS, setUser, setConversations, addMessage, updateMsgStatus, setOnline, setWS]);

  useEffect(() => {
    bootstrap();
    // WS intentionally NOT closed on unmount — navigation between pages
    // would kill the connection every time. WS lifecycle is managed by logout.
  }, [bootstrap]);

  return (
    <div className="fixed inset-0 void-bg flex justify-center">
      {/* Center content on desktop — max 480px wide */}
      <div className="flex flex-col h-full w-full" style={{ maxWidth: 480 }}>
        {/* Page content */}
        <main className="flex-1 overflow-hidden relative scroll-area" style={{ background: "var(--bg)" }}>
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
