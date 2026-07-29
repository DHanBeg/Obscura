import React, { useState, useCallback, useRef } from "react";
import {
  View,
  Text,
  StyleSheet,
  TouchableOpacity,
  ActivityIndicator,
  Alert,
  ScrollView,
  Platform,
  Animated,
  Easing,
} from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";
import { deriveOdiFromDid } from "@/lib/odi-display";

// ── NFC availability stub ─────────────────────────────────────────────────────
// react-native-nfc-manager is not in package.json. We expose a minimal
// platform-gated interface so the screen compiles and degrades gracefully on
// simulators / web.

type NfcStub = {
  isSupported: () => Promise<boolean>;
  start: () => Promise<void>;
  stop: () => Promise<void>;
  readTag: () => Promise<{ ndefMessage?: Array<{ payload: number[] }> }>;
  writeTag: (payload: string) => Promise<void>;
};

function getNfcManager(): NfcStub | null {
  if (Platform.OS === "web") return null;
  try {
    // eslint-disable-next-line @typescript-eslint/no-var-requires
    const NfcManager = require("react-native-nfc-manager").default;
    return NfcManager as NfcStub;
  } catch {
    return null;
  }
}

const nfcManager = getNfcManager();

// ── Tab types ─────────────────────────────────────────────────────────────────
type Tab = "checkin" | "pair";

// ── Utilities ────────────────────────────────────────────────────────────────
function decodeNdefPayload(payload: number[]): string {
  // NDEF Text record: skip status byte + language code
  try {
    const statusByte = payload[0];
    const langLen = statusByte & 0x3f;
    const text = payload
      .slice(1 + langLen)
      .map((b) => String.fromCharCode(b))
      .join("");
    return text;
  } catch {
    return payload.map((b) => String.fromCharCode(b)).join("");
  }
}

// ── Screen ────────────────────────────────────────────────────────────────────
export default function NFCScreen() {
  const insets = useSafeAreaInsets();
  const [activeTab, setActiveTab] = useState<Tab>("checkin");

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      {/* Header */}
      <View style={styles.header}>
        <Text style={styles.title} accessibilityRole="header">
          NFC
        </Text>
        <Text style={styles.subtitle}>Dokunarak bağlan</Text>
      </View>

      {/* Tab bar */}
      <View style={styles.tabRow} accessibilityRole="tablist">
        <TouchableOpacity
          style={[styles.tab, activeTab === "checkin" && styles.tabActive]}
          onPress={() => setActiveTab("checkin")}
          accessibilityRole="tab"
          accessibilityState={{ selected: activeTab === "checkin" }}
          accessibilityLabel="Check-in sekmesi"
        >
          <Ionicons
            name="scan-outline"
            size={14}
            color={activeTab === "checkin" ? colors.accent : colors.sub}
          />
          <Text
            style={[
              styles.tabLabel,
              activeTab === "checkin" && styles.tabLabelActive,
            ]}
          >
            Check-in
          </Text>
        </TouchableOpacity>
        <TouchableOpacity
          style={[styles.tab, activeTab === "pair" && styles.tabActive]}
          onPress={() => setActiveTab("pair")}
          accessibilityRole="tab"
          accessibilityState={{ selected: activeTab === "pair" }}
          accessibilityLabel="Cihaz eşleştirme sekmesi"
        >
          <Ionicons
            name="link-outline"
            size={14}
            color={activeTab === "pair" ? colors.accent : colors.sub}
          />
          <Text
            style={[
              styles.tabLabel,
              activeTab === "pair" && styles.tabLabelActive,
            ]}
          >
            Eşleştir
          </Text>
        </TouchableOpacity>
      </View>

      {/* Tab content */}
      {activeTab === "checkin" ? <CheckinTab /> : <PairTab />}
    </View>
  );
}

