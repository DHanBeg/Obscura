import React, { useEffect, useCallback, useRef, useState, useMemo } from "react";
import {
  View, Text, FlatList, TextInput, TouchableOpacity, Pressable,
  StyleSheet, KeyboardAvoidingView, Platform, StatusBar, Image, Alert, Modal,
  ActivityIndicator, Share,
} from "react-native";
import { useLocalSearchParams, router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import * as Haptics from "expo-haptics";
import * as ImagePicker from "expo-image-picker";
import * as ImageManipulator from "expo-image-manipulator";
import * as Clipboard from "expo-clipboard";
import * as DocumentPicker from "expo-document-picker";
import * as Location from "expo-location";
import { Audio } from "expo-av";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { useStore, type Message } from "@/lib/store";
import { api } from "@/lib/api";
import { Avatar } from "@/components/ui/Avatar";
import { formatFullTime } from "@/lib/format";
import { receiveMessage } from "@/lib/e2e";
import { sendSealedMessage } from "@/lib/message-send";
import { cachePlaintext } from "@/lib/plaintext-cache";
import { SELF_DESTRUCT_OPTIONS, isSelfDestructMessage, formatSelfDestructLabel } from "@/lib/self-destruct";
import { resizeActionFor } from "@/lib/image-resize";
import { shouldFetchConversationHistory, classifyConv } from "@/lib/group-send-gate";
import { sendGroupTextMessage, fetchAndDecryptGroupMessages } from "@/lib/mls/groupChat";
import { encryptFileForUpload, decryptToTempFile, parseMediaKey } from "@/lib/media-crypto";

const GRUP_KAPALI_MESAJI = "Grup mesajlaşma henüz aktif değil.";

// Obscura pençe izi — WhatsApp tik yerine.
// Şekil = teslimat durumu (beklemede/gönderildi: 2 çizgi, iletildi: 3 çizgi,
// görüldü: ∧ pençe), renk = normal (gri/yeşil) veya self-destruct (turuncu).
// selfDestruct artık msg.self_destruct_seconds'a bakan isSelfDestructMessage()
// ile besleniyor (bkz. lib/self-destruct.ts) — backend'in ayrı self_destruct_seconds/
// self_destruct_at alanları (expires_at/30 gün genel TTL'den bağımsız).
function statusLabel(status: Message["status"], selfDestruct?: boolean): string {
  if (status === "expired") return "Süresi doldu";
  const base = status === "read" ? "Görüldü"
    : status === "delivered" ? "İletildi"
    : status === "sent" ? "Gönderildi"
    : "Beklemede";
  return selfDestruct ? `${base}, kendini imha eden mesaj` : base;
}

function StatusIcon({ status, selfDestruct }: { status: Message["status"]; selfDestruct?: boolean }) {
  const a11yProps = {
    accessibilityRole: "image" as const,
    accessibilityLabel: statusLabel(status, selfDestruct),
  };
  if (status === "expired") {
    // Süresi doldu (self-destruct veya 30 gün genel TTL) — pençe deseniyle
    // karışmasın diye ayrı, nötr bir simge (soluk, ikisi de "geçmiş" durum).
    return (
      <View {...a11yProps}>
        <Ionicons name="flame-outline" size={12} color={colors.dim} />
      </View>
    );
  }
  if (status === "read") {
    // Görüldü: ∧ şekli — iki çizgi yukarı birleşiyor
    const c = selfDestruct ? colors.amber : colors.accent;
    return (
      <View {...a11yProps} style={{ flexDirection: "row", gap: 1, alignItems: "center", height: 12 }}>
        <View style={{ width: 2.2, height: 11, backgroundColor: c, borderRadius: 1.1, transform: [{ rotate: "22deg" }] }} />
        <View style={{ width: 2.2, height: 11, backgroundColor: c, borderRadius: 1.1, transform: [{ rotate: "-22deg" }] }} />
      </View>
    );
  }
  if (status === "delivered") {
    // İletildi: \\\ üç çapraz çizgi — yeşil (self-destruct: turuncu)
    const c = selfDestruct ? colors.amber : colors.accent;
    return (
      <View {...a11yProps} style={{ flexDirection: "row", gap: 3.5, alignItems: "center", height: 12 }}>
        <View style={{ width: 2.2, height: 11, backgroundColor: c, borderRadius: 1.1, transform: [{ rotate: "-20deg" }] }} />
        <View style={{ width: 2.2, height: 11, backgroundColor: c, borderRadius: 1.1, transform: [{ rotate: "-20deg" }] }} />
        <View style={{ width: 2.2, height: 11, backgroundColor: c, borderRadius: 1.1, transform: [{ rotate: "-20deg" }] }} />
      </View>
    );
  }
  if (status === "sent") {
    // Gönderildi: \\ iki çapraz çizgi — gri (self-destruct: turuncu)
    const c = selfDestruct ? colors.amber : "rgba(232,232,240,0.45)";
    return (
      <View {...a11yProps} style={{ flexDirection: "row", gap: 3.5, alignItems: "center", height: 12 }}>
        <View style={{ width: 2.2, height: 11, backgroundColor: c, borderRadius: 1.1, transform: [{ rotate: "-20deg" }] }} />
        <View style={{ width: 2.2, height: 11, backgroundColor: c, borderRadius: 1.1, transform: [{ rotate: "-20deg" }] }} />
      </View>
    );
  }
  // Beklemede: aynı 2-çizgi şekli, sent'ten daha soluk (self-destruct: turuncu)
  const c = selfDestruct ? colors.amber : colors.dim;
  return (
    <View {...a11yProps} style={{ flexDirection: "row", gap: 3.5, alignItems: "center", height: 12 }}>
      <View style={{ width: 2.2, height: 11, backgroundColor: c, borderRadius: 1.1, transform: [{ rotate: "-20deg" }] }} />
      <View style={{ width: 2.2, height: 11, backgroundColor: c, borderRadius: 1.1, transform: [{ rotate: "-20deg" }] }} />
    </View>
  );
}

export default function ChatScreen() {
  const { id: convId } = useLocalSearchParams<{ id: string }>();
  const insets = useSafeAreaInsets();
  const {
    conversations, messages, setMessages, addMessage, replaceMessage,
    updateMsgStatus, removeMessage, onlineUsers, user, fontSize,
    wsConnected, mlsMessageNudge,
  } = useStore();

  const [inputVal, setInputVal] = useState("");
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [isTyping, setIsTyping] = useState(false);
  const [decrypted, setDecrypted] = useState<Record<string, string>>({});
  const [replyTo, setReplyTo] = useState<Message | null>(null);
  const [attachOpen, setAttachOpen] = useState(false);
  const [inviteModal, setInviteModal] = useState(false);
  const [inviteSlug, setInviteSlug] = useState("");
  const [inviteMaxUses, setInviteMaxUses] = useState("");
  const [inviteMaxMembers, setInviteMaxMembers] = useState("");
  const [inviteCreating, setInviteCreating] = useState(false);
  const [isRecording, setIsRecording] = useState(false);
  const [recordingSec, setRecordingSec] = useState(0);
  const [playingId, setPlayingId] = useState<string | null>(null);
  const [selfDestructSeconds, setSelfDestructSeconds] = useState<number | null>(null);
  const [timerModalOpen, setTimerModalOpen] = useState(false);

  const flatListRef = useRef<FlatList>(null);
  const sendingRef = useRef(false);
  const recordingRef = useRef<Audio.Recording | null>(null);
  const recordTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const soundRef = useRef<Audio.Sound | null>(null);

  const conv = conversations.find((c) => c.id === convId);
  const peerName = conv?.name || conv?.peer_name || "Sohbet";
  const peerOnline = conv?.peer_did ? onlineUsers.has(conv.peer_did) : false;
  const convMsgs = useMemo(() =>
    (messages[convId] || []).filter((m) => m.ciphertext !== "__init__"),
    [messages, convId]
  );

  useEffect(() => {
    // Fetch convId ile yapılıyor (backend handlers.go:592-599 sadece
    // conv_members üyeliğine bakıyor, peer_did'e ihtiyacı yok) — grup
    // konuşmalarında peer_did asla dolmuyor (handlers.go:533), eski
    // !conv?.peer_did guard'ı grup geçmişini hiç çekmiyordu (sessiz no-op).
    if (!shouldFetchConversationHistory(convId, conv)) return;
    const isGroup = classifyConv(conv) === "group";
    (async () => {
      setLoading(true);
      try {
        if (isGroup) {
          if (!conv?.mls_group_id) { setMessages(convId, []); return; }
          const decMsgs = await fetchAndDecryptGroupMessages(conv.mls_group_id);
          setMessages(convId, decMsgs.map((m): Message => ({
            id: m.id, conv_id: convId, from_did: m.sender_did, to_did: convId,
            type: "text", ciphertext: "", status: "delivered", sent_at: m.created_at,
          })));
          setDecrypted((prev) => {
            const next = { ...prev };
            for (const m of decMsgs) next[m.id] = m.plaintext;
            return next;
          });
        } else {
          // X3DH oturum kurulumu peer'ın PreKey bundle'ını kendisi çekip
          // doğruluyor (lib/e2e.ts) — eski v1 akışın api.getUser tabanlı
          // peerPublicKey state'ine artık gerek yok.
          const msgs = await api.getMessages(convId);
          setMessages(convId, msgs || []);
        }
      } catch {} finally { setLoading(false); }
    })();
  }, [convId, conv?.id]);

  useEffect(() => {
    // Grup mesajları kendi decrypt akışından geçer (fetchAndDecryptGroupMessages
    // / polling effect'i, MLS state ile) — bu effect sadece 1:1 ratchet.
    if (classifyConv(conv) !== "direct") return;
    if (convMsgs.length === 0) return;
    const pending = convMsgs.filter((m) => decrypted[m.id] === undefined);
    if (pending.length === 0) return;
    Promise.all(
      pending.map(async (m) => {
        // senderDid = mesajı gönderen (ratchet oturumu peer DID ile anahtarlı);
        // m.id ile başarılı v2 çözümler önbelleğe alınır — ratchet anahtarları
        // tek kullanımlık olduğundan geçmiş, sonraki açılışlarda oradan okunur.
        const plain = await receiveMessage(m.ciphertext, m.from_did, {}, m.id).catch(() => m.ciphertext);
        return [m.id, plain] as const;
      })
    ).then((results) => {
      setDecrypted((prev) => {
        const next = { ...prev };
        for (const [id, plain] of results) next[id] = plain;
        return next;
      });
    });
  }, [convMsgs, conv?.is_group]);

  useEffect(() => {
    // B6 — 4sn polling kaldırıldı, WS "mls_message" push'una (mls_handlers.go:435)
    // bağlandı. Tetikleyiciler: (1) bu grup için nudge geldi (_layout.tsx'teki
    // "mls_message" case'i, store.mlsMessageNudge), (2) wsConnected false→true
    // geçişi — WS koparsa (createWS 3sn'de reconnect eder, lib/api.ts:373-376)
    // kopukluk penceresinde kaçan mesaj bu tick ile yakalanır (1:1'in
    // "new_message"ı ile aynı desen: WS + reconnect, periyodik fallback yok
    // — durum zaten kalıcı, mls_messages tablosuna WS broadcast'ten ÖNCE
    // yazılıyor, mls_handlers.go:409-416, bu yüzden kaçan event asla veri
    // kaybı değil, sadece "henüz haber verilmedi").
    // Decrypt YOLU AYNI — fetchAndDecryptGroupMessages, E1'in çağırdığı fonksiyon.
    if (classifyConv(conv) !== "group" || !conv?.mls_group_id) return;
    if (!wsConnected) return; // kopukken deneme yok — reconnect olunca bu effect zaten yeniden tetiklenir
    const groupId = conv.mls_group_id;
    if (mlsMessageNudge && mlsMessageNudge.groupId !== groupId) return; // başka bir grubun nudge'ı, bizi ilgilendirmiyor
    let cancelled = false;
    (async () => {
      try {
        const decMsgs = await fetchAndDecryptGroupMessages(groupId);
        if (cancelled) return;
        setDecrypted((prev) => {
          const next = { ...prev };
          for (const m of decMsgs) if (next[m.id] === undefined) next[m.id] = m.plaintext;
          return next;
        });
        const known = new Set((useStore.getState().messages[convId] || []).map((m) => m.id));
        for (const m of decMsgs) {
          if (!known.has(m.id)) {
            addMessage({
              id: m.id, conv_id: convId, from_did: m.sender_did, to_did: convId,
              type: "text", ciphertext: "", status: "delivered", sent_at: m.created_at,
            });
          }
        }
      } catch {}
    })();
    return () => { cancelled = true; };
  }, [conv?.mls_group_id, convId, wsConnected, mlsMessageNudge]);

  // cleanup audio on unmount
  useEffect(() => {
    return () => {
      soundRef.current?.unloadAsync();
      recordingRef.current?.stopAndUnloadAsync();
    };
  }, []);

  // Grup metin gönderimi — 1:1'in optimistic-echo/rollback desenini izler,
  // hedefi conv.mls_group_id (bkz. resolveSendTarget) ve şifrelemeyi
  // encryptGroupMessage/sendGroupTextMessage üzerinden yapar.
  const sendGroupText = useCallback(async () => {
    const text = inputVal.trim();
    if (!text || !conv || !user || sendingRef.current) return;
    const groupId = conv.mls_group_id;
    if (!groupId) { Alert.alert("Hata", "Bu grup MLS'e bağlı değil."); return; }
    const replyId = replyTo?.id ?? undefined;
    sendingRef.current = true;
    setSending(true);
    setInputVal("");
    setReplyTo(null);
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);

    const tempId = `temp-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
    const nowIso = new Date().toISOString();
    addMessage({
      id: tempId, conv_id: convId, from_did: user.did, to_did: convId,
      type: "text", ciphertext: text, status: "pending",
      sent_at: nowIso, reply_to_id: replyId,
    });
    setDecrypted((prev) => ({ ...prev, [tempId]: text }));

    try {
      const sent = await sendGroupTextMessage(groupId, text);
      const realMsg: Message = {
        id: sent.id, conv_id: convId, from_did: user.did, to_did: convId,
        type: "text", ciphertext: "", status: "sent",
        sent_at: sent.created_at || nowIso, reply_to_id: replyId,
      };
      replaceMessage(convId, tempId, realMsg);
      setDecrypted((prev) => {
        const next = { ...prev };
        delete next[tempId];
        next[realMsg.id] = text;
        return next;
      });
      cachePlaintext(realMsg.id, text).catch(() => {});
    } catch (e: any) {
      removeMessage(convId, tempId);
      setDecrypted((prev) => {
        const next = { ...prev };
        delete next[tempId];
        return next;
      });
      setInputVal(text);
      Alert.alert("Hata", e?.message || "Mesaj gönderilemedi.");
    } finally {
      setSending(false);
      sendingRef.current = false;
    }
  }, [inputVal, conv, replyTo, user, convId]);

  const sendMessage = useCallback(async () => {
    const kind = classifyConv(conv);
    if (kind === "unknown") { Alert.alert("Grup", GRUP_KAPALI_MESAJI); return; }
    if (kind === "group") { await sendGroupText(); return; }
    const text = inputVal.trim();
    if (!text || !conv?.peer_did || !user || sendingRef.current) return;
    const replyId = replyTo?.id ?? undefined;
    sendingRef.current = true;
    setSending(true);
    setInputVal("");
    setReplyTo(null);
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);

    // Optimistic local echo — show the bubble instantly instead of waiting on
    // the encrypt→POST→WS round trip. Our own outbound ciphertext is encrypted
    // for the peer's key and can't be decrypted back on this device, so we
    // seed the plaintext directly rather than relying on the decrypt effect.
    const tempId = `temp-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
    const nowIso = new Date().toISOString();
    const sdSeconds = selfDestructSeconds;
    setSelfDestructSeconds(null); // her mesajda tekrar seçilmeli — sessizce yapışık kalmasın
    addMessage({
      id: tempId, conv_id: convId, from_did: user.did, to_did: conv.peer_did,
      type: "text", ciphertext: text, status: "pending",
      sent_at: nowIso, reply_to_id: replyId,
      self_destruct_seconds: sdSeconds,
    });
    setDecrypted((prev) => ({ ...prev, [tempId]: text }));

    try {
      // Server only echoes back { id, conv_id, status } — fill the rest from
      // what we already know locally rather than assuming a full message body.
      const sent = await sendSealedMessage(conv.peer_did, text, "text", {}, {
        ...(replyId ? { reply_to_id: replyId } : {}),
        ...(sdSeconds !== null ? { self_destruct_seconds: sdSeconds } : {}),
      });
      const realMsg: Message = {
        id: sent.id, conv_id: sent.conv_id || convId,
        from_did: user.did, to_did: conv.peer_did,
        type: "text", ciphertext: sent.ciphertext, status: (sent.status as Message["status"]) || "sent",
        sent_at: nowIso, reply_to_id: replyId,
        self_destruct_seconds: sdSeconds,
      };
      replaceMessage(convId, tempId, realMsg);
      setDecrypted((prev) => {
        const next = { ...prev };
        delete next[tempId];
        next[realMsg.id] = text;
        return next;
      });
      // Kendi giden mesajımız kendi cihazımızda bir daha ÇÖZÜLEMEZ (ratchet
      // anahtarı karşı taraf için) — sonraki açılışlarda da okunabilsin diye
      // düz metni şifreli önbelleğe yaz. Başarısızlık göndermeyi etkilemesin.
      cachePlaintext(realMsg.id, text).catch(() => {});
    } catch (e: any) {
      removeMessage(convId, tempId);
      setDecrypted((prev) => {
        const next = { ...prev };
        delete next[tempId];
        return next;
      });
      setInputVal(text);
      setSelfDestructSeconds(sdSeconds);
      Alert.alert("Hata", e?.message || "Mesaj gönderilemedi.");
    } finally {
      setSending(false);
      sendingRef.current = false;
    }
  }, [inputVal, conv, replyTo, user, convId, selfDestructSeconds, sendGroupText]);

  // B5 — grup medya gönderimi. sendGroupText'in optimistic-echo/rollback
  // desenini izler (bkz. yukarı) ama payload zaten formatlanmış medya
  // string'i ([img]/[video]/[file]/[location]/[voice]) — MLS/ratchet'e
  // sendGroupTextMessage üzerinden AYNI yoldan girer, kripto tarafında
  // text'ten farksız (encryptGroupMessage plaintext:string alır, bkz. B5
  // Faz 0 keşfi). Grupta WS broadcast göndereni HARİÇ tutuğundan
  // (mls_handlers.go, `user_did != ?`) kendi gönderdiğimiz medyayı ekranda
  // görebilmemizin TEK yolu bu optimistic echo — sendGroupText'e dokunulmadı
  // (regresyon riski, zaten canlı doğrulanmış E1 akışı).
  const sendGroupMedia = useCallback(async (payload: string): Promise<void> => {
    if (!conv || !user) throw new Error("Sohbet yüklenmedi.");
    const groupId = conv.mls_group_id;
    if (!groupId) throw new Error("Bu grup MLS'e bağlı değil.");
    const tempId = `temp-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
    const nowIso = new Date().toISOString();
    addMessage({
      id: tempId, conv_id: convId, from_did: user.did, to_did: convId,
      type: "text", ciphertext: payload, status: "pending", sent_at: nowIso,
    });
    setDecrypted((prev) => ({ ...prev, [tempId]: payload }));
    try {
      const sent = await sendGroupTextMessage(groupId, payload);
      replaceMessage(convId, tempId, {
        id: sent.id, conv_id: convId, from_did: user.did, to_did: convId,
        type: "text", ciphertext: "", status: "sent", sent_at: sent.created_at || nowIso,
      });
      setDecrypted((prev) => {
        const next = { ...prev };
        delete next[tempId];
        next[sent.id] = payload;
        return next;
      });
      cachePlaintext(sent.id, payload).catch(() => {});
    } catch (e) {
      removeMessage(convId, tempId);
      setDecrypted((prev) => {
        const next = { ...prev };
        delete next[tempId];
        return next;
      });
      throw e;
    }
  }, [conv, user, convId]);

  const pickAndSend = useCallback(async (mediaType: "Images" | "Videos") => {
    const kind = classifyConv(conv);
    if (kind === "unknown") { Alert.alert("Grup", GRUP_KAPALI_MESAJI); return; }
    if (kind === "direct" && !conv?.peer_did) return;
    if (sendingRef.current) return;
    setAttachOpen(false);
    const { status } = await ImagePicker.requestMediaLibraryPermissionsAsync();
    if (status !== "granted") {
      Alert.alert("İzin gerekli", "Galeri erişimine izin verin.");
      return;
    }
    const result = await ImagePicker.launchImageLibraryAsync({
      mediaTypes: mediaType === "Images"
        ? ImagePicker.MediaTypeOptions.Images
        : ImagePicker.MediaTypeOptions.Videos,
      quality: 0.7,
    });
    if (result.canceled || !result.assets[0]) return;
    sendingRef.current = true;
    setSending(true);
    await Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    try {
      const asset = result.assets[0];
      let payload: string;
      let type: string;
      if (mediaType === "Images") {
        // Kamera fotoğrafları 4000×3000+ olabilir — resize'sız ham haliyle
        // base64 inline şifrelenip gönderiliyordu (yavaş gönderim, yüksek
        // veri kullanımı, düşük uçlu cihazda decode OOM riski). 1280px'i
        // aşan kenar varsa küçültülür, yoksa sadece yeniden sıkıştırılır.
        const action = resizeActionFor(asset.width, asset.height);
        const manipulated = await ImageManipulator.manipulateAsync(
          asset.uri,
          action ? [action] : [],
          { compress: 0.7, format: ImageManipulator.SaveFormat.JPEG, base64: true }
        );
        if (!manipulated.base64) throw new Error("Görsel işlenemedi");
        payload = `[img]${manipulated.base64}`;
        type = "image";
      } else {
        // B11 — blob'u yükleme öncesi şifrele (rastgele blob-key, mesaj
        // payload'ında taşınır) — sunucu/MinIO artık düz video baytı görmüyor.
        const { tempUri, keyB64 } = await encryptFileForUpload(asset.uri);
        const uploaded = await api.uploadMedia({ uri: tempUri, name: "video.bin", type: "application/octet-stream" }, "media");
        payload = `[video]${uploaded.url}|${keyB64}`;
        type = "video";
      }
      if (kind === "group") await sendGroupMedia(payload);
      else await sendSealedMessage(conv!.peer_did!, payload, type);
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Dosya gönderilemedi.");
    } finally {
      setSending(false);
      sendingRef.current = false;
    }
  }, [conv, sendGroupMedia]);

  const pickAndSendDocument = useCallback(async () => {
    const kind = classifyConv(conv);
    if (kind === "unknown") { Alert.alert("Grup", GRUP_KAPALI_MESAJI); return; }
    if (kind === "direct" && !conv?.peer_did) return;
    if (sendingRef.current) return;
    setAttachOpen(false);
    try {
      const result = await DocumentPicker.getDocumentAsync({ copyToCacheDirectory: true });
      if (result.canceled) return;
      const doc = result.assets[0];
      sendingRef.current = true;
      setSending(true);
      await Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
      // B11 — blob'u yükleme öncesi şifrele (bkz. video dalı yorumu).
      const { tempUri, keyB64 } = await encryptFileForUpload(doc.uri);
      const uploaded = await api.uploadMedia({ uri: tempUri, name: "file.bin", type: "application/octet-stream" }, "media");
      const payload = `[file]${doc.name}|${uploaded.url}|${keyB64}`;
      if (kind === "group") await sendGroupMedia(payload);
      else await sendSealedMessage(conv!.peer_did!, payload, "file");
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Dosya gönderilemedi.");
    } finally {
      setSending(false);
      sendingRef.current = false;
    }
  }, [conv, sendGroupMedia]);

  const sendLocation = useCallback(async () => {
    const kind = classifyConv(conv);
    if (kind === "unknown") { Alert.alert("Grup", GRUP_KAPALI_MESAJI); return; }
    if (kind === "direct" && !conv?.peer_did) return;
    if (sendingRef.current) return;
    setAttachOpen(false);
    const { status } = await Location.requestForegroundPermissionsAsync();
    if (status !== "granted") {
      Alert.alert("İzin gerekli", "Konum erişimine izin verin.");
      return;
    }
    sendingRef.current = true;
    setSending(true);
    await Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    try {
      const loc = await Location.getCurrentPositionAsync({ accuracy: Location.Accuracy.Balanced });
      const payload = `[location]${loc.coords.latitude},${loc.coords.longitude}`;
      if (kind === "group") await sendGroupMedia(payload);
      else await sendSealedMessage(conv!.peer_did!, payload, "location");
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Konum gönderilemedi.");
    } finally {
      setSending(false);
      sendingRef.current = false;
    }
  }, [conv, sendGroupMedia]);

  const startRecording = useCallback(async () => {
    const kind = classifyConv(conv);
    if (kind === "unknown") { Alert.alert("Grup", GRUP_KAPALI_MESAJI); return; }
    if (kind === "direct" && !conv?.peer_did) return;
    try {
      const { granted } = await Audio.requestPermissionsAsync();
      if (!granted) { Alert.alert("İzin gerekli", "Mikrofon erişimine izin verin."); return; }
      await Audio.setAudioModeAsync({ allowsRecordingIOS: true, playsInSilentModeIOS: true });
      const { recording } = await Audio.Recording.createAsync(Audio.RecordingOptionsPresets.HIGH_QUALITY);
      recordingRef.current = recording;
      setIsRecording(true);
      setRecordingSec(0);
      recordTimerRef.current = setInterval(() => setRecordingSec((s) => s + 1), 1000);
      await Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    } catch {
      Alert.alert("Hata", "Kayıt başlatılamadı.");
    }
  }, [conv]);

  const stopRecording = useCallback(async () => {
    const kind = classifyConv(conv);
    if (kind === "unknown") { Alert.alert("Grup", GRUP_KAPALI_MESAJI); return; }
    if (!recordingRef.current || (kind === "direct" && !conv?.peer_did)) return;
    if (recordTimerRef.current) clearInterval(recordTimerRef.current);
    setIsRecording(false);
    setRecordingSec(0);
    try {
      await recordingRef.current.stopAndUnloadAsync();
      await Audio.setAudioModeAsync({ allowsRecordingIOS: false });
      const uri = recordingRef.current.getURI();
      recordingRef.current = null;
      if (!uri) return;
      sendingRef.current = true;
      setSending(true);
      await Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      // B11 — blob'u yükleme öncesi şifrele (bkz. pickAndSend video dalı yorumu).
      const { tempUri, keyB64 } = await encryptFileForUpload(uri);
      const uploaded = await api.uploadMedia({ uri: tempUri, name: "voice.bin", type: "application/octet-stream" }, "media");
      const payload = `[voice]${uploaded.url}|${keyB64}`;
      if (kind === "group") await sendGroupMedia(payload);
      else await sendSealedMessage(conv!.peer_did!, payload, "voice");
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Ses gönderilemedi.");
    } finally {
      sendingRef.current = false;
      setSending(false);
    }
  }, [conv, sendGroupMedia]);

  const handleShare = () => setInviteModal(true);

  const handleCreateInvite = async () => {
    setInviteCreating(true);
    try {
      const res = await api.createConvInvite(convId, {
        slug: inviteSlug || undefined,
        max_uses: inviteMaxUses ? parseInt(inviteMaxUses, 10) : 0,
        max_members: inviteMaxMembers ? parseInt(inviteMaxMembers, 10) : 0,
      });
      setInviteModal(false);
      setInviteSlug("");
      setInviteMaxUses("");
      setInviteMaxMembers("");
      await Share.share({ message: `Obscura'da bize katıl: ${res.invite_url}` });
    } catch (e: any) {
      Alert.alert("Hata", e.message || "Davet oluşturulamadı");
    } finally {
      setInviteCreating(false);
    }
  };

  const cancelRecording = useCallback(async () => {
    if (!recordingRef.current) return;
    if (recordTimerRef.current) clearInterval(recordTimerRef.current);
    setIsRecording(false);
    setRecordingSec(0);
    try {
      await recordingRef.current.stopAndUnloadAsync();
      recordingRef.current = null;
    } catch {}
    await Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
  }, []);

  const playVoice = useCallback(async (url: string, keyB64: string | null, msgId: string) => {
    if (playingId === msgId) {
      await soundRef.current?.stopAsync();
      await soundRef.current?.unloadAsync();
      soundRef.current = null;
      setPlayingId(null);
      return;
    }
    if (soundRef.current) {
      await soundRef.current.stopAsync();
      await soundRef.current.unloadAsync();
      soundRef.current = null;
    }
    setPlayingId(msgId);
    try {
      // B11 — keyB64 varsa şifreli blob (yeni format): indir+çöz+geçici
      // dosyaya yaz, player'a onu ver. Yoksa (eski mesaj, legacy) direkt
      // uzak URL'i çal — geriye uyumluluk, bkz. B11 Faz 0.
      const playUri = keyB64 ? await decryptToTempFile(url, keyB64, "m4a") : url;
      const { sound } = await Audio.Sound.createAsync({ uri: playUri }, { shouldPlay: true });
      soundRef.current = sound;
      sound.setOnPlaybackStatusUpdate((s) => {
        if (s.isLoaded && s.didJustFinish) {
          setPlayingId(null);
          sound.unloadAsync();
          soundRef.current = null;
        }
      });
    } catch {
      setPlayingId(null);
      Alert.alert("Hata", "Ses oynatılamadı.");
    }
  }, [playingId]);

  const onLongPressMessage = useCallback((msg: Message) => {
    const mine = msg.from_did === user?.did;
    const text = decrypted[msg.id] ?? "";
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    const options = [
      { text: "Yanıtla", onPress: () => setReplyTo(msg) },
      ...(text && !text.startsWith("[") ? [{ text: "Kopyala", onPress: () => Clipboard.setStringAsync(text) }] : []),
      ...(mine ? [{
        text: "Sil", style: "destructive" as const,
        onPress: () => {
          Alert.alert("Mesajı Sil", "Bu mesaj kalıcı olarak silinecek.", [
            { text: "İptal", style: "cancel" },
            {
              text: "Sil", style: "destructive",
              onPress: async () => {
                try {
                  await api.deleteMessage(msg.id);
                  removeMessage(msg.conv_id, msg.id);
                } catch (e: any) {
                  Alert.alert("Hata", e?.message || "Mesaj silinemedi.");
                }
              },
            },
          ]);
        },
      }] : []),
      { text: "İptal", style: "cancel" as const },
    ];
    Alert.alert("", "", options as any);
  }, [user, decrypted, removeMessage]);

  const renderMsgContent = (msg: Message) => {
    const txt = decrypted[msg.id];
    const mine = msg.from_did === user?.did;

    // Süresi dolan mesaj: backend ciphertext'i zaten sildi (ADIM 3), client
    // plaintext-cache'i de purge etti (bkz. lib/ws-handlers.ts). decrypted
    // state'te hâlâ eski bir kopya olsa bile status kontrolü ÖNCE gelir —
    // aksi halde self-destruct görsel olarak sahte kalır.
    if (msg.status === "expired") {
      return (
        <Text style={[styles.msgText, mine ? styles.msgTextMine : styles.msgTextTheirs, { fontSize, fontStyle: "italic", opacity: 0.7 }]}>
          Mesaj süresi doldu
        </Text>
      );
    }

    if (txt === undefined) return <Text style={[styles.msgText, styles.msgTextTheirs, { fontSize }]}>{"..."}</Text>;

    if (txt.startsWith("[img]")) {
      return (
        <Image
          source={{ uri: `data:image/jpeg;base64,${txt.slice(5)}` }}
          style={styles.msgImage}
          resizeMode="cover"
        />
      );
    }

    if (txt.startsWith("[voice]")) {
      const { url, keyB64 } = parseMediaKey(txt.slice(7));
      const playing = playingId === msg.id;
      return (
        <TouchableOpacity style={styles.voiceRow} onPress={() => playVoice(url, keyB64, msg.id)}>
          <Ionicons name={playing ? "stop" : "play"} size={18} color={mine ? colors.head : colors.accent} />
          <View style={styles.voiceWave}>
            {Array.from({ length: 16 }).map((_, i) => (
              <View key={i} style={[styles.voiceBar, { height: 4 + (i % 5) * 3 }, mine ? styles.voiceBarMine : styles.voiceBarTheirs]} />
            ))}
          </View>
          <Text style={[styles.voiceDur, mine ? styles.msgTextMine : styles.msgTextTheirs]}>🎤</Text>
        </TouchableOpacity>
      );
    }

    if (txt.startsWith("[video]")) {
      return (
        <View style={styles.videoPlaceholder}>
          <Ionicons name="videocam" size={28} color={colors.accent} />
          <Text style={[styles.msgText, mine ? styles.msgTextMine : styles.msgTextTheirs, { fontSize }]}>Video</Text>
        </View>
      );
    }

    if (txt.startsWith("[file]")) {
      const parts = txt.slice(6).split("|");
      const filename = parts[0] || "Dosya";
      return (
        <View style={styles.fileRow}>
          <Ionicons name="document-outline" size={20} color={mine ? colors.head : colors.accent} />
          <Text style={[styles.msgText, mine ? styles.msgTextMine : styles.msgTextTheirs, { fontSize }]} numberOfLines={1}>{filename}</Text>
        </View>
      );
    }

    if (txt.startsWith("[location]")) {
      const coords = txt.slice(10);
      const [lat, lng] = coords.split(",");
      return (
        <View style={styles.locationRow}>
          <Ionicons name="location" size={20} color={mine ? colors.head : colors.accent} />
          <View>
            <Text style={[styles.msgText, mine ? styles.msgTextMine : styles.msgTextTheirs, { fontSize }]}>Konum</Text>
            <Text style={[styles.locationCoords, mine ? styles.msgTextMine : { color: colors.dim }]} numberOfLines={1}>
              {parseFloat(lat).toFixed(4)}, {parseFloat(lng).toFixed(4)}
            </Text>
          </View>
        </View>
      );
    }

    return (
      <Text style={[styles.msgText, mine ? styles.msgTextMine : styles.msgTextTheirs, { fontSize }]}>
        {txt}
      </Text>
    );
  };

  const getReplyPreview = (replyToId: string): string => {
    const replied = convMsgs.find((m) => m.id === replyToId);
    if (!replied) return "Mesaj";
    const txt = decrypted[replied.id];
    if (!txt) return "...";
    if (txt.startsWith("[img]")) return "📷 Fotoğraf";
    if (txt.startsWith("[voice]")) return "🎤 Ses mesajı";
    if (txt.startsWith("[video]")) return "🎬 Video";
    if (txt.startsWith("[file]")) return "📄 " + txt.slice(6).split("|")[0];
    if (txt.startsWith("[location]")) return "📍 Konum";
    return txt.length > 60 ? txt.slice(0, 60) + "…" : txt;
  };

  const getReplyAuthor = (replyToId: string): string => {
    const replied = convMsgs.find((m) => m.id === replyToId);
    if (!replied) return "";
    return replied.from_did === user?.did ? "Sen" : peerName;
  };

  const renderMessage = ({ item: msg, index }: { item: Message; index: number }) => {
    const mine = msg.from_did === user?.did;
    const prevMsg = convMsgs[index - 1];
    const nextMsg = convMsgs[index + 1];
    const sameAsPrev = prevMsg?.from_did === msg.from_did;
    const sameAsNext = nextMsg?.from_did === msg.from_did;
    const isLast = !sameAsNext;
    const showDate = !prevMsg || new Date(prevMsg.sent_at).toDateString() !== new Date(msg.sent_at).toDateString();

    return (
      <>
        {showDate && (
          <View style={styles.dateDivider}>
            <Text style={styles.dateText}>
              {(() => {
                const d = new Date(msg.sent_at);
                const today = new Date();
                if (d.toDateString() === today.toDateString()) return "Bugün";
                const yest = new Date(today); yest.setDate(today.getDate() - 1);
                if (d.toDateString() === yest.toDateString()) return "Dün";
                return d.toLocaleDateString("tr-TR", { day: "numeric", month: "long" });
              })()}
            </Text>
          </View>
        )}
        <View style={[styles.msgWrapper, mine ? styles.msgMine : styles.msgTheirs]}>
          {!mine && !sameAsPrev && (
            <View style={styles.avatarSpace}>
              <Avatar name={peerName} size="xs" />
            </View>
          )}
          {!mine && sameAsPrev && <View style={styles.avatarPlaceholder} />}
          <Pressable onLongPress={() => onLongPressMessage(msg)} delayLongPress={350}>
            <View style={[
              styles.bubble,
              mine ? styles.bubbleMine : styles.bubbleTheirs,
              mine && sameAsPrev && styles.bubbleMineNotFirst,
              mine && !sameAsNext && styles.bubbleMineLast,
              !mine && sameAsPrev && styles.bubbleTheirsNotFirst,
              !mine && !sameAsNext && styles.bubbleTheirsLast,
            ]}>
              {msg.reply_to_id && (
                <View style={[styles.replyQuote, mine ? styles.replyQuoteMine : styles.replyQuoteTheirs]}>
                  <View style={styles.replyQuoteLine} />
                  <View style={styles.replyQuoteBody}>
                    <Text style={styles.replyQuoteAuthor} numberOfLines={1}>
                      {getReplyAuthor(msg.reply_to_id)}
                    </Text>
                    <Text style={styles.replyQuoteText} numberOfLines={1}>
                      {getReplyPreview(msg.reply_to_id)}
                    </Text>
                  </View>
                </View>
              )}
              {renderMsgContent(msg)}
            </View>
          </Pressable>
          {isLast && (
            <View style={[styles.msgMeta, mine ? styles.msgMetaMine : styles.msgMetaTheirs]}>
              <Text style={styles.msgTime}>{formatFullTime(msg.sent_at)}</Text>
              {mine && <StatusIcon status={msg.status} selfDestruct={isSelfDestructMessage(msg)} />}
            </View>
          )}
        </View>
      </>
    );
  };

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />

      {/* Header */}
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <TouchableOpacity
          style={styles.headerInfo}
          onPress={() => {
            if (conv?.is_group) {
              router.push(`/(main)/group-profile?convId=${convId}` as any);
            } else if (conv?.peer_did) {
              router.push(`/(main)/user-profile?did=${conv.peer_did}&name=${encodeURIComponent(peerName)}&convId=${convId}` as any);
            }
          }}
          activeOpacity={0.7}
        >
          <Avatar name={peerName} size="sm" online={peerOnline} tier={conv?.peer_tier} imageUrl={conv?.is_group ? conv?.avatar_url : undefined} />
          <View>
            <Text style={styles.headerName}>{peerName}</Text>
            <Text style={styles.headerStatus}>
              {isTyping ? "yazıyor..." : peerOnline ? "çevrimiçi" : conv?.last_msg_at
                ? `son görülme ${new Date(conv.last_msg_at).toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" })}`
                : "çevrimdışı"}
            </Text>
          </View>
        </TouchableOpacity>
        <View style={styles.headerActions}>
          <TouchableOpacity style={styles.iconBtn} onPress={handleShare}>
            <Ionicons name="share-outline" size={20} color={colors.sub} />
          </TouchableOpacity>
          {classifyConv(conv) === "direct" && (
            <TouchableOpacity
              style={styles.iconBtn}
              onPress={() => router.push(`/(main)/voice-call?peer=${conv?.peer_did}&peerName=${encodeURIComponent(peerName)}&convId=${convId}` as any)}
            >
              <Ionicons name="call-outline" size={20} color={colors.sub} />
            </TouchableOpacity>
          )}
          {classifyConv(conv) === "direct" && (
            <TouchableOpacity
              style={styles.iconBtn}
              onPress={() => router.push(`/(main)/video-call?peer=${conv?.peer_did}&peerName=${encodeURIComponent(peerName)}&convId=${convId}` as any)}
            >
              <Ionicons name="videocam-outline" size={20} color={colors.sub} />
            </TouchableOpacity>
          )}
        </View>
      </View>

      <KeyboardAvoidingView
        style={{ flex: 1 }}
        behavior={Platform.OS === "ios" ? "padding" : "height"}
        keyboardVerticalOffset={0}
      >
        <FlatList
          ref={flatListRef}
          data={convMsgs}
          keyExtractor={(m) => m.id}
          renderItem={renderMessage}
          onContentSizeChange={() => flatListRef.current?.scrollToEnd({ animated: false })}
          contentContainerStyle={styles.messageList}
          ListEmptyComponent={
            !loading ? (
              <View style={styles.empty}>
                <Ionicons name="shield-checkmark" size={36} color={colors.accent} />
                <Text style={styles.emptyTitle}>Sohbet başladı</Text>
                <Text style={styles.emptyText}>Mesajlar uçtan uca şifreli</Text>
              </View>
            ) : null
          }
        />

        {/* Reply bar */}
        {replyTo && (
          <View style={styles.replyBar}>
            <Ionicons name="return-down-forward-outline" size={14} color={colors.accent} />
            <View style={styles.replyBarLine} />
            <View style={styles.replyBarContent}>
              <Text style={styles.replyBarAuthor} numberOfLines={1}>
                {getReplyAuthor(replyTo.id)}
              </Text>
              <Text style={styles.replyText} numberOfLines={1}>
                {getReplyPreview(replyTo.id)}
              </Text>
            </View>
            <TouchableOpacity onPress={() => setReplyTo(null)} hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}>
              <Ionicons name="close-circle" size={18} color={colors.dim} />
            </TouchableOpacity>
          </View>
        )}

        {/* Recording bar */}
        {isRecording && (
          <View style={styles.recordingBar}>
            <View style={styles.recDot} />
            <Text style={styles.recText}>
              Kaydediliyor {Math.floor(recordingSec / 60).toString().padStart(2, "0")}:{(recordingSec % 60).toString().padStart(2, "0")}
            </Text>
            <TouchableOpacity onPress={cancelRecording} style={styles.recCancel}>
              <Ionicons name="close" size={18} color={colors.red} />
              <Text style={styles.recCancelText}>İptal</Text>
            </TouchableOpacity>
          </View>
        )}

        {/* Composer */}
        <View style={[styles.composer, { paddingBottom: insets.bottom + 8 }]}>
          {!isRecording && (
            <TouchableOpacity style={styles.attachBtn} onPress={() => setAttachOpen(true)} disabled={sending}>
              <Ionicons name="attach" size={22} color={colors.sub} />
            </TouchableOpacity>
          )}
          {!isRecording && (
            <TouchableOpacity
              style={styles.attachBtn}
              onPress={() => setTimerModalOpen(true)}
              disabled={sending}
              accessibilityLabel={`Self-destruct süresi: ${formatSelfDestructLabel(selfDestructSeconds)}`}
            >
              <Ionicons name="timer-outline" size={22} color={selfDestructSeconds !== null ? colors.amber : colors.sub} />
            </TouchableOpacity>
          )}
          {!isRecording && (
            <View style={styles.inputWrap}>
              <TextInput
                style={styles.msgInput}
                value={inputVal}
                onChangeText={setInputVal}
                placeholder="Mesaj..."
                placeholderTextColor={colors.dim}
                multiline
                maxLength={4000}
                returnKeyType="default"
              />
            </View>
          )}
          {!isRecording && (
            <TouchableOpacity
              style={[styles.sendBtn, inputVal.trim() ? styles.sendBtnActive : styles.sendBtnInactive]}
              onPress={inputVal.trim() ? sendMessage : undefined}
              onLongPress={!inputVal.trim() ? startRecording : undefined}
              delayLongPress={300}
              disabled={sending}
            >
              {sending
                ? <ActivityIndicator size="small" color={inputVal.trim() ? colors.void : colors.dim} />
                : <Ionicons
                    name={inputVal.trim() ? "arrow-up" : "mic"}
                    size={20}
                    color={inputVal.trim() ? colors.void : colors.dim}
                  />
              }
            </TouchableOpacity>
          )}
          {isRecording && (
            <TouchableOpacity style={[styles.sendBtn, styles.sendBtnActive, { flex: 1, borderRadius: radius.xl }]} onPress={stopRecording}>
              <Ionicons name="stop" size={20} color={colors.void} />
              <Text style={{ color: colors.void, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>Gönder</Text>
            </TouchableOpacity>
          )}
        </View>
      </KeyboardAvoidingView>

      {/* Attachment Modal */}
      <Modal visible={attachOpen} transparent animationType="slide" onRequestClose={() => setAttachOpen(false)}>
        <TouchableOpacity style={styles.attachOverlay} activeOpacity={1} onPress={() => setAttachOpen(false)}>
          <View style={[styles.attachSheet, { paddingBottom: insets.bottom + 8 }]}>
            <View style={styles.attachHandle} />
            <Text style={styles.attachTitle}>Dosya Ekle</Text>
            <View style={styles.attachGrid}>
              <TouchableOpacity style={styles.attachItem} onPress={() => pickAndSend("Images")}>
                <View style={[styles.attachIcon, { backgroundColor: "rgba(74,222,128,0.12)" }]}>
                  <Ionicons name="image-outline" size={26} color={colors.accent} />
                </View>
                <Text style={styles.attachLabel}>Fotoğraf</Text>
              </TouchableOpacity>
              <TouchableOpacity style={styles.attachItem} onPress={() => pickAndSend("Videos")}>
                <View style={[styles.attachIcon, { backgroundColor: "rgba(99,102,241,0.12)" }]}>
                  <Ionicons name="videocam-outline" size={26} color="#818cf8" />
                </View>
                <Text style={styles.attachLabel}>Video</Text>
              </TouchableOpacity>
              <TouchableOpacity style={styles.attachItem} onPress={pickAndSendDocument}>
                <View style={[styles.attachIcon, { backgroundColor: "rgba(251,191,36,0.12)" }]}>
                  <Ionicons name="document-outline" size={26} color="#fbbf24" />
                </View>
                <Text style={styles.attachLabel}>Dosya</Text>
              </TouchableOpacity>
              <TouchableOpacity style={styles.attachItem} onPress={sendLocation}>
                <View style={[styles.attachIcon, { backgroundColor: "rgba(239,68,68,0.12)" }]}>
                  <Ionicons name="location-outline" size={26} color="#f87171" />
                </View>
                <Text style={styles.attachLabel}>Konum</Text>
              </TouchableOpacity>
            </View>
          </View>
        </TouchableOpacity>
      </Modal>

      {/* Self-Destruct Timer Modal */}
      <Modal visible={timerModalOpen} transparent animationType="slide" onRequestClose={() => setTimerModalOpen(false)}>
        <TouchableOpacity style={styles.attachOverlay} activeOpacity={1} onPress={() => setTimerModalOpen(false)}>
          <View style={[styles.attachSheet, { paddingBottom: insets.bottom + 8 }]}>
            <View style={styles.attachHandle} />
            <Text style={styles.attachTitle}>Kendini İmha Süresi</Text>
            {SELF_DESTRUCT_OPTIONS.map((opt) => {
              const selected = selfDestructSeconds === opt.seconds;
              return (
                <TouchableOpacity
                  key={String(opt.seconds)}
                  style={{
                    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
                    paddingVertical: 14, paddingHorizontal: 4,
                  }}
                  onPress={() => { setSelfDestructSeconds(opt.seconds); setTimerModalOpen(false); }}
                >
                  <Text style={{ color: selected ? colors.amber : colors.head, fontSize: 15, fontWeight: selected ? "600" : "400" }}>
                    {opt.label}
                  </Text>
                  {selected && <Ionicons name="checkmark" size={18} color={colors.amber} />}
                </TouchableOpacity>
              );
            })}
          </View>
        </TouchableOpacity>
      </Modal>

      {/* Invite Modal */}
      <Modal visible={inviteModal} transparent animationType="slide" onRequestClose={() => setInviteModal(false)}>
        <View style={styles.inviteOverlay}>
          <View style={[styles.inviteSheet, { paddingBottom: insets.bottom + 8 }]}>
            <View style={styles.attachHandle} />
            <Text style={styles.inviteTitle}>Davet Linki Oluştur</Text>
            <TextInput
              style={styles.inviteInput}
              placeholder="Özel link adı (opsiyonel, örn: hoşgeldinDostum)"
              placeholderTextColor={colors.dim}
              value={inviteSlug}
              onChangeText={setInviteSlug}
              autoCapitalize="none"
            />
            <TextInput
              style={styles.inviteInput}
              placeholder="Maks. kullanım sayısı (0=sınırsız)"
              placeholderTextColor={colors.dim}
              value={inviteMaxUses}
              onChangeText={setInviteMaxUses}
              keyboardType="numeric"
            />
            <TextInput
              style={styles.inviteInput}
              placeholder="Maks. üye sayısı (0=sınırsız)"
              placeholderTextColor={colors.dim}
              value={inviteMaxMembers}
              onChangeText={setInviteMaxMembers}
              keyboardType="numeric"
            />
            <TouchableOpacity style={styles.inviteBtn} onPress={handleCreateInvite} disabled={inviteCreating}>
              {inviteCreating
                ? <ActivityIndicator color={colors.void} />
                : <Text style={styles.inviteBtnText}>Oluştur ve Paylaş</Text>
              }
            </TouchableOpacity>
            <TouchableOpacity onPress={() => setInviteModal(false)}>
              <Text style={styles.inviteCancel}>İptal</Text>
            </TouchableOpacity>
          </View>
        </View>
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },
  header: {
    flexDirection: "row", alignItems: "center",
    paddingHorizontal: spacing.sm, paddingVertical: 10,
    borderBottomWidth: 1, borderBottomColor: colors.border + "60",
    gap: 4,
  },
  backBtn: { width: 40, height: 40, alignItems: "center", justifyContent: "center" },
  headerInfo: { flex: 1, flexDirection: "row", alignItems: "center", gap: 10 },
  headerName: { fontSize: typography.md, fontWeight: "600", color: colors.head },
  headerStatus: { fontSize: 11, color: colors.dim, marginTop: 1 },
  headerActions: { flexDirection: "row", gap: 0 },
  iconBtn: { width: 38, height: 38, alignItems: "center", justifyContent: "center" },
  messageList: { paddingHorizontal: spacing.sm, paddingTop: spacing.sm, flexGrow: 1 },
  msgWrapper: { marginVertical: 1 },
  msgMine: { alignItems: "flex-end" },
  msgTheirs: { flexDirection: "row", alignItems: "flex-end", gap: 6 },
  avatarSpace: { width: 28, alignItems: "flex-start", paddingBottom: 2 },
  avatarPlaceholder: { width: 28 },
  bubble: { maxWidth: "75%", paddingHorizontal: 14, paddingVertical: 10 },
  bubbleMine: {
    backgroundColor: "rgba(94,196,110,0.15)",
    borderWidth: 1, borderColor: "rgba(94,196,110,0.15)",
    borderRadius: 22, borderTopRightRadius: 22, borderBottomRightRadius: 6,
  },
  bubbleTheirs: {
    backgroundColor: colors.raised,
    borderWidth: 1, borderColor: colors.border,
    borderRadius: 22, borderTopLeftRadius: 22, borderBottomLeftRadius: 6,
  },
  bubbleMineNotFirst: { borderTopRightRadius: 6 },
  bubbleMineLast: { borderBottomRightRadius: 4 },
  bubbleTheirsNotFirst: { borderTopLeftRadius: 6 },
  bubbleTheirsLast: { borderBottomLeftRadius: 4 },
  msgText: { fontSize: typography.sm, lineHeight: 20 },
  msgTextMine: { color: colors.head },
  msgTextTheirs: { color: colors.body },
  msgMeta: { flexDirection: "row", alignItems: "center", gap: 3, marginTop: 2, paddingHorizontal: 4 },
  msgMetaMine: { justifyContent: "flex-end" },
  msgMetaTheirs: { justifyContent: "flex-start" },
  msgTime: { fontSize: 10, color: colors.dim, fontFamily: "monospace" },
  dateDivider: { alignItems: "center", marginVertical: 12 },
  dateText: {
    fontSize: 11, color: colors.dim,
    backgroundColor: colors.raised, borderWidth: 1, borderColor: colors.border,
    paddingHorizontal: 12, paddingVertical: 4, borderRadius: radius.full,
  },
  empty: { flex: 1, alignItems: "center", justifyContent: "center", gap: 8, paddingVertical: 60 },
  emptyTitle: { fontSize: typography.sm, color: colors.body, fontWeight: "500" },
  emptyText: { fontSize: 12, color: colors.dim },
  // Reply bar (composer)
  replyBar: {
    flexDirection: "row", alignItems: "center", gap: 8,
    paddingHorizontal: spacing.md, paddingVertical: 9,
    backgroundColor: colors.raised,
    borderTopWidth: 1, borderTopColor: "rgba(74,222,128,0.12)",
  },
  replyBarLine: { width: 2, height: 30, backgroundColor: colors.accent, borderRadius: 2, flexShrink: 0 },
  replyBarContent: { flex: 1, gap: 1 },
  replyBarAuthor: { fontSize: 11, color: colors.accent, fontWeight: "600" },
  replyText: { fontSize: 12, color: colors.sub },
  // Reply quote inside bubble
  replyQuote: {
    flexDirection: "row", alignItems: "stretch", gap: 7,
    borderRadius: 8, paddingHorizontal: 8, paddingVertical: 6,
    marginBottom: 6,
  },
  replyQuoteMine: { backgroundColor: "rgba(0,0,0,0.2)" },
  replyQuoteTheirs: { backgroundColor: "rgba(255,255,255,0.07)" },
  replyQuoteLine: { width: 2, borderRadius: 1, backgroundColor: colors.accent, alignSelf: "stretch" },
  replyQuoteBody: { flex: 1, gap: 1 },
  replyQuoteAuthor: { fontSize: 10, color: colors.accent, fontWeight: "700" },
  replyQuoteText: { fontSize: 11, color: colors.sub },
  // Media
  msgImage: { width: 200, height: 150, borderRadius: 12 },
  voiceRow: { flexDirection: "row", alignItems: "center", gap: 8, paddingVertical: 2 },
  voiceWave: { flex: 1, flexDirection: "row", alignItems: "center", gap: 2 },
  voiceBar: { width: 2.5, borderRadius: 2, backgroundColor: colors.accent + "80" },
  voiceBarMine: { backgroundColor: colors.head + "80" },
  voiceBarTheirs: { backgroundColor: colors.accent + "80" },
  voiceDur: { fontSize: 11 },
  videoPlaceholder: { flexDirection: "row", alignItems: "center", gap: 8, paddingVertical: 4 },
  fileRow: { flexDirection: "row", alignItems: "center", gap: 8, maxWidth: 180 },
  locationRow: { flexDirection: "row", alignItems: "center", gap: 8 },
  locationCoords: { fontSize: 10, marginTop: 1 },
  // Recording bar
  recordingBar: {
    flexDirection: "row", alignItems: "center", gap: 8,
    paddingHorizontal: spacing.md, paddingVertical: 12,
    backgroundColor: "rgba(239,68,68,0.08)",
    borderTopWidth: 1, borderTopColor: "rgba(239,68,68,0.15)",
  },
  recDot: {
    width: 8, height: 8, borderRadius: 4,
    backgroundColor: colors.red,
  },
  recText: { flex: 1, fontSize: 13, color: colors.red, fontWeight: "500" },
  recCancel: { flexDirection: "row", alignItems: "center", gap: 4 },
  recCancelText: { fontSize: 12, color: colors.red },
  // Composer
  composer: {
    flexDirection: "row", alignItems: "flex-end", gap: 8,
    paddingHorizontal: spacing.sm, paddingTop: 10,
    borderTopWidth: 1, borderTopColor: colors.border + "50",
  },
  attachBtn: { width: 40, height: 44, alignItems: "center", justifyContent: "center" },
  inputWrap: {
    flex: 1, minHeight: 44, maxHeight: 120,
    backgroundColor: colors.raised, borderRadius: radius.xxl,
    borderWidth: 1, borderColor: colors.border,
    paddingHorizontal: 14, paddingVertical: 10,
    justifyContent: "center",
  },
  msgInput: { color: colors.body, fontSize: typography.sm, maxHeight: 100 },
  sendBtn: {
    width: 44, height: 44, borderRadius: 22,
    alignItems: "center", justifyContent: "center",
    flexDirection: "row",
  },
  sendBtnActive: {
    backgroundColor: colors.accent,
    shadowColor: colors.accent, shadowOffset: { width: 0, height: 0 },
    shadowOpacity: 0.4, shadowRadius: 8, elevation: 4,
  },
  sendBtnInactive: { backgroundColor: colors.raised, borderWidth: 1, borderColor: colors.border },
  // Invite modal
  inviteOverlay: { flex: 1, backgroundColor: "rgba(0,0,0,0.6)", justifyContent: "flex-end" },
  inviteSheet: {
    backgroundColor: colors.ground,
    borderTopLeftRadius: radius.xxl, borderTopRightRadius: radius.xxl,
    padding: spacing.xl, gap: spacing.md,
  },
  inviteTitle: { fontSize: typography.base, fontWeight: "700", color: colors.head, textAlign: "center" },
  inviteInput: {
    backgroundColor: colors.raised,
    borderWidth: 1, borderColor: colors.border,
    borderRadius: radius.xl,
    paddingHorizontal: 14, paddingVertical: 12,
    color: colors.body, fontSize: typography.sm,
  },
  inviteBtn: {
    backgroundColor: colors.accent,
    borderRadius: radius.xl,
    paddingVertical: 14,
    alignItems: "center" as const,
    marginTop: spacing.sm,
  },
  inviteBtnText: { color: colors.void, fontSize: typography.sm, fontWeight: "700" as const },
  inviteCancel: { textAlign: "center" as const, color: colors.dim, fontSize: typography.sm, paddingVertical: spacing.sm },
  // Attach modal
  attachOverlay: { flex: 1, backgroundColor: "rgba(0,0,0,0.6)", justifyContent: "flex-end" },
  attachSheet: {
    backgroundColor: colors.ground,
    borderTopLeftRadius: radius.xxl, borderTopRightRadius: radius.xxl,
    padding: spacing.xl, gap: spacing.md,
  },
  attachHandle: {
    width: 40, height: 4, borderRadius: 2,
    backgroundColor: colors.border, alignSelf: "center", marginBottom: 4,
  },
  attachTitle: { fontSize: typography.base, fontWeight: "700", color: colors.head, textAlign: "center" },
  attachGrid: { flexDirection: "row", justifyContent: "space-around", paddingVertical: spacing.sm },
  attachItem: { alignItems: "center", gap: 8 },
  attachIcon: {
    width: 60, height: 60, borderRadius: 20,
    alignItems: "center", justifyContent: "center",
  },
  attachLabel: { fontSize: 12, color: colors.sub, fontWeight: "500" },
});
