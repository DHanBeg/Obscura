import React, { useState, useEffect, useRef, useReducer } from "react";
import {
  View, Text, TouchableOpacity, StyleSheet, StatusBar,
  Animated, Easing, Alert,
} from "react-native";
import { useLocalSearchParams, router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { Avatar } from "@/components/ui/Avatar";
import { useStore } from "@/lib/store";
import { api } from "@/lib/api";
import * as rtcSession from "@/lib/rtcSession";

export default function VoiceCallScreen() {
  const { peer, peerName: rawName, convId, mode, callId: paramCallId } = useLocalSearchParams<{
    peer: string; peerName: string; convId: string; mode?: string; callId?: string;
  }>();
  const insets = useSafeAreaInsets();

  const peerName = rawName || "Arıyor";
  const isCallee = mode === "incoming";
  const callIdRef = useRef(paramCallId || "");

  const pendingCallAnswer = useStore((s) => s.pendingCallAnswer);
  const setPendingCallAnswer = useStore((s) => s.setPendingCallAnswer);

  const [, forceUpdate] = useReducer((x) => x + 1, 0);
  const [duration, setDuration] = useState(0);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const timerStarted = useRef(false);
  const backInitiated = useRef(false);

  const ring1 = useRef(new Animated.Value(1)).current;
  const ring2 = useRef(new Animated.Value(1)).current;
  const ring3 = useRef(new Animated.Value(1)).current;

  const { state, isMuted, isCameraOff } = rtcSession.getState();
  const callState: "ringing" | "active" | "ended" =
    state === "connected" ? "active" :
    state === "ended" ? "ended" :
    "ringing";

  // Subscribe to rtcSession state changes
  useEffect(() => {
    return rtcSession.subscribe(forceUpdate);
  }, []);

  // Start call timer when connected; navigate back when ended
  useEffect(() => {
    const { state: s } = rtcSession.getState();
    if (s === "connected" && !timerStarted.current) {
      timerStarted.current = true;
      timerRef.current = setInterval(() => setDuration((d) => d + 1), 1000);
    }
    if (s === "ended" && !backInitiated.current) {
      backInitiated.current = true;
      if (timerRef.current) clearInterval(timerRef.current);
      setTimeout(() => router.back(), 800);
    }
  });

  // Caller: initiate the call
  useEffect(() => {
    if (isCallee) return;
    (async () => {
      try {
        const sdp = await rtcSession.initCaller("audio");
        const result = await api.callInvite({
          to_did: peer,
          sdp_offer: sdp,
          call_type: "audio",
        });
        callIdRef.current = result.call_id;
      } catch (e: any) {
        Alert.alert("Arama Hatası", e?.message || "Arama başlatılamadı");
        rtcSession.markEnded();
        router.back();
      }
    })();

    return () => {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, []);

  // Apply SDP answer when it arrives (caller side)
  useEffect(() => {
    if (!pendingCallAnswer || isCallee) return;
    rtcSession.applyAnswer(pendingCallAnswer).catch(() => {});
    setPendingCallAnswer(null);
  }, [pendingCallAnswer]);

  // Pulse rings animation
  useEffect(() => {
    const pulse = (val: Animated.Value, delay: number) =>
      Animated.loop(
        Animated.sequence([
          Animated.delay(delay),
          Animated.timing(val, { toValue: 1.55, duration: 1400, easing: Easing.out(Easing.cubic), useNativeDriver: true }),
          Animated.timing(val, { toValue: 1, duration: 0, useNativeDriver: true }),
        ])
      ).start();
    pulse(ring1, 0);
    pulse(ring2, 470);
    pulse(ring3, 940);
  }, []);

  const fmt = (s: number) =>
    `${Math.floor(s / 60).toString().padStart(2, "0")}:${(s % 60).toString().padStart(2, "0")}`;

  const handleEnd = async () => {
    if (timerRef.current) clearInterval(timerRef.current);
    if (callIdRef.current) {
      api.callEnd({ call_id: callIdRef.current, reason: "hung_up" }).catch(() => {});
    }
    rtcSession.markEnded();
    router.back();
  };

  const handleToggleMute = () => {
    rtcSession.toggleMute();
    forceUpdate();
  };

  const handleSwitchToVideo = () => {
    router.replace(`/(main)/video-call?peer=${peer}&peerName=${encodeURIComponent(peerName)}&convId=${convId}&mode=${mode}&callId=${callIdRef.current}` as any);
  };

  const { isMuted: muted } = rtcSession.getState();

  const statusText =
    callState === "ringing"
      ? (isCallee ? "Bağlanıyor..." : "Çağrı yapılıyor...")
      : callState === "ended" ? "Arama Bitti"
      : fmt(duration);

  return (
    <View style={[styles.container, { paddingTop: insets.top, paddingBottom: insets.bottom + 24 }]}>
      <StatusBar barStyle="light-content" />
      <View style={styles.ambient} pointerEvents="none" />

      {/* Header */}
      <View style={styles.header}>
        <TouchableOpacity onPress={handleEnd} style={styles.iconBtn}>
          <Ionicons name="chevron-down" size={24} color={colors.sub} />
        </TouchableOpacity>
        <View style={styles.e2eTag}>
          <Ionicons name="lock-closed" size={10} color={colors.accent} />
          <Text style={styles.e2eText}>Uçtan uca şifreli sesli arama</Text>
        </View>
        <View style={styles.iconBtn} />
      </View>

      {/* Avatar hero */}
      <View style={styles.avatarSection}>
        <View style={styles.ringsWrap}>
          <Animated.View style={[styles.ring, styles.ringLg, {
            transform: [{ scale: ring3 }],
            opacity: ring3.interpolate({ inputRange: [1, 1.55], outputRange: [0.07, 0] }),
          }]} />
          <Animated.View style={[styles.ring, styles.ringMd, {
            transform: [{ scale: ring2 }],
            opacity: ring2.interpolate({ inputRange: [1, 1.55], outputRange: [0.12, 0] }),
          }]} />
          <Animated.View style={[styles.ring, {
            transform: [{ scale: ring1 }],
            opacity: ring1.interpolate({ inputRange: [1, 1.55], outputRange: [0.18, 0] }),
          }]} />
          <View style={styles.avatarBorder}>
            <Avatar name={peerName} size="xl" />
          </View>
        </View>

        <Text style={styles.peerName}>{peerName}</Text>
        <Text style={[styles.statusText, callState === "active" && styles.statusActive]}>
          {statusText}
        </Text>
      </View>

      {/* Controls */}
      <View style={styles.controls}>
        <View style={styles.secRow}>
          <TouchableOpacity style={styles.secBtn} onPress={handleSwitchToVideo} activeOpacity={0.8}>
            <View style={styles.secIcon}>
              <Ionicons name="videocam-outline" size={20} color={colors.sub} />
            </View>
            <Text style={styles.secLabel}>Video'ya geç</Text>
          </TouchableOpacity>

          <TouchableOpacity style={styles.secBtn} activeOpacity={0.8}>
            <View style={styles.secIcon}>
              <Ionicons name="keypad-outline" size={20} color={colors.sub} />
            </View>
            <Text style={styles.secLabel}>Tuş takımı</Text>
          </TouchableOpacity>

          <TouchableOpacity style={styles.secBtn} activeOpacity={0.8}>
            <View style={styles.secIcon}>
              <Ionicons name="add" size={20} color={colors.sub} />
            </View>
            <Text style={styles.secLabel}>Kişi ekle</Text>
          </TouchableOpacity>
        </View>

        <View style={styles.mainRow}>
          <TouchableOpacity
            style={[styles.ctrlBtn, muted && styles.ctrlBtnOn]}
            onPress={handleToggleMute}
            activeOpacity={0.8}
          >
            <Ionicons name={muted ? "mic-off" : "mic"} size={24} color={muted ? colors.void : colors.head} />
            <Text style={[styles.ctrlLabel, muted && { color: colors.void }]}>
              {muted ? "Sesi Aç" : "Sessize Al"}
            </Text>
          </TouchableOpacity>

          <TouchableOpacity style={styles.endBtn} onPress={handleEnd} activeOpacity={0.85}>
            <Ionicons name="call" size={28} color="#fff" style={{ transform: [{ rotate: "135deg" }] }} />
          </TouchableOpacity>

          <TouchableOpacity style={styles.ctrlBtn} activeOpacity={0.8}>
            <Ionicons name="volume-medium" size={24} color={colors.head} />
            <Text style={styles.ctrlLabel}>Hoparlör</Text>
          </TouchableOpacity>
        </View>
      </View>
    </View>
  );
}

const RING_BASE = 140;

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void, alignItems: "center" },
  ambient: {
    position: "absolute",
    top: -80, alignSelf: "center",
    width: 420, height: 420, borderRadius: 210,
    backgroundColor: "rgba(74,222,128,0.05)",
  },
  header: {
    width: "100%",
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    paddingHorizontal: spacing.md, paddingVertical: spacing.sm,
  },
  iconBtn: { width: 40, height: 40, alignItems: "center", justifyContent: "center" },
  e2eTag: {
    flexDirection: "row", alignItems: "center", gap: 5,
    paddingHorizontal: 10, paddingVertical: 4,
    backgroundColor: "rgba(74,222,128,0.07)",
    borderWidth: 1, borderColor: "rgba(74,222,128,0.15)",
    borderRadius: radius.full,
  },
  e2eText: { fontSize: 11, color: colors.accent, fontWeight: "500" },
  avatarSection: { flex: 1, alignItems: "center", justifyContent: "center", gap: 14 },
  ringsWrap: {
    alignItems: "center", justifyContent: "center",
    width: RING_BASE + 120, height: RING_BASE + 120,
    marginBottom: 8,
  },
  ring: {
    position: "absolute",
    width: RING_BASE, height: RING_BASE,
    borderRadius: RING_BASE / 2,
    backgroundColor: "rgba(74,222,128,0.25)",
  },
  ringMd: { width: RING_BASE + 44, height: RING_BASE + 44, borderRadius: (RING_BASE + 44) / 2 },
  ringLg: { width: RING_BASE + 88, height: RING_BASE + 88, borderRadius: (RING_BASE + 88) / 2 },
  avatarBorder: {
    borderWidth: 3, borderColor: "rgba(74,222,128,0.3)",
    borderRadius: 46,
    shadowColor: colors.accent,
    shadowOffset: { width: 0, height: 0 },
    shadowOpacity: 0.35, shadowRadius: 20, elevation: 10,
  },
  peerName: {
    fontSize: typography.xxxl, fontWeight: "700",
    color: colors.head, letterSpacing: -1, textAlign: "center",
  },
  statusText: { fontSize: typography.base, color: colors.sub, fontWeight: "500" },
  statusActive: {
    color: colors.accent, fontWeight: "600",
    fontVariant: ["tabular-nums"] as any,
  },
  controls: { width: "100%", paddingHorizontal: spacing.xl, gap: 20, marginBottom: 8 },
  secRow: { flexDirection: "row", justifyContent: "space-around" },
  secBtn: { alignItems: "center", gap: 6 },
  secIcon: {
    width: 52, height: 52, borderRadius: 17,
    backgroundColor: colors.raised, borderWidth: 1, borderColor: colors.border,
    alignItems: "center", justifyContent: "center",
  },
  secLabel: { fontSize: 10, color: colors.dim, fontWeight: "500" },
  mainRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  ctrlBtn: {
    alignItems: "center", justifyContent: "center", gap: 6,
    width: 76, height: 76, borderRadius: 24,
    backgroundColor: colors.raised, borderWidth: 1, borderColor: colors.border,
  },
  ctrlBtnOn: { backgroundColor: colors.accent, borderColor: colors.accent },
  ctrlLabel: { fontSize: 9, color: colors.sub, fontWeight: "500", textAlign: "center" },
  endBtn: {
    width: 76, height: 76, borderRadius: 38,
    backgroundColor: colors.red,
    alignItems: "center", justifyContent: "center",
    shadowColor: colors.red,
    shadowOffset: { width: 0, height: 4 }, shadowOpacity: 0.5, shadowRadius: 14, elevation: 10,
  },
});