// ── Check-in Tab ─────────────────────────────────────────────────────────────
function CheckinTab() {
  const [scanning, setScanning] = useState(false);
  const [result, setResult] = useState<{
    eventId: string;
    status: "success" | "error";
    message: string;
  } | null>(null);
  const pulseAnim = useRef(new Animated.Value(1)).current;
  const pulseLoop = useRef<Animated.CompositeAnimation | null>(null);

  const startPulse = useCallback(() => {
    pulseAnim.setValue(1);
    pulseLoop.current = Animated.loop(
      Animated.sequence([
        Animated.timing(pulseAnim, {
          toValue: 1.18,
          duration: 800,
          easing: Easing.inOut(Easing.ease),
          useNativeDriver: true,
        }),
        Animated.timing(pulseAnim, {
          toValue: 1,
          duration: 800,
          easing: Easing.inOut(Easing.ease),
          useNativeDriver: true,
        }),
      ])
    );
    pulseLoop.current.start();
  }, [pulseAnim]);

  const stopPulse = useCallback(() => {
    pulseLoop.current?.stop();
    pulseAnim.setValue(1);
  }, [pulseAnim]);

  const handleScan = useCallback(async () => {
    if (Platform.OS === "web") {
      Alert.alert("Desteklenmiyor", "NFC web tarayıcısında kullanılamaz.");
      return;
    }

    if (!nfcManager) {
      Alert.alert(
        "NFC Kullanılamıyor",
        Platform.OS === "ios"
          ? "Bu cihaz NFC Core Session desteklemiyor veya modül yüklü değil."
          : "NFC adaptörü bulunamadı veya modül yüklü değil."
      );
      return;
    }

    setResult(null);
    setScanning(true);
    startPulse();

    try {
      const supported = await nfcManager.isSupported();
      if (!supported) {
        Alert.alert("NFC Desteklenmiyor", "Cihazınız NFC desteklemiyor.");
        return;
      }

      await nfcManager.start();
      const tag = await nfcManager.readTag();

      let eventId = "";
      if (tag.ndefMessage && tag.ndefMessage.length > 0) {
        const raw = decodeNdefPayload(tag.ndefMessage[0].payload);
        // Expected format: "obscura:event:<id>" or just "<id>"
        if (raw.startsWith("obscura:event:")) {
          eventId = raw.replace("obscura:event:", "").trim();
        } else {
          eventId = raw.trim();
        }
      }

      if (!eventId) {
        setResult({
          eventId: "",
          status: "error",
          message: "NFC tag'dan etkinlik ID okunamadı.",
        });
        return;
      }

      await api.nfcCheckin(eventId);
      setResult({
        eventId,
        status: "success",
        message: "Check-in başarılı!",
      });
    } catch (e: any) {
      const msg: string = e?.message ?? "Bilinmeyen hata";
      if (msg.includes("503") || msg.toLowerCase().includes("trusted")) {
        setResult({
          eventId: "",
          status: "error",
          message: "Etkinlik servisi şu an kullanılamıyor.",
        });
      } else {
        setResult({ eventId: "", status: "error", message: msg });
      }
    } finally {
      stopPulse();
      setScanning(false);
      try {
        await nfcManager?.stop();
      } catch {}
    }
  }, [startPulse, stopPulse]);

  return (
    <ScrollView
      contentContainerStyle={styles.tabContent}
      showsVerticalScrollIndicator={false}
    >
      {/* Platform guard banner */}
      {Platform.OS === "web" && (
        <View style={styles.warnBanner}>
          <Ionicons name="warning-outline" size={16} color={colors.amber} />
          <Text style={styles.warnText}>NFC bu platformda desteklenmiyor.</Text>
        </View>
      )}

      {/* NFC ring animation */}
      <View
        style={styles.nfcRingWrap}
        accessibilityLabel="NFC tarama alanı"
        accessibilityRole="image"
      >
        <Animated.View
          style={[styles.nfcRingOuter, { transform: [{ scale: pulseAnim }] }]}
        />
        <View style={styles.nfcRingMiddle} />
        <View style={styles.nfcRingInner}>
          {scanning ? (
            <ActivityIndicator size="large" color={colors.accent} />
          ) : (
            <Ionicons
              name="radio-outline"
              size={48}
              color={result?.status === "success" ? colors.accent : colors.sub}
            />
          )}
        </View>
      </View>

      <Text style={styles.nfcHint}>
        {scanning
          ? "Telefonu NFC tag'e yaklaştır…"
          : "Etkinlik check-in için NFC tag'e dokunun"}
      </Text>

      {/* Scan button */}
      <TouchableOpacity
        style={[styles.primaryBtn, scanning && styles.primaryBtnBusy]}
        onPress={handleScan}
        disabled={scanning}
        accessibilityLabel={scanning ? "Taranıyor" : "NFC ile tara"}
        accessibilityRole="button"
      >
        {scanning ? (
          <ActivityIndicator size="small" color={colors.void} />
        ) : (
          <>
            <Ionicons name="scan" size={18} color={colors.void} />
            <Text style={styles.primaryBtnText}>NFC ile Tara</Text>
          </>
        )}
      </TouchableOpacity>

      {/* Result card */}
      {result && (
        <View
          style={[
            styles.resultCard,
            result.status === "success"
              ? styles.resultSuccess
              : styles.resultError,
          ]}
          accessibilityLiveRegion="polite"
        >
          <Ionicons
            name={
              result.status === "success"
                ? "checkmark-circle-outline"
                : "close-circle-outline"
            }
            size={22}
            color={
              result.status === "success" ? colors.accent : colors.red
            }
          />
          <View style={{ flex: 1 }}>
            <Text
              style={[
                styles.resultTitle,
                { color: result.status === "success" ? colors.accent : colors.red },
              ]}
            >
              {result.status === "success" ? "Check-in Başarılı" : "Hata"}
            </Text>
            <Text style={styles.resultMsg}>{result.message}</Text>
            {result.eventId ? (
              <Text style={styles.resultSub}>Etkinlik: {result.eventId}</Text>
            ) : null}
          </View>
        </View>
      )}

      {/* Info box */}
      <View style={styles.infoCard}>
        <Ionicons name="information-circle-outline" size={16} color={colors.sub} />
        <Text style={styles.infoText}>
          {Platform.OS === "ios"
            ? "iOS: NFC Core Session — iPhone 7 ve üzeri gereklidir."
            : Platform.OS === "android"
            ? "Android: NFC Adapter — Cihaz ayarlarından NFC'nin açık olduğunu doğrulayın."
            : "NFC yalnızca iOS ve Android cihazlarda kullanılabilir."}
        </Text>
      </View>
    </ScrollView>
  );
}

