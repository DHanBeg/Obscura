import React, { useState, useEffect, useRef, useReducer } from "react";
import {
  View, Text, TouchableOpacity, StyleSheet, StatusBar,
  Animated, Dimensions, Alert,
} from "react-native";
import { useLocalSearchParams, router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { LinearGradient } from "expo-linear-gradient";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { Avatar } from "@/components/ui/Avatar";
import { useStore } from "@/lib/store";
import { api } from "@/lib/api";
import * as rtcSession from "@/lib/rtcSession";

// RTCView must be lazy-imported because the package requires native rebuild
let RTCView: any = null;
try {
  // eslint-disable-next-line @typescript-eslint/no-var-requires
  RTCView = require("react-native-webrtc").RTCView;
} catch {}

const { height } = Dimensions.get("window");

export default function VideoCallScreen() {
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

  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const controlsAnim = useRef(new Animated.Value(1)).current;

  const { state, localStream, remoteStream, isMuted, isCameraOff } = rtcSession.getState();
  const callState: "connecting" | "active" | "ended" =
    state === "connected" ? "active" :
    state === "ended" ? "ended" :
    "connecting";

  useEffect(() => {
    return rtcSession.subscribe(forceUpdate);
  }, []);

  useEffect(() => {
    const { state: s } = rtcSession.getState();
    if (s === "connected" && !timerStarted.current) {
      timerStarted.current = true;
      timerRef.current = setInterval(() => setDuration((d) => d + 1), 1000);
      scheduleHide();
    }
    if (s === "ended" && !backInitiated.current) {
      backInitiated.current = true;
      if (timerRef.current) clearInterval(timerRef.current);
      setTimeout(() => router.back(), 800);
    }
  });

  // Caller: initiate video call
  useEffect(() => {
    if (isCallee) return;
    (async () => {
      try {
        const sdp = await rtcSession.initCaller("video");
        const result = await api.callInvite({
          to_did: peer,
          sdp_offer: sdp,
          call_type: "video",
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
      if (hideTimerRef.current) clearTimeout(hideTimerRef.current);
    };
  }, []);

  // Apply SDP answer (caller side)
  useEffect(() => {
    if (!pendingCallAnswer || isCallee) return;
    rtcSession.applyAnswer(pendingCallAnswer).catch(() => {});
    setPendingCallAnswer(null);
  }, [pendingCallAnswer]);

  const scheduleHide = () => {
    if (hideTimerRef.current) clearTimeout(hideTimerRef.current);
    hideTimerRef.current = setTimeout(() => {
      Animated.timing(controlsAnim, { toValue: 0, duration: 400, useNativeDriver: true }).start();
    }, 4000);
  };

  const showControls = () => {
    Animated.timing(controlsAnim, { toValue: 1, duration: 200, useNativeDriver: true }).start();
    scheduleHide();
  };

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

  const handleToggleMute = () => { rtcSession.toggleMute(); forceUpdate(); };
  const handleToggleCamera = () => { rtcSession.toggleCamera(); forceUpdate(); };

  const handleSwitchToVoice = () => {
    router.replace(`/(main)/voice-call?peer=${peer}&peerName=${encodeURIComponent(peerName)}&convId=${convId}&mode=${mode}&callId=${callIdRef.current}` as any);
  };

  const { isMuted: muted, isCameraOff: camOff, remoteStream: rStream, localStream: lStream } = rtcSession.getState();
  const remoteURL = rStream?.toURL ? rStream.toURL() : null;
  const localURL = lStream?.toURL ? lStream.toURL() : null;

  return (
    <TouchableOpacity activeOpacity={1} style={styles.container} onPress={showControls}>
      <StatusBar barStyle="light-content" hidden />

      {/* Remote video */}
      <View style={StyleSheet.absoluteFillObject}>
        {callState === "connecting" || !remoteURL || !RTCView ? (
          <View style={styles.connectingBg}>
            <Avatar name={peerName} size="xl" />
            <Text style={styles.connectingText}>
              {isCallee ? "Bağlanıyor..." : "Çağrı yapılıyor..."}
            </Text>
          </View>
        ) : (
          <>
            <LinearGradient
              colors={["#0d0d1a", "#111827", "#0a0a14"]}
              style={StyleSheet.absoluteFillObject}
            />
            <RTCView
              streamURL={remoteURL}
              style={StyleSheet.absoluteFillObject}
              objectFit="cover"
            />
          </>
        )}
      </View>

      {/* Gradient overlays */}
      <LinearGradient
        colors={["rgba(0,0,0,0.72)", "transparent"]}
        style={[styles.topGradient, { height: 160 + insets.top }]}
        pointerEvents="none"
      />
      <LinearGradient
        colors={["transparent", "rgba(0,0,0,0.78)"]}
        style={styles.bottomGradient}
        pointerEvents="none"
      />

      {/* Self-view PiP */}
      <Animated.View
        style={[styles.selfView, { top: insets.top + 58, opacity: controlsAnim }]}
        pointerEvents="none"
      >
        {camOff || !localURL || !RTCView ? (
          <View style={styles.selfCamOff}>
            <Ionicons name="videocam-off" size={18} color={colors.dim} />
          </View>
        ) : (
          <RTCView
            streamURL={localURL}
            style={StyleSheet.absoluteFillObject}
            objectFit="cover"
            mirror
          />
        )}
      </Animated.View>

      {/* Top bar */}
      <Animated.View
        style={[styles.topBar, { paddingTop: insets.top + 10, opacity: controlsAnim }]}
      >
        <TouchableOpacity onPress={handleEnd} style={styles.navBtn}>
          <Ionicons name="chevron-down" size={22} color="rgba(255,255,255,0.75)" />
        </TouchableOpacity>
        <View style={styles.topInfo}>
          <Text style={styles.topName}>{peerName}</Text>
          <View style={styles.topStatus}>
            <Ionicons name="lock-closed" size={10} color={colors.accent} />
            <Text style={styles.topStatusText}>
              {callState === "active" ? fmt(duration) : "Bağlanıyor..."}
            </Text>
          </View>
        </View>
        <View style={styles.navBtn} />
      </Animated.View>

      {/* Side buttons */}
      <Animated.View
        style={[styles.sideControls, { top: height * 0.4, opacity: controlsAnim }]}
      >
        <TouchableOpacity style={styles.sideBtn} activeOpacity={0.85}>
          <Ionicons name="camera-reverse-outline" size={20} color="#fff" />
        </TouchableOpacity>
        <TouchableOpacity style={styles.sideBtn} onPress={handleSwitchToVoice} activeOpacity={0.85}>
          <Ionicons name="call-outline" size={20} color="#fff" />
        </TouchableOpacity>
      </Animated.View>

      {/* Bottom controls */}
      <Animated.View
        style={[styles.bottomControls, { paddingBottom: insets.bottom + 30, opacity: controlsAnim }]}
      >
        <View style={styles.ctrlGroup}>
          <TouchableOpacity
            style={[styles.ctrlBtn, muted && styles.ctrlOn]}
            onPress={handleToggleMute}
            activeOpacity={0.85}
          >
            <Ionicons name={muted ? "mic-off" : "mic"} size={22} color={muted ? colors.void : "#fff"} />
          </TouchableOpacity>
          <Text style={styles.ctrlLabel}>{muted ? "Sessiz" : "Mikrofon"}</Text>
        </View>

        <View style={styles.ctrlGroup}>
          <TouchableOpacity style={styles.endBtn} onPress={handleEnd} activeOpacity={0.85}>
            <Ionicons name="call" size={26} color="#fff" style={{ transform: [{ rotate: "135deg" }] }} />
          </TouchableOpacity>
          <Text style={styles.ctrlLabel}>Kapat</Text>
        </View>

        <View style={styles.ctrlGroup}>
          <TouchableOpacity
            style={[styles.ctrlBtn, camOff && styles.ctrlOn]}
            onPress={handleToggleCamera}
            activeOpacity={0.85}
          >
            <Ionicons name={camOff ? "videocam-off" : "videocam"} size={22} color={camOff ? colors.void : "#fff"} />
          </TouchableOpacity>
          <Text style={styles.ctrlLabel}>{camOff ? "Kamera kapalı" : "Kamera"}</Text>
        </View>
      </Animated.View>
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#0a0a14" },
  connectingBg: {
    flex: 1, backgroundColor: colors.void,
    alignItems: "center", justifyContent: "center", gap: 20,
  },
  connectingText: { fontSize: typography.sm, color: colors.sub },
  topGradient: { position: "absolute", top: 0, left: 0, right: 0 },
  bottomGradient: { position: "absolute", bottom: 0, left: 0, right: 0, height: 240 },
  selfView: {
    position: "absolute",
    right: 14,
    width: 86, height: 126,
    borderRadius: 14,
    overflow: "hidden",
    borderWidth: 1.5, borderColor: "rgba(255,255,255,0.18)",
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 4 }, shadowOpacity: 0.6, shadowRadius: 8,
    elevation: 8, zIndex: 10,
  },
  selfCamOff: {
    flex: 1, backgroundColor: colors.raised,
    alignItems: "center", justifyContent: "center",
  },
  topBar: {
    position: "absolute", top: 0, left: 0, right: 0,
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    paddingHorizontal: spacing.md, paddingBottom: 16,
    zIndex: 5,
  },
  navBtn: { width: 40, height: 40, alignItems: "center", justifyContent: "center" },
  topInfo: { alignItems: "center", gap: 3 },
  topName: {
    fontSize: typography.md, fontWeight: "700", color: "#fff", letterSpacing: -0.3,
    textShadowColor: "rgba(0,0,0,0.8)", textShadowOffset: { width: 0, height: 1 }, textShadowRadius: 4,
  },
  topStatus: { flexDirection: "row", alignItems: "center", gap: 4 },
  topStatusText: {
    fontSize: 11, color: colors.accent, fontWeight: "600",
    fontVariant: ["tabular-nums"] as any,
  },
  sideControls: {
    position: "absolute", right: 14, gap: 10, zIndex: 5,
  },
  sideBtn: {
    width: 44, height: 44, borderRadius: 22,
    backgroundColor: "rgba(255,255,255,0.15)",
    alignItems: "center", justifyContent: "center",
  },
  bottomControls: {
    position: "absolute", bottom: 0, left: 0, right: 0,
    flexDirection: "row", alignItems: "flex-end", justifyContent: "space-evenly",
    paddingTop: 20, zIndex: 5,
  },
  ctrlGroup: { alignItems: "center", gap: 8 },
  ctrlBtn: {
    width: 60, height: 60, borderRadius: 30,
    backgroundColor: "rgba(255,255,255,0.18)",
    alignItems: "center", justifyContent: "center",
  },
  ctrlOn: { backgroundColor: colors.accent },
  endBtn: {
    width: 68, height: 68, borderRadius: 34,
    backgroundColor: colors.red,
    alignItems: "center", justifyContent: "center",
    shadowColor: colors.red,
    shadowOffset: { width: 0, height: 4 }, shadowOpacity: 0.6, shadowRadius: 14,
    elevation: 12,
  },
  ctrlLabel: {
    fontSize: 10, color: "rgba(255,255,255,0.6)", fontWeight: "500", textAlign: "center",
  },
});
