import React, { useEffect, useCallback, useRef, useState, useMemo } from "react";
import {
  View, Text, FlatList, TextInput, TouchableOpacity,
  StyleSheet, KeyboardAvoidingView, Platform, StatusBar, Image,
} from "react-native";
import { useLocalSearchParams, router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import * as Haptics from "expo-haptics";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { useStore, type Message } from "@/lib/store";
import { api } from "@/lib/api";
import { Avatar } from "@/components/ui/Avatar";
import { EncryptionBadge } from "@/components/ui/EncryptionBadge";
import { formatFullTime, formatTime } from "@/lib/format";

function StatusIcon({ status }: { status: Message["status"] }) {
  if (status === "read") return (
    <Image source={require("@/assets/icons/status-read.jpeg")} style={{ width: 14, height: 14, tintColor: colors.accent }} resizeMode="contain" />
  );
  if (status === "delivered") return (
    <Image source={require("@/assets/icons/status-delivered.jpeg")} style={{ width: 16, height: 10, tintColor: colors.dim }} resizeMode="contain" />
  );
  if (status === "sent") return (
    <Image source={require("@/assets/icons/status-sent.jpeg")} style={{ width: 14, height: 10, tintColor: colors.dim, opacity: 0.6 }} resizeMode="contain" />
  );
  return <Ionicons name="time-outline" size={11} color={colors.dim} />;
}

export default function ChatScreen() {
  const { id: convId } = useLocalSearchParams<{ id: string }>();
  const insets = useSafeAreaInsets();
  const {
    conversations, messages, setMessages, addMessage,
    updateMsgStatus, onlineUsers, user,
  } = useStore();
  const [inputVal, setInputVal] = useState("");
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [isTyping, setIsTyping] = useState(false);
  const flatListRef = useRef<FlatList>(null);

  const conv = conversations.find((c) => c.id === convId);
  const peerName = conv?.name || conv?.peer_name || "Sohbet";
  const peerOnline = conv?.peer_did ? onlineUsers.has(conv.peer_did) : false;
  const convMsgs = useMemo(() =>
    (messages[convId] || []).filter((m) => m.ciphertext !== "__init__"),
    [messages, convId]
  );

  useEffect(() => {
    if (!convId) return;
    (async () => {
      setLoading(true);
      try {
        const data = await api.getMessages(convId);
        setMessages(convId, data || []);
      } catch {} finally { setLoading(false); }
    })();
  }, [convId]);

  const sendMessage = useCallback(async () => {
    const text = inputVal.trim();
    if (!text || !conv?.peer_did || sending) return;
    setInputVal("");
    setSending(true);
    await Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    try {
      await api.sendMessage({ to_id: conv.peer_did, ciphertext: text, type: "text" });
    } catch { setInputVal(text); }
    finally { setSending(false); }
  }, [inputVal, conv, sending]);

  const renderMessage = ({ item: msg, index }: { item: Message; index: number }) => {
    const mine = msg.from_did === user?.did;
    const prevMsg = convMsgs[index - 1];
    const nextMsg = convMsgs[index + 1];
    const sameAsPrev = prevMsg?.from_did === msg.from_did;
    const sameAsNext = nextMsg?.from_did === msg.from_did;
    const isLast = !sameAsNext;

    // Date divider
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
          <View style={[
            styles.bubble,
            mine ? styles.bubbleMine : styles.bubbleTheirs,
            mine && sameAsPrev && styles.bubbleMineNotFirst,
            mine && !sameAsNext && styles.bubbleMineLast,
            !mine && sameAsPrev && styles.bubbleTheirsNotFirst,
            !mine && !sameAsNext && styles.bubbleTheirsLast,
          ]}>
            <Text style={[styles.msgText, mine ? styles.msgTextMine : styles.msgTextTheirs]}>
              {msg.ciphertext}
            </Text>
          </View>
          {isLast && (
            <View style={[styles.msgMeta, mine ? styles.msgMetaMine : styles.msgMetaTheirs]}>
              <Text style={styles.msgTime}>{formatFullTime(msg.sent_at)}</Text>
              {mine && <StatusIcon status={msg.status} />}
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
          <Ionicons name="chevron-back" size={24} color={colors.body} />
        </TouchableOpacity>
        <View style={styles.headerInfo}>
          <Avatar name={peerName} size="sm" online={peerOnline} tier={conv?.peer_tier} />
          <View>
            <Text style={styles.headerName}>{peerName}</Text>
            <View style={styles.headerSub}>
              <EncryptionBadge />
              <Text style={styles.headerStatus}>
                {isTyping ? "yazıyor..." : peerOnline ? "çevrimiçi" : "uçtan uca şifreli"}
              </Text>
            </View>
          </View>
        </View>
        <View style={styles.headerActions}>
          <TouchableOpacity
            style={styles.iconBtn}
            onPress={() => router.push(`/(main)/call?peer=${conv?.peer_did}`)}
          >
            <Ionicons name="call-outline" size={20} color={colors.sub} />
          </TouchableOpacity>
          <TouchableOpacity style={styles.iconBtn}>
            <Ionicons name="videocam-outline" size={20} color={colors.sub} />
          </TouchableOpacity>
        </View>
      </View>

      {/* Messages */}
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

        {/* Composer */}
        <View style={[styles.composer, { paddingBottom: insets.bottom + 8 }]}>
          <TouchableOpacity style={styles.attachBtn}>
            <Ionicons name="attach" size={22} color={colors.sub} />
          </TouchableOpacity>
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
          <TouchableOpacity
            style={[styles.sendBtn, inputVal.trim() ? styles.sendBtnActive : styles.sendBtnInactive]}
            onPress={inputVal.trim() ? sendMessage : undefined}
            disabled={sending}
          >
            <Ionicons
              name={inputVal.trim() ? "arrow-up" : "mic"}
              size={20}
              color={inputVal.trim() ? colors.void : colors.dim}
            />
          </TouchableOpacity>
        </View>
      </KeyboardAvoidingView>
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
  headerName: { fontSize: typography.sm, fontWeight: "600", color: colors.head },
  headerSub: { flexDirection: "row", alignItems: "center", gap: 4, marginTop: 1 },
  headerStatus: { fontSize: 11, color: colors.dim },
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
  msgTime: { fontSize: 10, color: colors.dim },
  dateDivider: { alignItems: "center", marginVertical: 12 },
  dateText: {
    fontSize: 11, color: colors.dim,
    backgroundColor: colors.raised, borderWidth: 1, borderColor: colors.border,
    paddingHorizontal: 12, paddingVertical: 4, borderRadius: radius.full,
  },
  empty: { flex: 1, alignItems: "center", justifyContent: "center", gap: 8, paddingVertical: 60 },
  emptyTitle: { fontSize: typography.sm, color: colors.body, fontWeight: "500" },
  emptyText: { fontSize: 12, color: colors.dim },
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
  },
  sendBtnActive: {
    backgroundColor: colors.accent,
    shadowColor: colors.accent, shadowOffset: { width: 0, height: 0 },
    shadowOpacity: 0.4, shadowRadius: 8, elevation: 4,
  },
  sendBtnInactive: { backgroundColor: colors.raised, borderWidth: 1, borderColor: colors.border },
});
