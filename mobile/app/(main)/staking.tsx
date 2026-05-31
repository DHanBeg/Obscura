import React, { useState, useEffect, useCallback } from "react";
import {
  View, Text, StyleSheet, ScrollView, TouchableOpacity,
  TextInput, ActivityIndicator, Alert, RefreshControl,
} from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { colors } from "@/lib/theme";
import { api } from "@/lib/api";

interface StakingPosition {
  id: string;
  amount: string;
  staked_at: string;
  unlock_at?: string;
  status: string;
  reward_accrued?: string;
}

interface SlashEvent {
  id: string;
  position_id: string;
  slash_amount: string;
  reason: string;
  created_at: string;
  reviewed: boolean;
}

const STATUS_MAP: Record<string, { label: string; color: string }> = {
  active:   { label: "Aktif",     color: colors.accent },
  unlocked: { label: "Çözüldü",   color: colors.amber },
  slashed:  { label: "Kesildi",   color: colors.red },
};

export default function StakingScreen() {
  const [positions, setPositions] = useState<StakingPosition[]>([]);
  const [slashes, setSlashes]     = useState<SlashEvent[]>([]);
  const [loading, setLoading]     = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [tab, setTab]             = useState<"positions" | "slashes">("positions");

  const [stakeAmt,   setStakeAmt]   = useState("");
  const [unstakeAmt, setUnstakeAmt] = useState("");
  const [busy, setBusy]             = useState<string | null>(null);

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true);
    else setLoading(true);
    try {
      const [pos, sl] = await Promise.all([
        api.stakingPositions(),
        api.stakingSlashes(),
      ]);
      setPositions(pos?.positions || []);
      setSlashes(sl?.slashes || []);
    } catch {}
    finally { setLoading(false); setRefreshing(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const doAction = async (
    key: string,
    fn: () => Promise<any>,
    successMsg: string,
    clearInput?: () => void,
  ) => {
    setBusy(key);
    try {
      await fn();
      clearInput?.();
      await load();
      Alert.alert("Başarılı", successMsg);
    } catch (e: any) {
      Alert.alert("Hata", e.message);
    } finally {
      setBusy(null);
    }
  };

  const totalStaked = positions
    .filter(p => p.status === "active")
    .reduce((sum, p) => sum + parseFloat(p.amount || "0"), 0)
    .toFixed(4);

  const fmt = (v: string) => parseFloat(v || "0").toFixed(4);

  if (loading) {
    return <View style={styles.center}><ActivityIndicator color={colors.accent} size="large" /></View>;
  }

  return (
    <View style={styles.container}>
      <ScrollView
        contentContainerStyle={styles.scroll}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => load(true)} tintColor={colors.accent} />}
      >
        {/* Header */}
        <Text style={styles.title}>Staking</Text>

        {/* Summary card */}
        <View style={styles.summaryCard}>
          <View style={styles.summaryRow}>
            <Ionicons name="lock-closed" size={18} color={colors.accent} />
            <Text style={styles.summaryLabel}>TOPLAM KİLİTLİ</Text>
          </View>
          <Text style={styles.summaryValue}>{totalStaked} <Text style={styles.currency}>OBS</Text></Text>
        </View>

        {/* Stake action */}
        <View style={styles.actionCard}>
          <Text style={styles.actionTitle}>OBS Kilitle</Text>
          <View style={styles.inputRow}>
            <TextInput
              value={stakeAmt}
              onChangeText={setStakeAmt}
              placeholder="Miktar"
              placeholderTextColor={colors.dim}
              style={styles.input}
              keyboardType="decimal-pad"
            />
            <TouchableOpacity
              style={[styles.actionBtn, styles.accentBtn, (!stakeAmt || busy === "stake") && styles.disabledBtn]}
              disabled={!stakeAmt.trim() || busy === "stake"}
              onPress={() => doAction(
                "stake",
                () => api.stakingStake({ amount: stakeAmt.trim() }),
                "OBS kilitleme başarılı.",
                () => setStakeAmt(""),
              )}
            >
              {busy === "stake"
                ? <ActivityIndicator color="#000" size="small" />
                : <Text style={styles.accentBtnText}>Kilitle</Text>
              }
            </TouchableOpacity>
          </View>
        </View>

        {/* Unstake action */}
        <View style={styles.actionCard}>
          <Text style={styles.actionTitle}>Kilidi Aç</Text>
          <View style={styles.inputRow}>
            <TextInput
              value={unstakeAmt}
              onChangeText={setUnstakeAmt}
              placeholder="Miktar"
              placeholderTextColor={colors.dim}
              style={styles.input}
              keyboardType="decimal-pad"
            />
            <TouchableOpacity
              style={[styles.actionBtn, styles.outlineBtn, (!unstakeAmt || busy === "unstake") && styles.disabledBtn]}
              disabled={!unstakeAmt.trim() || busy === "unstake"}
              onPress={() => doAction(
                "unstake",
                () => api.stakingUnstake({ amount: unstakeAmt.trim() }),
                "Kilit açma talebi gönderildi.",
                () => setUnstakeAmt(""),
              )}
            >
              {busy === "unstake"
                ? <ActivityIndicator color={colors.sub} size="small" />
                : <Text style={styles.outlineBtnText}>Kilidi Aç</Text>
              }
            </TouchableOpacity>
          </View>
        </View>

        {/* Withdraw */}
        <TouchableOpacity
          style={[styles.withdrawBtn, busy === "withdraw" && styles.disabledBtn]}
          disabled={busy === "withdraw"}
          onPress={() => doAction(
            "withdraw",
            () => api.stakingWithdraw(),
            "Ödüller çekildi.",
          )}
        >
          {busy === "withdraw"
            ? <ActivityIndicator color={colors.sub} size="small" />
            : <>
                <Ionicons name="download-outline" size={16} color={colors.sub} />
                <Text style={styles.withdrawBtnText}>Ödülleri Çek</Text>
              </>
          }
        </TouchableOpacity>

        {/* Tabs */}
        <View style={styles.tabs}>
          <TouchableOpacity
            style={[styles.tab, tab === "positions" && styles.activeTab]}
            onPress={() => setTab("positions")}
          >
            <Text style={[styles.tabText, tab === "positions" && styles.activeTabText]}>Pozisyonlar</Text>
          </TouchableOpacity>
          <TouchableOpacity
            style={[styles.tab, tab === "slashes" && styles.activeTab]}
            onPress={() => setTab("slashes")}
          >
            <Text style={[styles.tabText, tab === "slashes" && styles.activeTabText]}>Kesintiler</Text>
          </TouchableOpacity>
        </View>

        {/* Content */}
        {tab === "positions" ? (
          <View style={styles.card}>
            {positions.length === 0 ? (
              <View style={styles.emptyWrap}>
                <Ionicons name="lock-open-outline" size={32} color={colors.dim} />
                <Text style={styles.emptyText}>Henüz pozisyon yok</Text>
              </View>
            ) : (
              positions.map((p, i) => {
                const st = STATUS_MAP[p.status] || { label: p.status, color: colors.dim };
                return (
                  <View key={p.id} style={[styles.row, i > 0 && styles.rowBorder]}>
                    <View style={styles.rowLeft}>
                      <Text style={styles.rowAmount}>{fmt(p.amount)} OBS</Text>
                      {p.reward_accrued && parseFloat(p.reward_accrued) > 0 && (
                        <Text style={styles.rowReward}>+{fmt(p.reward_accrued)} ödül</Text>
                      )}
                    </View>
                    <View style={[styles.badge, { backgroundColor: st.color + "22" }]}>
                      <Text style={[styles.badgeText, { color: st.color }]}>{st.label}</Text>
                    </View>
                  </View>
                );
              })
            )}
          </View>
        ) : (
          <View style={styles.card}>
            {slashes.length === 0 ? (
              <View style={styles.emptyWrap}>
                <Ionicons name="shield-checkmark-outline" size={32} color={colors.dim} />
                <Text style={styles.emptyText}>Kesinti kaydı yok</Text>
              </View>
            ) : (
              slashes.map((s, i) => (
                <View key={s.id} style={[styles.row, i > 0 && styles.rowBorder]}>
                  <View style={styles.rowLeft}>
                    <Text style={[styles.rowAmount, { color: colors.red }]}>-{fmt(s.slash_amount)} OBS</Text>
                    <Text style={styles.rowSub} numberOfLines={2}>{s.reason}</Text>
                  </View>
                  {s.reviewed && (
                    <View style={[styles.badge, { backgroundColor: colors.accent + "22" }]}>
                      <Text style={[styles.badgeText, { color: colors.accent }]}>İncelendi</Text>
                    </View>
                  )}
                </View>
              ))
            )}
          </View>
        )}
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  container:     { flex: 1, backgroundColor: colors.void },
  center:        { flex: 1, alignItems: "center", justifyContent: "center", backgroundColor: colors.void },
  scroll:        { padding: 16, paddingBottom: 100 },
  title:         { fontSize: 26, fontWeight: "700", color: colors.head, marginBottom: 16 },

  summaryCard: {
    backgroundColor: colors.accentDeep,
    borderRadius: 20,
    padding: 20,
    marginBottom: 12,
    borderWidth: 1,
    borderColor: colors.accentDim,
  },
  summaryRow:    { flexDirection: "row", alignItems: "center", gap: 6, marginBottom: 8 },
  summaryLabel:  { fontSize: 11, color: colors.accent, letterSpacing: 1.5, fontWeight: "700" },
  summaryValue:  { fontSize: 36, fontWeight: "800", color: colors.head },
  currency:      { fontSize: 18, color: colors.dim, fontWeight: "500" },

  actionCard: {
    backgroundColor: colors.surface,
    borderRadius: 16,
    padding: 14,
    marginBottom: 10,
    borderWidth: 1,
    borderColor: colors.border,
  },
  actionTitle:   { fontSize: 12, color: colors.sub, marginBottom: 10, fontWeight: "600" },
  inputRow:      { flexDirection: "row", gap: 8 },
  input: {
    flex: 1,
    backgroundColor: colors.raised,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: 12,
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontSize: 14,
    color: colors.body,
  },
  actionBtn:     { width: 90, borderRadius: 12, alignItems: "center", justifyContent: "center" },
  accentBtn:     { backgroundColor: colors.accent },
  accentBtnText: { fontSize: 14, fontWeight: "700", color: "#000" },
  outlineBtn:    { borderWidth: 1, borderColor: colors.border },
  outlineBtnText: { fontSize: 14, fontWeight: "600", color: colors.sub },
  disabledBtn:   { opacity: 0.45 },

  withdrawBtn: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 6,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: 14,
    height: 42,
    marginBottom: 20,
  },
  withdrawBtnText: { fontSize: 14, color: colors.sub },

  tabs:          { flexDirection: "row", gap: 4, marginBottom: 12 },
  tab: {
    flex: 1, height: 36, borderRadius: 10,
    backgroundColor: colors.surface,
    borderWidth: 1, borderColor: colors.border,
    alignItems: "center", justifyContent: "center",
  },
  activeTab:     { backgroundColor: colors.raised, borderColor: colors.accent + "55" },
  tabText:       { fontSize: 13, fontWeight: "600", color: colors.dim },
  activeTabText: { color: colors.head },

  card:          { backgroundColor: colors.surface, borderRadius: 20, borderWidth: 1, borderColor: colors.border, overflow: "hidden" },
  row:           { flexDirection: "row", alignItems: "center", padding: 14, gap: 10 },
  rowBorder:     { borderTopWidth: 1, borderTopColor: "rgba(255,255,255,0.06)" },
  rowLeft:       { flex: 1 },
  rowAmount:     { fontSize: 15, fontWeight: "700", color: colors.head },
  rowReward:     { fontSize: 12, color: colors.accent, marginTop: 2 },
  rowSub:        { fontSize: 12, color: colors.dim, marginTop: 2 },
  badge:         { borderRadius: 8, paddingHorizontal: 8, paddingVertical: 3 },
  badgeText:     { fontSize: 10, fontWeight: "700" },
  emptyWrap:     { alignItems: "center", paddingVertical: 32, gap: 10 },
  emptyText:     { fontSize: 14, color: colors.dim },
});
