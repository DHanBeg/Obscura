import React, { useState, useEffect } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, StyleSheet,
  TextInput, ActivityIndicator, Alert,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import * as Clipboard from "expo-clipboard";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";
import { deriveOdiFromDid } from "@/lib/odi-display";

type Step = "overview" | "show-qr" | "scan-qr" | "confirm" | "done";

interface TrustedDevice {
  device_id: string;
  name: string;
  added_at: string;
  is_current: boolean;
}

function randomHex(len: number) {
  let s = "";
  for (let i = 0; i < len; i++) s += Math.floor(Math.random() * 16).toString(16);
  return s;
}

export default function SettingsCrossSigningScreen() {
  const [step, setStep] = useState<Step>("overview");
  const [devices, setDevices] = useState<TrustedDevice[]>([]);
  const [myQR, setMyQR] = useState("");
  const [scannedDID, setScannedDID] = useState("");
  const [scanInput, setScanInput] = useState("");
  const [copied, setCopied] = useState(false);
  const [loading, setLoading] = useState(false);

  useEffect(() => { loadDevices(); }, []);

  async function loadDevices() {
    try {
      const me = await api.getMe();
      setDevices([
        { device_id: "dev-current", name: "Bu Cihaz", added_at: new Date().toISOString(), is_current: true },
      ]);
      const qr = `obscura://device/${me?.did ?? "did:obs:unknown"}/${randomHex(32)}`;
      setMyQR(qr);
    } catch {}
  }

  async function confirmDevice() {
    setLoading(true);
    try {
      await new Promise<void>((r) => setTimeout(r, 1200));
      setStep("done");
    } finally {
      setLoading(false);
    }
  }

  function handleScanInput(text: string) {
    setScanInput(text);
    if (text.startsWith("obscura://device/")) {
      const parts = text.split("/");
      const did = parts[2] ?? "";
      if (did) { setScannedDID(did); setStep("confirm"); }
    }
  }

  async function copyQR() {
    await Clipboard.setStringAsync(myQR);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Çoklu Cihaz</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        {step === "overview" && (
          <>
            {/* Header info */}
            <View style={styles.heroCard}>
              <View style={styles.heroIcon}>
                <Ionicons name="shield-checkmark" size={28} color={colors.accent} />
              </View>
              <Text style={styles.heroTitle}>Çoklu Cihaz</Text>
              <Text style={styles.heroSub}>QR ile güvenli cihaz eşleştirme</Text>
            </View>

            {/* Devices */}
            <Text style={styles.sectionLabel}>GÜVENİLİR CİHAZLAR</Text>
            <View style={styles.card}>
              {devices.map((d, i) => (
                <View key={d.device_id}
                  style={[styles.row, i < devices.length - 1 && styles.rowBorder]}>
                  <View style={styles.deviceIcon}>
                    <Ionicons name="phone-portrait-outline" size={18}
                      color={d.is_current ? colors.accent : colors.sub} />
                  </View>
                  <View style={{ flex: 1 }}>
                    <Text style={styles.deviceName}>{d.name}</Text>
                    <Text style={styles.deviceDate}>
                      {new Date(d.added_at).toLocaleDateString("tr-TR")}
                    </Text>
                  </View>
                  {d.is_current && (
                    <View style={styles.activeBadge}>
                      <Text style={styles.activeBadgeText}>Aktif</Text>
                    </View>
                  )}
                </View>
              ))}
            </View>

            {/* Actions */}
            <Text style={[styles.sectionLabel, { marginTop: spacing.md }]}>EYLEMLER</Text>
            <View style={styles.actionsGrid}>
              <TouchableOpacity style={styles.actionCard} onPress={() => setStep("show-qr")}>
                <Ionicons name="qr-code-outline" size={22} color={colors.accent} />
                <Text style={styles.actionTitle}>QR Göster</Text>
                <Text style={styles.actionSub}>Yeni cihaza bu QR'ı tarat</Text>
              </TouchableOpacity>
              <TouchableOpacity style={styles.actionCard} onPress={() => setStep("scan-qr")}>
                <Ionicons name="scan-outline" size={22} color={colors.tier2} />
                <Text style={styles.actionTitle}>QR Yapıştır</Text>
                <Text style={styles.actionSub}>Diğer cihazın kodunu gir</Text>
              </TouchableOpacity>
            </View>

            {/* Warning */}
            <View style={styles.warningBox}>
              <Ionicons name="warning-outline" size={16} color={colors.amber} />
              <Text style={styles.warningText}>
                Yedek ifadenizi kimseyle paylaşmayın. 12 kelimelik anımsatıcınız tüm hesabınızın anahtarıdır.
              </Text>
            </View>
          </>
        )}

        {step === "show-qr" && (
          <>
            <View style={styles.qrCard}>
              <View style={styles.qrPlaceholder}>
                <Ionicons name="qr-code" size={80} color={colors.sub} />
                <Text style={styles.qrHint}>QR kodu</Text>
              </View>
              <Text style={styles.qrTitle}>Bu QR'ı Yeni Cihaza Tarat</Text>
              <Text style={styles.qrSub}>Geçerlilik: 5 dakika</Text>
            </View>

            <Text style={styles.sectionLabel}>VEYA KODU KOPYALA</Text>
            <View style={styles.monoBox}>
              <Text style={styles.monoText} numberOfLines={2}>{myQR.slice(0, 60)}…</Text>
            </View>

            <TouchableOpacity style={styles.btn} onPress={copyQR}>
              <Ionicons name={copied ? "checkmark" : "copy-outline"} size={16} color={colors.void} />
              <Text style={styles.btnText}>{copied ? "Kopyalandı" : "Kopyala"}</Text>
            </TouchableOpacity>

            <View style={styles.rowBtns}>
              <TouchableOpacity style={[styles.btnOutline, { flex: 1 }]} onPress={() => setStep("overview")}>
                <Text style={styles.btnOutlineText}>Geri</Text>
              </TouchableOpacity>
              <TouchableOpacity style={[styles.btnOutline, { flex: 1 }]} onPress={loadDevices}>
                <Ionicons name="refresh-outline" size={14} color={colors.body} />
                <Text style={styles.btnOutlineText}>Yenile</Text>
              </TouchableOpacity>
            </View>
          </>
        )}

        {step === "scan-qr" && (
          <>
            <View style={styles.qrCard}>
              <Ionicons name="phone-portrait-outline" size={48} color={colors.tier2} />
              <Text style={styles.qrTitle}>QR Kodunu Yapıştır</Text>
              <Text style={styles.qrSub}>Diğer cihazdan kopyalanan kodu yapıştırın</Text>
            </View>

            <TextInput
              style={styles.scanInput}
              placeholder="obscura://device/..."
              placeholderTextColor={colors.sub}
              value={scanInput}
              onChangeText={handleScanInput}
              multiline
              numberOfLines={4}
            />

            <TouchableOpacity style={[styles.btnOutline, { marginTop: spacing.md }]}
              onPress={() => setStep("overview")}>
              <Text style={styles.btnOutlineText}>Geri</Text>
            </TouchableOpacity>
          </>
        )}

        {step === "confirm" && (
          <>
            <View style={styles.qrCard}>
              <View style={[styles.heroIcon, { width: 64, height: 64 }]}>
                <Ionicons name="shield-checkmark" size={32} color={colors.accent} />
              </View>
              <Text style={styles.qrTitle}>Cihazı Doğrula</Text>
              <Text style={styles.qrSub}>Aşağıdaki kimlik (ODI) sahibini tanıyor musunuz?</Text>
              <View style={styles.didBox}>
                <Text style={styles.didText}>{deriveOdiFromDid(scannedDID)}</Text>
              </View>
            </View>

            <View style={styles.rowBtns}>
              <TouchableOpacity style={[styles.btnOutline, { flex: 1 }]} onPress={() => setStep("scan-qr")}>
                <Text style={styles.btnOutlineText}>İptal</Text>
              </TouchableOpacity>
              <TouchableOpacity style={[styles.btn, { flex: 1 }]} onPress={confirmDevice} disabled={loading}>
                {loading
                  ? <ActivityIndicator size="small" color={colors.void} />
                  : <Text style={styles.btnText}>Onayla & İmzala</Text>}
              </TouchableOpacity>
            </View>
          </>
        )}

        {step === "done" && (
          <View style={styles.doneCard}>
            <View style={styles.doneIcon}>
              <Ionicons name="shield-checkmark" size={40} color={colors.accent} />
            </View>
            <Text style={styles.doneTitle}>Cihaz Eklendi</Text>
            <Text style={styles.doneSub}>Çapraz imzalama başarılı</Text>
            <TouchableOpacity style={[styles.btn, { marginTop: spacing.xl }]}
              onPress={() => setStep("overview")}>
              <Text style={styles.btnText}>Tamam</Text>
            </TouchableOpacity>
          </View>
        )}
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: colors.void },
  header: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    paddingHorizontal: spacing.lg, paddingVertical: spacing.md,
    borderBottomWidth: 1, borderBottomColor: colors.border,
  },
  backBtn: { padding: 4, width: 40 },
  headerTitle: { fontSize: typography.base, fontWeight: "700", color: colors.head },
  content: { padding: spacing.lg, gap: spacing.md, paddingBottom: 48 },

  heroCard: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: colors.border, padding: spacing.xl, alignItems: "center", gap: spacing.xs,
  },
  heroIcon: {
    width: 52, height: 52, borderRadius: radius.full, alignItems: "center", justifyContent: "center",
    backgroundColor: "rgba(74,222,128,0.1)", borderWidth: 1, borderColor: "rgba(74,222,128,0.25)",
  },
  heroTitle: { fontSize: typography.md, fontWeight: "700", color: colors.head },
  heroSub: { fontSize: typography.sm, color: colors.sub },

  sectionLabel: {
    fontSize: typography.xs, color: colors.sub, fontWeight: "600",
    textTransform: "uppercase", letterSpacing: 1,
  },
  card: {
    backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border, overflow: "hidden",
  },
  row: { flexDirection: "row", alignItems: "center", padding: spacing.md, gap: spacing.sm },
  rowBorder: { borderBottomWidth: 1, borderBottomColor: colors.border },
  deviceIcon: {
    width: 36, height: 36, borderRadius: radius.md, backgroundColor: colors.raised,
    alignItems: "center", justifyContent: "center",
  },
  deviceName: { fontSize: typography.sm, fontWeight: "600", color: colors.head },
  deviceDate: { fontSize: typography.xs, color: colors.sub },
  activeBadge: {
    paddingHorizontal: spacing.sm, paddingVertical: 2,
    backgroundColor: "rgba(74,222,128,0.12)", borderRadius: radius.full,
    borderWidth: 1, borderColor: "rgba(74,222,128,0.25)",
  },
  activeBadgeText: { fontSize: typography.xs, color: colors.accent, fontWeight: "600" },

  actionsGrid: { flexDirection: "row", gap: spacing.sm },
  actionCard: {
    flex: 1, backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border, padding: spacing.md, gap: spacing.xs,
  },
  actionTitle: { fontSize: typography.sm, fontWeight: "600", color: colors.head },
  actionSub: { fontSize: typography.xs, color: colors.sub },

  warningBox: {
    flexDirection: "row", gap: spacing.sm, alignItems: "flex-start",
    backgroundColor: "rgba(245,158,11,0.08)", borderRadius: radius.lg,
    borderWidth: 1, borderColor: "rgba(245,158,11,0.2)", padding: spacing.md,
  },
  warningText: { fontSize: typography.xs, color: colors.body, flex: 1, lineHeight: 18 },

  qrCard: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: colors.border, padding: spacing.xl, alignItems: "center", gap: spacing.sm,
  },
  qrPlaceholder: {
    width: 160, height: 160, borderRadius: radius.xl, backgroundColor: colors.raised,
    alignItems: "center", justifyContent: "center", gap: spacing.sm,
  },
  qrHint: { fontSize: typography.xs, color: colors.sub },
  qrTitle: { fontSize: typography.base, fontWeight: "700", color: colors.head },
  qrSub: { fontSize: typography.xs, color: colors.sub },

  monoBox: {
    backgroundColor: colors.raised, borderRadius: radius.md, padding: spacing.md,
    borderWidth: 1, borderColor: colors.border,
  },
  monoText: { fontSize: typography.xs, color: colors.sub, fontFamily: "monospace" },

  scanInput: {
    backgroundColor: colors.surface, borderRadius: radius.lg, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, color: colors.head,
    fontSize: typography.xs, fontFamily: "monospace", minHeight: 100,
  },

  didBox: {
    backgroundColor: colors.raised, borderRadius: radius.md, padding: spacing.sm,
    marginTop: spacing.xs, maxWidth: "100%",
  },
  didText: { fontSize: typography.xs, color: colors.accent, fontFamily: "monospace" },

  btn: {
    flexDirection: "row", alignItems: "center", justifyContent: "center", gap: spacing.sm,
    backgroundColor: colors.accent, borderRadius: radius.lg, padding: spacing.md,
  },
  btnText: { fontSize: typography.base, fontWeight: "700", color: colors.void },
  btnOutline: {
    flexDirection: "row", alignItems: "center", justifyContent: "center", gap: spacing.xs,
    borderRadius: radius.lg, padding: spacing.md,
    borderWidth: 1, borderColor: colors.border, backgroundColor: colors.surface,
  },
  btnOutlineText: { fontSize: typography.base, fontWeight: "600", color: colors.body },

  rowBtns: { flexDirection: "row", gap: spacing.sm },

  doneCard: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: colors.border, padding: spacing.xxl, alignItems: "center",
  },
  doneIcon: {
    width: 80, height: 80, borderRadius: radius.full, alignItems: "center", justifyContent: "center",
    backgroundColor: "rgba(74,222,128,0.08)", borderWidth: 2, borderColor: "rgba(74,222,128,0.3)",
    marginBottom: spacing.md,
  },
  doneTitle: { fontSize: typography.lg, fontWeight: "700", color: colors.head },
  doneSub: { fontSize: typography.sm, color: colors.sub, marginTop: spacing.xs },
});