// ── Pair Tab ─────────────────────────────────────────────────────────────────
type PairStep = "idle" | "waiting" | "reading" | "writing" | "done" | "error";

function PairTab() {
  const [step, setStep] = useState<PairStep>("idle");
  const [peerDID, setPeerDID] = useState<string | null>(null);
  const [errorMsg, setErrorMsg] = useState<string>("");
  const pulseAnim = useRef(new Animated.Value(1)).current;
  const pulseLoop = useRef<Animated.CompositeAnimation | null>(null);

  const startPulse = useCallback(() => {
    pulseAnim.setValue(1);
    pulseLoop.current = Animated.loop(
      Animated.sequence([
        Animated.timing(pulseAnim, {
          toValue: 1.14,
          duration: 700,
          easing: Easing.inOut(Easing.quad),
          useNativeDriver: true,
        }),
        Animated.timing(pulseAnim, {
          toValue: 1,
          duration: 700,
          easing: Easing.inOut(Easing.quad),
          useNativeDriver: true,
        }),
      ])
    );
    pulseLoop.current.start();
  }, [pulseAnim]);

  const stopPulse = useCallback(() => {
    pulseLoop.current?.stop();
    pulseAnim.setValue(1);
  }, [pulseAnim]);

  const handlePair = useCallback(async () => {
    if (Platform.OS === "web") {
      Alert.alert("Desteklenmiyor", "NFC web tarayıcısında kullanılamaz.");
      return;
    }
    if (!nfcManager) {
      Alert.alert(
        "NFC Kullanılamıyor",
        "react-native-nfc-manager modülü yüklü değil."
      );
      return;
    }

    setStep("waiting");
    setPeerDID(null);
    setErrorMsg("");
    startPulse();

    try {
      const supported = await nfcManager.isSupported();
      if (!supported) {
        throw new Error("Bu cihaz NFC desteklemiyor.");
      }
      await nfcManager.start();

      // Step 1: read peer DID from tag
      setStep("reading");
      const tag = await nfcManager.readTag();
      let did = "";
      if (tag.ndefMessage && tag.ndefMessage.length > 0) {
        const raw = decodeNdefPayload(tag.ndefMessage[0].payload);
        if (raw.startsWith("did:obs:")) {
          did = raw.trim();
        }
      }
      if (!did) {
        throw new Error("Geçerli bir DID okunamadı.");
      }
      setPeerDID(did);

      // Step 2: write our own DID back (cross-signing stub)
      // In a real implementation this would use Android Beam / iOS NFCNDEFReaderSession
      setStep("writing");
      // Stub: real DID yazma burada gerçekleşir
      // await nfcManager.writeTag(`did:obs:local_did_placeholder`);

      // Cross-signing başlatma stub
      // In production: initiate cross-signing handshake with peerDID
      await new Promise<void>((resolve) => setTimeout(resolve, 800));

      setStep("done");
    } catch (e: any) {
      setErrorMsg(e?.message ?? "Bilinmeyen hata");
      setStep("error");
    } finally {
      stopPulse();
      try {
        await nfcManager?.stop();
      } catch {}
    }
  }, [startPulse, stopPulse]);

  const reset = () => {
    setStep("idle");
    setPeerDID(null);
    setErrorMsg("");
  };

  const STEP_LABELS: Record<PairStep, string> = {
    idle: "Eşleştirme başlatmak için butona bas",
    waiting: "Karşı cihazı yaklaştır…",
    reading: "DID okunuyor…",
    writing: "DID yazılıyor…",
    done: "Eşleştirme tamamlandı",
    error: "Bir hata oluştu",
  };

  return (
    <ScrollView
      contentContainerStyle={styles.tabContent}
      showsVerticalScrollIndicator={false}
    >
      {Platform.OS === "web" && (
        <View style={styles.warnBanner}>
          <Ionicons name="warning-outline" size={16} color={colors.amber} />
          <Text style={styles.warnText}>NFC bu platformda desteklenmiyor.</Text>
        </View>
      )}

      {/* Animated NFC ring */}
      <View
        style={styles.nfcRingWrap}
        accessibilityLabel="NFC eşleştirme alanı"
        accessibilityRole="image"
      >
        <Animated.View
          style={[
            styles.nfcRingOuter,
            step === "done" && styles.nfcRingDone,
            { transform: [{ scale: pulseAnim }] },
          ]}
        />
        <View
          style={[
            styles.nfcRingMiddle,
            step === "done" && styles.nfcRingMiddleDone,
          ]}
        />
        <View style={styles.nfcRingInner}>
          {step === "waiting" || step === "reading" || step === "writing" ? (
            <ActivityIndicator size="large" color={colors.accent} />
          ) : step === "done" ? (
            <Ionicons name="checkmark-circle" size={48} color={colors.accent} />
          ) : step === "error" ? (
            <Ionicons name="close-circle" size={48} color={colors.red} />
          ) : (
            <Ionicons name="link-outline" size={48} color={colors.sub} />
          )}
        </View>
      </View>

      <Text style={styles.nfcHint}>{STEP_LABELS[step]}</Text>

      {/* Action button */}
      {step === "idle" || step === "error" ? (
        <TouchableOpacity
          style={styles.primaryBtn}
          onPress={step === "error" ? reset : handlePair}
          accessibilityLabel={step === "error" ? "Tekrar dene" : "NFC ile eşleştir"}
          accessibilityRole="button"
        >
          <Ionicons
            name={step === "error" ? "refresh" : "link"}
            size={18}
            color={colors.void}
          />
          <Text style={styles.primaryBtnText}>
            {step === "error" ? "Tekrar Dene" : "NFC ile Eşleştir"}
          </Text>
        </TouchableOpacity>
      ) : step === "done" ? (
        <TouchableOpacity
          style={[styles.primaryBtn, styles.primaryBtnDone]}
          onPress={reset}
          accessibilityLabel="Yeni eşleştirme başlat"
          accessibilityRole="button"
        >
          <Ionicons name="add-circle-outline" size={18} color={colors.void} />
          <Text style={styles.primaryBtnText}>Yeni Eşleştirme</Text>
        </TouchableOpacity>
      ) : null}

      {/* Error card */}
      {step === "error" && errorMsg ? (
        <View style={[styles.resultCard, styles.resultError]}>
          <Ionicons name="close-circle-outline" size={22} color={colors.red} />
          <Text style={[styles.resultMsg, { color: colors.red }]}>{errorMsg}</Text>
        </View>
      ) : null}

      {/* Done — peer info */}
      {step === "done" && peerDID && (
        <View style={[styles.resultCard, styles.resultSuccess]}>
          <Ionicons
            name="checkmark-circle-outline"
            size={22}
            color={colors.accent}
          />
          <View style={{ flex: 1 }}>
            <Text style={[styles.resultTitle, { color: colors.accent }]}>
              Cihaz Tanındı
            </Text>
            <Text style={styles.resultSub} numberOfLines={2}>
              {deriveOdiFromDid(peerDID)}
            </Text>
            <Text style={[styles.resultMsg, { marginTop: 4 }]}>
              Cross-signing el sıkışması başlatılıyor…
            </Text>
          </View>
        </View>
      )}

      {/* How it works */}
      <View style={styles.infoCard}>
        <Ionicons name="information-circle-outline" size={16} color={colors.sub} />
        <Text style={styles.infoText}>
          İki cihaz NFC ile dokunduğunda kimlikler karşılıklı okunur (ekranda
          ODI olarak gösterilir) ve cross-signing prosedürü başlatılır. Onay
          ekranı açılır.
        </Text>
      </View>
    </ScrollView>
  );
}

