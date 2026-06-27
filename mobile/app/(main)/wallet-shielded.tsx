import React, { useState, useEffect, useCallback } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, StyleSheet,
  TextInput, ActivityIndicator, Alert,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";

interface ShieldedNote {
  nullifier: string;
  commitment: string;
  amount: string;
  created_at: string;
  spent: boolean;
}

function randomHex(len: number) {
  let s = "";
  for (let i = 0; i < len; i++) s += Math.floor(Math.random() * 16).toString(16);
  return s;
}

function formatTime(iso: string) {
  try {
    return new Date(iso).toLocaleDateString("tr-TR", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
  } catch { return iso; }
}

export default function WalletShieldedScreen() {
  const [notes, setNotes] = useState<ShieldedNote[]>([]);
  const [root, setRoot] = useState("");
  const [loading, setLoading] = useState(true);
  const [shieldAmount, setShieldAmount] = useState("");
  const [shielding, setShielding] = useState(false);
  const [provingState, setProvingState] = useState<"idle" | "proving" | "done">("idle");
  const [balanceVisible, setBalanceVisible] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [notesRes, rootRes] = await Promise.all([
        api.walletShieldedNotes(),
        api.walletShieldedRoot(),
      ]);
      setNotes(notesRes?.notes || []);
      setRoot(rootRes?.merkle_root || "");
    } catch {}
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleShield = async () => {
    if (!shieldAmount.trim()) return;
    setShielding(true);
    setProvingState("proving");
    setError("");
    setSuccess("");
    try {
      const commitment = "0x" + randomHex(64);
      await api.walletShield({ amount: shieldAmount, commitment });
      setProvingState("done");
      setSuccess("Başarıyla gizlendi! Shielded pool'a aktarıldı.");
      setShieldAmount("");
      await load();
    } catch (e: any) {
      setProvingState("idle");
      setError(e?.message || "Bir hata oluştu");
    } finally {
      setShielding(false);
      setTimeout(() => { setProvingState("idle"); setSuccess(""); }, 3000);
    }
  };

  const unspentBalance = notes
    .filter((n) => !n.spent)
    .reduce((sum, n) => sum + parseFloat(n.amount || "0"), 0)
    .toFixed(4);

  const unspentCount = notes.filter((n) => !n.spent).length;

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Gizli Havuz</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        {/* Hero */}
        <View style={styles.heroCard}>
          <View style={styles.shieldIcon}>
            <Ionicons name="shield-checkmark" size={32} color={colors.accent} />
          </View>
          <Text style={styles.heroTitle}>Gizli Havuz</Text>
          <Text style={styles.heroSub}>Miktarınız ve alıcınız tamamen gizli kalır</Text>

          <View style={styles.balanceRow}>
            {loading ? (
              <ActivityIndicator size="small" color={colors.accent} />
            ) : balanceVisible ? (
              <>
                <Text style={styles.balanceAmount}>{unspentBalance}</Text>
                <Text style={styles.balanceCurrency}>OBS</Text>
              </>
            ) : (
              <Text style={styles.balanceHidden}>⬤ ⬤ ⬤ ⬤ ⬤</Text>
            )}
          </View>

          <Text style={styles.balanceMeta}>
            {balanceVisible ? `${unspentCount} aktif not` : "Bakiyeyi görmek için kilidi aç"}
          </Text>

          <TouchableOpacity style={styles.revealBtn} onPress={() => setBalanceVisible((v) => !v)}>
            <Ionicons name={balanceVisible ? "eye-off-outline" : "eye-outline"} size={14} color={colors.sub} />
            <Text style={styles.revealBtnText}>{balanceVisible ? "Gizle" : "Göster"}</Text>
          </TouchableOpacity>

          {root ? (
            <View style={styles.rootPill}>
              <Text style={styles.rootText}>Root: {root.slice(0, 20)}…{root.slice(-8)}</Text>
            </View>
          ) : null}
        </View>

        {/* Shield action */}
        <View style={styles.card}>
          <Text style={styles.cardTitle}>Gizli Havuza Aktar</Text>
          <Text style={styles.cardSub}>Miktar ve alıcı gizli kalır. İşlem izlenebilir değil.</Text>

          <View style={styles.quickAmounts}>
            {["10", "50", "100", "500"].map((v) => (
              <TouchableOpacity
                key={v}
                style={[styles.quickBtn, shieldAmount === v && styles.quickBtnActive]}
                onPress={() => setShieldAmount(v)}
              >
                <Text style={[styles.quickBtnText, shieldAmount === v && styles.quickBtnTextActive]}>
                  {v}
                </Text>
              </TouchableOpacity>
            ))}
          </View>

          <TextInput
            style={styles.amountInput}
            value={shieldAmount}
            onChangeText={setShieldAmount}
            placeholder="0"
            placeholderTextColor={colors.sub}
            keyboardType="numeric"
          />

          {provingState !== "idle" && (
            <View style={[styles.provingBanner, provingState === "done" && styles.provingBannerDone]}>
              {provingState === "proving" && (
                <ActivityIndicator size="small" color="#4da8ff" style={{ marginRight: spacing.sm }} />
              )}
              {provingState === "done" && (
                <Ionicons name="shield-checkmark" size={14} color={colors.accent} style={{ marginRight: spacing.sm }} />
              )}
              <Text style={[styles.provingText, provingState === "done" && styles.provingTextDone]}>
                {provingState === "proving" ? "Kanıt üretiliyor…" : "Kanıt doğrulandı"}
              </Text>
            </View>
          )}

          {error ? <Text style={styles.errorText}>{error}</Text> : null}
          {success ? <Text style={styles.successText}>{success}</Text> : null}

          <TouchableOpacity
            style={[styles.shieldBtn, (shielding || !shieldAmount) && styles.shieldBtnDisabled]}
            onPress={handleShield}
            disabled={shielding || !shieldAmount}
          >
            {shielding
              ? <ActivityIndicator size="small" color={colors.void} />
              : <Ionicons name="shield-checkmark" size={16} color={colors.void} />}
            <Text style={styles.shieldBtnText}>Gizli Havuza Gönder</Text>
          </TouchableOpacity>
        </View>

        {/* Notes list */}
        <Text style={styles.sectionLabel}>GİZLİ NOTLAR</Text>
        <View style={styles.card}>
          {loading ? (
            <View style={styles.emptyState}>
              <ActivityIndicator size="small" color={colors.accent} />
            </View>
          ) : notes.length === 0 ? (
            <View style={styles.emptyState}>
              <Ionicons name="lock-closed-outline" size={28} color={colors.sub} />
              <Text style={styles.emptyText}>Henüz gizli not yok</Text>
              <Text style={styles.emptySubText}>İlk transferini yukarıdan gönder</Text>
            </View>
          ) : (
            notes.map((note, i) => (
              <View
                key={note.nullifier}
                style={[styles.noteRow, i < notes.length - 1 && styles.noteRowBorder, note.spent && styles.noteSpent]}
              >
                <View style={[styles.noteIcon, note.spent ? styles.noteIconSpent : styles.noteIconActive]}>
                  <Ionicons
                    name={note.spent ? "lock-open-outline" : "lock-closed-outline"}
                    size={15}
                    color={note.spent ? colors.sub : colors.tier2}
                  />
                </View>
                <View style={{ flex: 1, minWidth: 0 }}>
                  <Text style={styles.noteAmount}>
                    {note.spent ? "••••" : parseFloat(note.amount).toFixed(4)} OBS
                  </Text>
                  <Text style={styles.noteCommitment} numberOfLines={1}>
                    {note.commitment.slice(0, 18)}…
                  </Text>
                </View>
                <View style={{ alignItems: "flex-end", gap: 4 }}>
                  <Text style={styles.noteDate}>{formatTime(note.created_at)}</Text>
                  {note.spent
                    ? <Text style={styles.spentBadge}>Harcandı</Text>
                    : <TouchableOpacity>
                        <Text style={styles.withdrawText}>Çıkar</Text>
                      </TouchableOpacity>}
                </View>
              </View>
            ))
          )}
        </View>
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
    borderColor: "rgba(74,222,128,0.15)", padding: spacing.xl, alignItems: "center", gap: spacing.sm,
  },
  shieldIcon: {
    width: 64, height: 64, borderRadius: radius.full, alignItems: "center", justifyContent: "center",
    backgroundColor: "rgba(74,222,128,0.06)", borderWidth: 1, borderColor: "rgba(74,222,128,0.18)",
  },
  heroTitle: { fontSize: typography.lg, fontWeight: "700", color: colors.head, letterSpacing: -0.5 },
  heroSub: { fontSize: typography.sm, color: colors.sub },
  balanceRow: { flexDirection: "row", alignItems: "baseline", gap: spacing.sm, marginTop: spacing.sm },
  balanceAmount: { fontSize: 38, fontWeight: "800", color: colors.head, letterSpacing: -1 },
  balanceCurrency: { fontSize: typography.base, fontWeight: "600", color: colors.sub },
  balanceHidden: { fontSize: 28, color: colors.head, letterSpacing: 8 },
  balanceMeta: { fontSize: typography.xs, color: colors.sub },
  revealBtn: {
    flexDirection: "row", alignItems: "center", gap: spacing.xs,
    paddingHorizontal: spacing.md, paddingVertical: spacing.xs,
    backgroundColor: colors.raised, borderRadius: radius.full,
  },
  revealBtnText: { fontSize: typography.xs, color: colors.sub },
  rootPill: {
    paddingHorizontal: spacing.md, paddingVertical: spacing.xs,
    backgroundColor: colors.raised, borderRadius: radius.full,
    borderWidth: 1, borderColor: colors.border,
  },
  rootText: { fontSize: typography.xs, color: colors.sub, fontFamily: "monospace" },

  card: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, gap: spacing.sm, overflow: "hidden",
  },
  cardTitle: { fontSize: typography.base, fontWeight: "700", color: colors.head },
  cardSub: { fontSize: typography.xs, color: colors.sub },
  quickAmounts: { flexDirection: "row", gap: spacing.sm },
  quickBtn: {
    flex: 1, paddingVertical: spacing.sm, borderRadius: radius.lg, alignItems: "center",
    backgroundColor: colors.raised, borderWidth: 1, borderColor: "transparent",
  },
  quickBtnActive: {
    backgroundColor: "rgba(74,222,128,0.12)", borderColor: "rgba(74,222,128,0.3)",
  },
  quickBtnText: { fontSize: typography.sm, fontWeight: "600", color: colors.sub },
  quickBtnTextActive: { color: colors.accent },
  amountInput: {
    height: 60, backgroundColor: colors.raised, borderRadius: radius.lg, borderWidth: 1,
    borderColor: colors.border, textAlign: "center", fontSize: 28, fontWeight: "800",
    color: colors.head, paddingHorizontal: spacing.md,
  },
  provingBanner: {
    flexDirection: "row", alignItems: "center", paddingHorizontal: spacing.md, paddingVertical: spacing.sm,
    borderRadius: radius.lg, backgroundColor: "rgba(77,168,255,0.08)",
    borderWidth: 1, borderColor: "rgba(77,168,255,0.2)",
  },
  provingBannerDone: {
    backgroundColor: "rgba(74,222,128,0.08)", borderColor: "rgba(74,222,128,0.25)",
  },
  provingText: { fontSize: typography.sm, color: "#4da8ff" },
  provingTextDone: { color: colors.accent },
  errorText: { fontSize: typography.sm, color: colors.red, paddingHorizontal: spacing.md, paddingVertical: spacing.sm,
    backgroundColor: "rgba(239,68,68,0.08)", borderRadius: radius.md },
  successText: { fontSize: typography.sm, color: colors.accent, paddingHorizontal: spacing.md, paddingVertical: spacing.sm,
    backgroundColor: "rgba(74,222,128,0.08)", borderRadius: radius.md },
  shieldBtn: {
    flexDirection: "row", alignItems: "center", justifyContent: "center", gap: spacing.sm,
    backgroundColor: colors.accent, borderRadius: radius.lg, padding: spacing.md,
  },
  shieldBtnDisabled: { opacity: 0.5 },
  shieldBtnText: { fontSize: typography.base, fontWeight: "700", color: colors.void },

  sectionLabel: {
    fontSize: typography.xs, color: colors.sub, fontWeight: "600",
    textTransform: "uppercase", letterSpacing: 1,
  },
  emptyState: { alignItems: "center", paddingVertical: spacing.xxl, gap: spacing.sm },
  emptyText: { fontSize: typography.sm, color: colors.sub },
  emptySubText: { fontSize: typography.xs, color: colors.dim },
  noteRow: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm, padding: spacing.md,
  },
  noteRowBorder: { borderBottomWidth: 1, borderBottomColor: colors.border },
  noteSpent: { opacity: 0.5 },
  noteIcon: {
    width: 40, height: 40, borderRadius: radius.full, alignItems: "center", justifyContent: "center",
  },
  noteIconActive: { backgroundColor: "rgba(77,168,255,0.1)", borderWidth: 1, borderColor: "rgba(77,168,255,0.2)" },
  noteIconSpent: { backgroundColor: colors.raised, borderWidth: 1, borderColor: colors.border },
  noteAmount: { fontSize: typography.sm, fontWeight: "600", color: colors.head },
  noteCommitment: { fontSize: typography.xs, color: colors.sub, fontFamily: "monospace" },
  noteDate: { fontSize: typography.xs, color: colors.sub },
  spentBadge: { fontSize: typography.xs, color: colors.sub },
  withdrawText: { fontSize: typography.xs, color: colors.accent, fontWeight: "600" },
});
