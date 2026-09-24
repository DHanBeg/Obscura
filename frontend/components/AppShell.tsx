"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { api, createWS, AuthError } from "@/lib/api";
import { useStore } from "@/lib/store";
import { loadIdentity } from "@/lib/e2ee";
import { ensurePreKeysUploaded } from "@/lib/prekeys-sync";
import { decryptIncoming, setActiveAccountDid } from "@/lib/e2ee-session";
import { writePreview } from "@/lib/preview-cache";
import { sentAtToIso } from "@/lib/sent-at";
import { notifyIncomingMessage } from "@/lib/tab-notify";
import { getToken, onTauriEvent, requestWebPushPermission } from "@/lib/tauri";
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

// Web push kaydı. İzin zaten verilmişse kayıt sessizce yenilenir; izin henüz sorulmadıysa
// pencere AÇILIŞTA değil, kullanıcının İLK etkileşiminde (tıklama/tuş) açılır. Hiçbir
// durumda bootstrap'ı bloklamaz (await'siz) ve hata fırlatmaz.
let pushRegistrationScheduled = false;
function schedulePushRegistration(): void {
  if (pushRegistrationScheduled || typeof window === "undefined") return;
  pushRegistrationScheduled = true;

  const register = (interactive: boolean) => {
    requestWebPushPermission(interactive)
      .then((pushToken) => (pushToken ? api.registerDevice?.("fcm", pushToken) : undefined))
      .catch(() => {});
  };

  register(false);
  if (!("Notification" in window) || Notification.permission !== "default") return;
  const onFirstInteraction = () => {
    window.removeEventListener("pointerdown", onFirstInteraction);
    window.removeEventListener("keydown", onFirstInteraction);
    register(true);
  };
  window.addEventListener("pointerdown", onFirstInteraction);
  window.addEventListener("keydown", onFirstInteraction);
}
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
            // WS sent_at Unix SANİYESİ (sayı); store/REST ISO string bekler → normalize et
            addMessage({ ...p, sent_at: sentAtToIso(p.sent_at), ciphertext: plaintext });
            // Sohbet listesi önizlemesi (yerel önbellek): yalnız çözülmüş metin mesajı.
            if (p.type === "text") writePreview(s.user?.did, p.conv_id, { text: plaintext, msgId: p.id });
            // Sekme-içi bildirim (başlık sayacı, favicon rozeti, ses) + izin verilmişse OS
            // bildirimi. Push/service worker akışından ve Notification izninden BAĞIMSIZ;
            // yalnız sekme arka plandayken (gizli ya da odakta değil) çalışır. Hata fırlatmaz.
            notifyIncomingMessage({
              id: p.id,
              type: p.type,
              fromDid: p.from_did,
              ownDid: s.user?.did,
              plaintext,
            });
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

    // Push kaydı: WS AÇILDIKTAN SONRA ve await'siz — bootstrap'ı asla bloklamaz.
    schedulePushRegistration();
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
