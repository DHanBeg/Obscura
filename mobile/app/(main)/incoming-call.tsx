import React, { useState, useEffect, useRef } from "react";
import {
  View, Text, TouchableOpacity, StyleSheet, StatusBar,
  Animated, Easing, ActivityIndicator,
} from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { Avatar } from "@/components/ui/Avatar";
import { useStore } from "@/lib/store";
import { api } from "@/lib/api";
import * as rtcSession from "@/lib/rtcSession";

export default function IncomingCallScreen() {
  const insets = useSafeAreaInsets();
  const incomingCall = useStore((s) => s.incomingCall);
  const setIncomingCall = useStore((s) => s.setIncomingCall);
  const [answering, setAnswering] = useState(false);
  const [error, setError] = useState("");
  const mounted = useRef(false);

  const ring1 = useRef(new Animated.Value(1)).current;
  const ring2 = useRef(new Animated.Value(1)).current;
  const ring3 = useRef(new Animated.Value(1)).current;
  const slideAnim = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    Animated.spring(slideAnim, { toValue: 1, useNativeDriver: true, tension: 80, friction: 10 }).start();

    const pulse = (val: Animated.Value, delay: number) =>
      Animated.loop(
        Animated.sequence([
          Animated.delay(delay),
          Animated.timing(val, { toValue: 1.6, duration: 1400, easing: Easing.out(Easing.cubic), useNativeDriver: true }),
          Animated.timing(val, { toValue: 1, duration: 0, useNativeDriver: true }),
        ])
      ).start();
    pulse(ring1, 0);
    pulse(ring2, 470);
    pulse(ring3, 940);
  }, []);

  // Auto-dismiss if incomingCall cleared by remote call_end
  useEffect(() => {
    if (!mounted.current) { mounted.current = true; return; }
    if (!incomingCall && !answering) router.back();
  }, [incomingCall]);

  const handleAnswer = async () => {
    if (!incomingCall || answering) return;
    setAnswering(true);
    setError("");
    try {
      const sdpAnswer = await rtcSession.initCallee(incomingCall.sdpOffer, incomingCall.callType);
      await api.callAnswer({
        call_id: incomingCall.callId,
        sdp_answer: sdpAnswer,
        accepted: true,
      });
      const targetScreen = incomingCall.callType === "video"
        ? `/(main)/video-call?peer=${incomingCall.callerDid}&peerName=${encodeURIComponent(incomingCall.callerName)}&convId=&mode=incoming&callId=${incomingCall.callId}`
        : `/(main)/voice-call?peer=${incomingCall.callerDid}&peerName=${encodeURIComponent(incomingCall.callerName)}&convId=&mode=incoming&callId=${incomingCall.callId}`;
      router.replace(targetScreen as any);
    } catch (e: any) {
      setAnswering(false);
      setError("Bağlanırken hata oluştu");
    }
  };

  const handleReject = async () => {
    if (!incomingCall) return;
    try {
      await api.callAnswer({ call_id: incomingCall.callId, sdp_answer: "", accepted: false });
    } catch {}
    rtcSession.markEnded();
    setIncomingCall(null);
    router.back();
  };

  if (!incomingCall) {
    return <View style={styles.container}><StatusBar barStyle="light-content" /></View>;
  }

  const isVideo = incomingCall.callType === "video";

  return (
    <Animated.View
      style={[
        styles.container,
        { paddingTop: insets.top, paddingBottom: insets.bottom + 24 },
        {
          transform: [{
            translateY: slideAnim.interpolate({ inputRange: [0, 1], outputRange: [60, 0] }),
          }],
          opacity: slideAnim,
        },
      ]}
    >
      <StatusBar barStyle="light-content" />
      <View style={styles.ambient} pointerEvents="none" />

      {/* Header */}
      <View style={styles.header}>
        <View style={styles.callTypeBadge}>
          <Ionicons
            name={isVideo ? "videocam" : "call"}
            size={12}
            color={isVideo ? "#818cf8" : colors.accent}
          />
          <Text style={[styles.callTypeText, isVideo && { color: "#818cf8" }]}>
            {isVideo ? "Görüntülü Arama" : "Sesli Arama"}
          </Text>
        </View>
        <View style={styles.e2eTag}>
          <Ionicons name="lock-closed" size={10} color={colors.accent} />
          <Text style={styles.e2eText}>Uçtan uca şifreli</Text>
        </View>
      </View>

      {/* Avatar + rings */}
      <View style={styles.heroSection}>
        <View style={styles.ringsWrap}>
          <Animated.View style={[styles.ring, styles.ringLg, {
            transform: [{ scale: ring3 }],
            opacity: ring3.interpolate({ inputRange: [1, 1.6], outputRange: [0.07, 0] }),
          }]} />
          <Animated.View style={[styles.ring, styles.ringMd, {
            transform: [{ scale: ring2 }],
            opacity: ring2.interpolate({ inputRange: [1, 1.6], outputRange: [0.12, 0] }),
          }]} />
          <Animated.View style={[styles.ring, {
            transform: [{ scale: ring1 }],
            opacity: ring1.interpolate({ inputRange: [1, 1.6], outputRange: [0.18, 0] }),
          }]} />
          <View style={styles.avatarBorder}>
            <Avatar name={incomingCall.callerName} size="xl" />
          </View>
        </View>

        <Text style={styles.callerName}>{incomingCall.callerName}</Text>
        <Text style={styles.callStatus}>
          {answering ? "Bağlanıyor..." : isVideo ? "Görüntülü arama geliyor" : "Sesli arama geliyor"}
        </Text>
        {!!error && <Text style={styles.errorText}>{error}</Text>}
      </View>

      {/* Action buttons */}
      <View style={styles.actions}>
        <View style={styles.actionGroup}>
          <TouchableOpacity
            style={styles.rejectBtn}
            onPress={handleReject}
            activeOpacity={0.85}
            disabled={answering}
          >
            <Ionicons name="call" size={28} color="#fff" style={{ transform: [{ rotate: "135deg" }] }} />
          </TouchableOpacity>
          <Text style={styles.actionLabel}>Reddet</Text>
        </View>

        <View style={styles.actionGroup}>
          {answering ? (
            <View style={[styles.answerBtn, { justifyContent: "center", alignItems: "center" }]}>
              <ActivityIndicator color="#fff" />
            </View>
          ) : (
            <TouchableOpacity
              style={styles.answerBtn}
              onPress={handleAnswer}
              activeOpacity={0.85}
            >
              <Ionicons
                name={isVideo ? "videocam" : "call"}
                size={28}
                color="#fff"
              />
            </TouchableOpacity>
          )}
          <Text style={styles.actionLabel}>Cevapla</Text>
        </View>
      </View>
    </Animated.View>
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
    marginTop: 8,
  },
  callTypeBadge: {
    flexDirection: "row", alignItems: "center", gap: 5,
    paddingHorizontal: 10, paddingVertical: 4,
    backgroundColor: "rgba(74,222,128,0.08)",
    borderWidth: 1, borderColor: "rgba(74,222,128,0.15)",
    borderRadius: radius.full,
  },
  callTypeText: { fontSize: 11, color: colors.accent, fontWeight: "600" },
  e2eTag: {
    flexDirection: "row", alignItems: "center", gap: 5,
    paddingHorizontal: 10, paddingVertical: 4,
    backgroundColor: "rgba(74,222,128,0.07)",
    borderWidth: 1, borderColor: "rgba(74,222,128,0.12)",
    borderRadius: radius.full,
  },
  e2eText: { fontSize: 10, color: colors.dim, fontWeight: "500" },
  heroSection: {
    flex: 1, alignItems: "center", justifyContent: "center", gap: 14,
  },
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
    borderWidth: 3, borderColor: "rgba(74,222,128,0.3)", borderRadius: 46,
    shadowColor: colors.accent,
    shadowOffset: { width: 0, height: 0 },
    shadowOpacity: 0.35, shadowRadius: 20, elevation: 10,
  },
  callerName: {
    fontSize: typography.xxxl, fontWeight: "700",
    color: colors.head, letterSpacing: -1, textAlign: "center",
  },
  callStatus: {
    fontSize: typography.base, color: colors.sub, fontWeight: "500",
  },
  errorText: {
    fontSize: typography.sm, color: colors.red,
    marginTop: 4, textAlign: "center",
  },
  actions: {
    flexDirection: "row", justifyContent: "space-evenly",
    width: "100%", paddingHorizontal: spacing.xl, marginBottom: 8,
  },
  actionGroup: { alignItems: "center", gap: 10 },
  rejectBtn: {
    width: 76, height: 76, borderRadius: 38,
    backgroundColor: colors.red,
    alignItems: "center", justifyContent: "center",
    shadowColor: colors.red,
    shadowOffset: { width: 0, height: 4 }, shadowOpacity: 0.5, shadowRadius: 14, elevation: 10,
  },
  answerBtn: {
    width: 76, height: 76, borderRadius: 38,
    backgroundColor: colors.accent,
    alignItems: "center", justifyContent: "center",
    shadowColor: colors.accent,
    shadowOffset: { width: 0, height: 4 }, shadowOpacity: 0.5, shadowRadius: 14, elevation: 10,
  },
  actionLabel: {
    fontSize: 12, color: colors.sub, fontWeight: "500", textAlign: "center",
  },
});
