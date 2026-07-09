import React, { useState } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, StyleSheet,
  TextInput, ActivityIndicator, Alert,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import * as SecureStore from "expo-secure-store";
import * as Clipboard from "expo-clipboard";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";

type Tab = "setup" | "recover";

interface Share {
  index: number;
  value: number[];
}

export default function SettingsRecoveryScreen() {
  const [tab, setTab] = useState<Tab>("setup");

  // Setup
  const [shares, setShares] = useState<Share[] | null>(null);
  const [splitting, setSplitting] = useState(false);
  const [splitError, setSplitError] = useState("");

  // Recover
  const [inputShares, setInputShares] = useState<string[]>(["", "", ""]);
  const [recoveredMnemonic, setRecoveredMnemonic] = useState("");
  const [showRecovered, setShowRecovered] = useState(false);
  const [recovering, setRecovering] = useState(false);
  const [recoverError, setRecoverError] = useState("");

  const handleSplit = async () => {
    setSplitting(true);
    setSplitError("");
    setShares(null);
    try {
      const mnemonic = await SecureStore.getItemAsync("obscura_mnemonic");
      if (!mnemonic) throw new Error("Mnemonic bulunamadı. Önce cihazda yedek oluşturun.");
      const res = await api.shamirSplit(mnemonic, 3, 5);
      if (!res?.shares) throw new Error("Paylaşım oluşturulamadı");
      setShares(res.shares as Share[]);
    } catch (e: any) {
      setSplitError(e.message || "Hata oluştu");
    } finally {
      setSplitting(false);
    }
  };

  const copyShare = (idx: number) => {
    if (!shares) return;
    const share = shares[idx];
    const hex = Array.from(share.value).map((b) => b.toString(16).padStart(2, "0")).join("");
    Clipboard.setStringAsync(`${share.index}:${hex}`);
    Alert.alert("Kopyalandı", `Paylaşım ${idx + 1} panoya kopyalandı.`);
  };

  const handleRecover = async () => {
    const filtered = inputShares.filter((s) => s.trim());
    if (filtered.length < 3) { setRecoverError("En az 3 paylaşım gerekli"); return; }
    setRecoverError("");
    setRecovering(true);
    try {
      const parsed = filtered.map((s) => {
        const [idxStr, hex] = s.split(":");
        const value = hex ? Array.from({ length: hex.length / 2 }, (_, i) => parseInt(hex.slice(i * 2, i * 2 + 2), 16)) : [];
        return { index: parseInt(idxStr), value };
      });
      const res = await api.shamirCombine(parsed);
      if (!res?.mnemonic) throw new Error("Kurtarma başarısız");
      setRecoveredMnemonic(res.mnemonic);
    } catch (e: any) {
      setRecoverError(e.message || "Kurtarma başarısız");
    } finally {
      setRecovering(false);
    }
  };

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Hesap Kurtarma</Text>
        <View style={{ width: 40 }} />
      </View>

      {/* Tabs */}
      <View style={styles.tabs}>
        {(["setup", "recover"] as Tab[]).map((t) => (
          <TouchableOpacity
            key={t}
            style={[styles.tab, tab === t && styles.tabActive]}
            onPress={() => setTab(t)}
          >
            <Text style={[styles.tabText, tab === t && styles.tabTextActive]}>
              {t === "setup" ? "Kurulum" : "Kurtarma"}
            </Text>
          </TouchableOpacity>
        ))}
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        {tab === "setup" && (
          <>
            <View style={styles.infoBox}>
              <Ionicons name="shield-outline" size={20} color={colors.accent} />
              <Text style={styles.infoText}>
                Mnemonik ifaden 5 parçaya bölünür. Hesabını kurtarmak için herhangi 3 tanesine ihtiyacın var.
                Her parçayı farklı ve güvenilir bir kişiye ver.
              </Text>
            </View>

            <TouchableOpacity
              style={[styles.actionBtn, splitting && styles.actionBtnDim]}
              onPress={handleSplit}
              disabled={splitting}
            >
              {splitting
                ? <ActivityIndicator size="small" color={colors.void} />
                : <Text style={styles.actionBtnText}>Paylaşımları Oluştur</Text>}
            </TouchableOpacity>

            {splitError ? <Text style={styles.errorText}>{splitError}</Text> : null}

            {shares && (
              <View style={styles.sharesWrap}>
                {shares.map((_, i) => (
                  <View key={i} style={styles.shareCard}>
                    <View style={styles.shareHeader}>
                      <Text style={styles.shareLabel}>Paylaşım {i + 1}</Text>
                      <TouchableOpacity onPress={() => copyShare(i)} style={styles.copyBtn}>
                        <Ionicons name="copy-outline" size={16} color={colors.accent} />
                        <Text style={styles.copyBtnText}>Kopyala</Text>
                      </TouchableOpacity>
                    </View>
                    <Text style={styles.sharePreview}>••••••••••••••••••••</Text>
                  </View>
                ))}
              </View>
            )}
          </>
        )}

        {tab === "recover" && (
          <>
            <View style={styles.infoBox}>
              <Ionicons name="key-outline" size={20} color={colors.amber} />
              <Text style={styles.infoText}>
                3 veya daha fazla paylaşımı girerek mnemonik ifandenizi kurtarabilirsiniz.
              </Text>
            </View>

            {inputShares.map((s, i) => (
              <View key={i}>
                <Text style={styles.inputLabel}>Paylaşım {i + 1}</Text>
                <TextInput
                  style={styles.shareInput}
                  value={s}
                  onChangeText={(v) => setInputShares((prev) => { const n = [...prev]; n[i] = v; return n; })}
                  placeholder="index:hex..."
                  placeholderTextColor={colors.dim}
                  autoCapitalize="none"
                  autoCorrect={false}
                />
              </View>
            ))}

            {recoverError ? <Text style={styles.errorText}>{recoverError}</Text> : null}

            <TouchableOpacity
              style={[styles.actionBtn, recovering && styles.actionBtnDim]}
              onPress={handleRecover}
              disabled={recovering}
            >
              {recovering
                ? <ActivityIndicator size="small" color={colors.void} />
                : <Text style={styles.actionBtnText}>Kurtarma Yap</Text>}
            </TouchableOpacity>

            {recoveredMnemonic ? (
              <View style={styles.recoveredBox}>
                <View style={styles.recoveredHeader}>
                  <Text style={styles.recoveredLabel}>Kurtarılan Mnemonik</Text>
                  <TouchableOpacity onPress={() => setShowRecovered(!showRecovered)}>
                    <Ionicons name={showRecovered ? "eye-off-outline" : "eye-outline"} size={18} color={colors.sub} />
                  </TouchableOpacity>
                </View>
                <Text style={styles.recoveredText}>
                  {showRecovered ? recoveredMnemonic : "••• ••• ••• ••• ••• ••• ••• ••• ••• ••• ••• •••"}
                </Text>
              </View>
            ) : null}
          </>
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
  backBtn: { width: 40, height: 40, alignItems: "center", justifyContent: "center" },
  headerTitle: { flex: 1, textAlign: "center", fontSize: typography.md, fontWeight: "700", color: colors.head },
  tabs: {
    flexDirection: "row", borderBottomWidth: 1, borderBottomColor: colors.border,
  },
  tab: {
    flex: 1, paddingVertical: 12, alignItems: "center",
    borderBottomWidth: 2, borderBottomColor: "transparent",
  },
  tabActive: { borderBottomColor: colors.accent },
  tabText: { fontSize: typography.sm, color: colors.sub },
  tabTextActive: { color: colors.accent, fontWeight: "600" },
  content: { padding: spacing.lg, gap: spacing.md, paddingBottom: 48 },
  infoBox: {
    flexDirection: "row", gap: spacing.sm, alignItems: "flex-start",
    backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border, padding: spacing.md,
  },
  infoText: { flex: 1, fontSize: typography.sm, color: colors.body, lineHeight: 20 },
  actionBtn: {
    height: 52, borderRadius: radius.xl, backgroundColor: colors.accent,
    alignItems: "center", justifyContent: "center",
  },
  actionBtnDim: { opacity: 0.6 },
  actionBtnText: { fontSize: typography.base, fontWeight: "700", color: colors.void },
  errorText: { fontSize: typography.xs, color: colors.red },
  sharesWrap: { gap: spacing.sm },
  shareCard: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, gap: spacing.xs,
  },
  shareHeader: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  shareLabel: { fontSize: typography.sm, fontWeight: "600", color: colors.head },
  copyBtn: { flexDirection: "row", alignItems: "center", gap: 4 },
  copyBtnText: { fontSize: typography.xs, color: colors.accent },
  sharePreview: { fontSize: typography.xs, color: colors.dim, fontFamily: "monospace" },
  inputLabel: { fontSize: typography.xs, color: colors.sub, marginBottom: 4, fontWeight: "600" },
  shareInput: {
    backgroundColor: colors.surface, borderRadius: radius.lg, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, color: colors.head,
    fontSize: typography.xs, fontFamily: "monospace",
  },
  recoveredBox: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: "rgba(74,222,128,0.2)", padding: spacing.md, gap: spacing.sm,
  },
  recoveredHeader: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  recoveredLabel: { fontSize: typography.sm, fontWeight: "600", color: colors.head },
  recoveredText: { fontSize: typography.sm, color: colors.body, lineHeight: 22, fontFamily: "monospace" },
  mnemonicCopyBtn: { flexDirection: "row", alignItems: "center", gap: 6, alignSelf: "flex-start" },
  mnemonicCopyText: { fontSize: typography.xs, color: colors.accent },
});