// ── Styles ────────────────────────────────────────────────────────────────────
const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.void,
  },
  header: {
    paddingHorizontal: spacing.md,
    paddingTop: spacing.sm,
    paddingBottom: spacing.sm,
  },
  title: {
    fontSize: typography.xxl,
    fontWeight: "700",
    color: colors.head,
  },
  subtitle: {
    fontSize: typography.xs,
    color: colors.sub,
    marginTop: 2,
  },
  // ── Tabs ──
  tabRow: {
    flexDirection: "row",
    marginHorizontal: spacing.md,
    marginBottom: spacing.md,
    backgroundColor: colors.surface,
    borderRadius: radius.lg,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 4,
    gap: 4,
  },
  tab: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 6,
    height: 36,
    borderRadius: radius.md,
  },
  tabActive: {
    backgroundColor: colors.raised,
  },
  tabLabel: {
    fontSize: typography.sm,
    fontWeight: "600",
    color: colors.sub,
  },
  tabLabelActive: {
    color: colors.accent,
  },
  // ── Tab content ──
  tabContent: {
    paddingHorizontal: spacing.md,
    paddingBottom: 120,
    alignItems: "center",
  },
  warnBanner: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.xs,
    backgroundColor: colors.amber + "18",
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: colors.amber + "40",
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
    marginBottom: spacing.md,
    alignSelf: "stretch",
  },
  warnText: {
    fontSize: typography.sm,
    color: colors.amber,
    flex: 1,
  },
  // ── NFC ring ──
  nfcRingWrap: {
    width: 200,
    height: 200,
    alignItems: "center",
    justifyContent: "center",
    marginVertical: spacing.xl,
  },
  nfcRingOuter: {
    position: "absolute",
    width: 200,
    height: 200,
    borderRadius: 100,
    borderWidth: 1,
    borderColor: colors.accent + "20",
    backgroundColor: colors.accent + "05",
  },
  nfcRingDone: {
    borderColor: colors.accent + "40",
    backgroundColor: colors.accent + "10",
  },
  nfcRingMiddle: {
    position: "absolute",
    width: 148,
    height: 148,
    borderRadius: 74,
    borderWidth: 1,
    borderColor: colors.accent + "30",
    backgroundColor: colors.accent + "08",
  },
  nfcRingMiddleDone: {
    borderColor: colors.accent + "50",
  },
  nfcRingInner: {
    width: 96,
    height: 96,
    borderRadius: 48,
    borderWidth: 1,
    borderColor: colors.border,
    backgroundColor: colors.surface,
    alignItems: "center",
    justifyContent: "center",
  },
  nfcHint: {
    fontSize: typography.base,
    color: colors.sub,
    textAlign: "center",
    marginBottom: spacing.lg,
    lineHeight: 22,
    paddingHorizontal: spacing.lg,
  },
  // ── Buttons ──
  primaryBtn: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: spacing.sm,
    height: 52,
    borderRadius: radius.lg,
    backgroundColor: colors.accent,
    alignSelf: "stretch",
    marginBottom: spacing.md,
  },
  primaryBtnBusy: {
    opacity: 0.6,
  },
  primaryBtnDone: {
    backgroundColor: colors.accentDim,
  },
  primaryBtnText: {
    fontSize: typography.base,
    fontWeight: "700",
    color: colors.void,
  },
  // ── Result card ──
  resultCard: {
    flexDirection: "row",
    alignItems: "flex-start",
    gap: spacing.sm,
    borderRadius: radius.lg,
    borderWidth: 1,
    padding: spacing.md,
    alignSelf: "stretch",
    marginBottom: spacing.md,
  },
  resultSuccess: {
    backgroundColor: colors.accent + "12",
    borderColor: colors.accent + "40",
  },
  resultError: {
    backgroundColor: colors.red + "12",
    borderColor: colors.red + "40",
  },
  resultTitle: {
    fontSize: typography.base,
    fontWeight: "700",
    marginBottom: 2,
  },
  resultMsg: {
    fontSize: typography.sm,
    color: colors.body,
    lineHeight: 18,
  },
  resultSub: {
    fontSize: typography.xs,
    color: colors.sub,
    fontFamily: "Courier",
    marginTop: 2,
  },
  // ── Info card ──
  infoCard: {
    flexDirection: "row",
    alignItems: "flex-start",
    gap: spacing.sm,
    backgroundColor: colors.surface,
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: colors.border,
    padding: spacing.md,
    alignSelf: "stretch",
  },
  infoText: {
    fontSize: typography.sm,
    color: colors.sub,
    flex: 1,
    lineHeight: 18,
  },
});
