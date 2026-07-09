import React, { useState, useEffect, useCallback } from "react";
import {
  View, Text, StyleSheet, ScrollView, TouchableOpacity,
  Modal, TextInput, ActivityIndicator, Alert, RefreshControl, Image,
} from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, typography, radius, shadow } from "@/lib/theme";
import { TierBadge } from "@/components/ui/TierBadge";
import { ChamferCorner } from "@/components/ui/ChamferCorner";
import { api } from "@/lib/api";
import { useStore } from "@/lib/store";

interface WalletBalance {
  transparent_balance: string;
  currency: string;
}

interface WalletTx {
  id: string;
  from_did: string;
  to_did: string;
  amount: string;
  memo?: string;
  created_at: string;
  tx_type?: string;
}

export default function WalletScreen() {
  const { user } = useStore();
  const [balance, setBalance] = useState<WalletBalance | null>(null);
  const [txs, setTxs] = useState<WalletTx[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [toDID, setToDID] = useState("");
  const [amount, setAmount] = useState("");
  const [memo, setMemo] = useState("");
  const [sending, setSending] = useState(false);

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true);
    else setLoading(true);
    try {
      const [bal, hist] = await Promise.all([
        api.walletBalance(),
        api.walletTransactions(20),
      ]);
      setBalance(bal);
      setTxs(hist?.transactions || []);
    } catch {}
    finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleTransfer = async () => {
    if (!toDID.trim() || !amount.trim()) return;
    setSending(true);
    try {
      await api.walletTransfer({ to_did: toDID.trim(), amount: amount.trim(), memo: memo.trim() || undefined });
      Alert.alert("Başarılı", "Transfer tamamlandı.");
      setToDID(""); setAmount(""); setMemo("");
      setModalVisible(false);
      await load();
    } catch (e: any) {
      Alert.alert("Hata", e.message);
    } finally {
      setSending(false);
    }
  };

  const isOutgoing = (tx: WalletTx) => tx.from_did === user?.did;
  const fmt = (n: string) => parseFloat(n || "0").toFixed(4);

  if (loading) {
    return (
      <View style={styles.center}>
        <ActivityIndicator color={colors.accent} size="large" />
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <ScrollView
        contentContainerStyle={styles.scroll}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => load(true)} tintColor={colors.accent} />}
      >
        {/* Header */}
        <Text style={styles.title}>Cüzdan</Text>

        {/* Balance card */}
        <View style={styles.balanceCardShadowWrap}>
          <View style={styles.balanceCard}>
            <Image
              source={require("@/assets/logo.jpeg")}
              style={styles.balanceCardWatermark}
              resizeMode="contain"
            />
            <View style={styles.balanceLabelRow}>
              <Text style={styles.balanceLabel}>OBS BAKİYE</Text>
              {typeof (user as any)?.tier === "number" && <TierBadge tier={(user as any).tier} size="sm" />}
            </View>
            <View style={styles.balanceRow}>
              <Text style={styles.balanceAmount}>{fmt(balance?.transparent_balance || "0")}</Text>
              <Text style={styles.balanceCurrency}>OBS</Text>
            </View>
            {user?.did && (
              <Text style={styles.did} numberOfLines={1}>
                {user.did.slice(0, 20)}...{user.did.slice(-6)}
              </Text>
            )}
            <ChamferCorner corner="topRight" size={20} color={colors.void} />
            <ChamferCorner corner="bottomLeft" size={20} color={colors.void} />
          </View>
        </View>

        {/* Actions — bento: birincil dolu, ikincil outline */}
        <View style={{ flexDirection: "row", gap: spacing.sm, marginBottom: spacing.sm }}>
          <TouchableOpacity style={styles.bentoTilePrimary} onPress={() => setModalVisible(true)}>
            <View style={styles.bentoIconWrapPrimary}>
              <Ionicons name="arrow-up" size={18} color="#000" />
            </View>
            <Text style={styles.bentoLabelPrimary}>Gönder</Text>
            <Text style={styles.bentoSubPrimary}>OBS transferi</Text>
          </TouchableOpacity>
          <TouchableOpacity
            style={styles.bentoTileSecondary}
            onPress={() => router.push("/(main)/wallet-shielded" as any)}
          >
            <View style={styles.bentoIconWrapSecondary}>
              <Ionicons name="shield-checkmark-outline" size={18} color={colors.accent} />
            </View>
            <Text style={styles.bentoLabelSecondary}>Gizli Havuz</Text>
            <Text style={styles.bentoSub}>shielded transfer</Text>
          </TouchableOpacity>
        </View>

        {/* OBS Topluluk */}
        <TouchableOpacity
          style={styles.communityBtn}
          onPress={() => router.push("/(main)/governance" as any)}
        >
          <Ionicons name="people-outline" size={16} color={colors.accent} />
          <Text style={styles.communityBtnText}>OBS Topluluk</Text>
        </TouchableOpacity>

        {/* Blockchain kısayolları — 2×2 bento grid */}
        <View style={styles.bentoGrid}>
          {[
            { icon: "trending-up-outline" as const, label: "Staking", sub: "Kilitle, kazan", route: "/(main)/staking" },
            { icon: "hardware-chip-outline" as const, label: "Node'lar", sub: "Ağ gezgini", route: "/(main)/nodes" },
            { icon: "swap-horizontal-outline" as const, label: "Bridge", sub: "Zincirler arası", route: "/(main)/bridge" },
            { icon: "gift-outline" as const, label: "Airdrop", sub: "Talep et", route: "/(main)/airdrop" },
          ].map((item) => (
            <TouchableOpacity
              key={item.route}
              style={styles.bentoGridTile}
              onPress={() => router.push(item.route as any)}
            >
              <Ionicons name={item.icon} size={18} color={colors.accent} />
              <Text style={styles.bentoGridLabel}>{item.label}</Text>
              <Text style={styles.bentoGridSub}>{item.sub}</Text>
            </TouchableOpacity>
          ))}
        </View>

        {/* Transactions */}
        <Text style={styles.sectionTitle}>İşlem Geçmişi</Text>
        <View style={styles.card}>
          {txs.length === 0 ? (
            <Text style={styles.empty}>Henüz işlem yok</Text>
          ) : (
            txs.map((tx, i) => {
              const out = isOutgoing(tx);
              return (
                <View key={tx.id} style={[styles.txRow, i < txs.length - 1 && styles.txBorder]}>
                  <View style={[styles.txIcon, { backgroundColor: out ? "rgba(239,68,68,0.12)" : "rgba(99,102,241,0.12)" }]}>
                    <Ionicons
                      name={out ? "arrow-up" : "arrow-down"}
                      size={14}
                      color={out ? colors.red : colors.accent}
                    />
                  </View>
                  <View style={styles.txInfo}>
                    <Text style={styles.txTitle}>{out ? "Gönderildi" : "Alındı"}{tx.memo ? ` · ${tx.memo}` : ""}</Text>
                    <Text style={styles.txSub} numberOfLines={1}>
                      {out ? tx.to_did?.slice(0, 16) : tx.from_did?.slice(0, 16)}...
                    </Text>
                  </View>
                  <Text style={[styles.txAmount, { color: out ? colors.red : colors.accent }]}>
                    {out ? "-" : "+"}{fmt(tx.amount)}
                  </Text>
                </View>
              );
            })
          )}
        </View>
      </ScrollView>

      {/* Transfer Modal */}
      <Modal visible={modalVisible} animationType="slide" transparent>
        <View style={styles.modalOverlay}>
          <View style={styles.modalSheet}>
            <Text style={styles.modalTitle}>OBS Gönder</Text>
            <Text style={styles.inputLabel}>Alıcı DID</Text>
            <TextInput
              value={toDID} onChangeText={setToDID}
              placeholder="did:obs:..."
              placeholderTextColor={colors.dim}
              style={styles.input}
              autoCapitalize="none"
            />
            <Text style={styles.inputLabel}>Miktar (OBS)</Text>
            <TextInput
              value={amount} onChangeText={setAmount}
              placeholder="0.0000"
              placeholderTextColor={colors.dim}
              style={styles.input}
              keyboardType="decimal-pad"
            />
            <Text style={styles.inputLabel}>Not (isteğe bağlı)</Text>
            <TextInput
              value={memo} onChangeText={setMemo}
              placeholder="İşlem notu"
              placeholderTextColor={colors.dim}
              style={styles.input}
            />
            <View style={styles.modalBtns}>
              <TouchableOpacity style={styles.cancelBtn} onPress={() => setModalVisible(false)}>
                <Text style={styles.cancelBtnText}>İptal</Text>
              </TouchableOpacity>
              <TouchableOpacity
                style={[styles.confirmBtn, (sending || !toDID || !amount) && styles.disabledBtn]}
                onPress={handleTransfer}
                disabled={sending || !toDID.trim() || !amount.trim()}
              >
                {sending ? <ActivityIndicator color="#000" size="small" /> : <Text style={styles.confirmBtnText}>Gönder</Text>}
              </TouchableOpacity>
            </View>
          </View>
        </View>
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },
  center: { flex: 1, alignItems: "center", justifyContent: "center", backgroundColor: colors.void },
  scroll: { padding: spacing.md, paddingBottom: spacing.xxl * 2 },
  title: { fontSize: typography.xl, fontWeight: "700", color: colors.head, marginBottom: spacing.md },
  balanceCardShadowWrap: {
    borderRadius: radius.xl,
    borderTopRightRadius: 0,
    borderBottomLeftRadius: 0,
    marginBottom: spacing.sm,
    ...shadow.lg,
  },
  balanceCard: {
    backgroundColor: "rgba(99,102,241,0.12)",
    borderRadius: radius.xl,
    borderTopRightRadius: 0,
    borderBottomLeftRadius: 0,
    padding: spacing.lg,
    borderWidth: 1,
    borderColor: "rgba(99,102,241,0.2)",
    overflow: "hidden",
  },
  balanceCardWatermark: {
    position: "absolute",
    right: -30,
    bottom: -30,
    width: 160,
    height: 160,
    opacity: 0.06,
  },
  balanceLabelRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginBottom: spacing.sm },
  balanceLabel: { fontSize: typography.xs, color: colors.dim, letterSpacing: 1.5, fontWeight: "600" },
  balanceRow: { flexDirection: "row", alignItems: "baseline", gap: 6 },
  balanceAmount: { fontSize: typography.xxxl, fontWeight: "800", color: colors.head },
  balanceCurrency: { fontSize: typography.md, color: colors.dim, fontWeight: "500" },
  did: { fontSize: typography.xs, color: colors.dim, fontFamily: "Courier", marginTop: spacing.sm },
  bentoTilePrimary: {
    flex: 1.2, backgroundColor: colors.accent, borderRadius: radius.lg,
    padding: spacing.md, ...shadow.accent,
  },
  bentoIconWrapPrimary: {
    width: 32, height: 32, borderRadius: radius.md,
    backgroundColor: "rgba(0,0,0,0.15)", alignItems: "center", justifyContent: "center",
    marginBottom: spacing.sm,
  },
  bentoLabelPrimary: { fontSize: typography.base, fontWeight: "700", color: "#000" },
  bentoSubPrimary: { fontSize: 11, color: "rgba(0,0,0,0.6)", marginTop: 2 },
  bentoTileSecondary: {
    flex: 1, backgroundColor: colors.accentDeep, borderWidth: 1, borderColor: colors.accentDim,
    borderRadius: radius.lg, padding: spacing.md,
  },
  bentoIconWrapSecondary: {
    width: 32, height: 32, borderRadius: radius.md,
    backgroundColor: colors.accentDim, alignItems: "center", justifyContent: "center",
    marginBottom: spacing.sm,
  },
  bentoLabelSecondary: { fontSize: typography.base, fontWeight: "700", color: colors.accent },
  bentoSub: { fontSize: 11, color: colors.dim, marginTop: 2 },
  bentoGrid: {
    flexDirection: "row", flexWrap: "wrap", gap: spacing.sm, marginTop: spacing.sm,
  },
  bentoGridTile: {
    width: "47%", backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.border,
    borderRadius: radius.lg, padding: spacing.md,
  },
  bentoGridLabel: { fontSize: typography.sm, fontWeight: "700", color: colors.head, marginTop: spacing.sm },
  bentoGridSub: { fontSize: 11, color: colors.dim, marginTop: 2 },
  sectionTitle: { fontSize: typography.xs, color: colors.dim, letterSpacing: 1.5, fontWeight: "600", marginBottom: spacing.sm },
  card: { backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1, borderColor: colors.border, overflow: "hidden" },
  empty: { padding: spacing.lg, textAlign: "center", color: colors.dim, fontSize: typography.sm },
  txRow: { flexDirection: "row", alignItems: "center", padding: spacing.md, gap: spacing.sm },
  txBorder: { borderBottomWidth: 1, borderBottomColor: "rgba(255,255,255,0.06)" },
  txIcon: { width: 36, height: 36, borderRadius: radius.md, alignItems: "center", justifyContent: "center" },
  txInfo: { flex: 1 },
  txTitle: { fontSize: typography.sm, fontWeight: "600", color: colors.body },
  txSub: { fontSize: typography.xs, color: colors.dim, marginTop: 1 },
  txAmount: { fontSize: typography.sm, fontWeight: "700" },
  modalOverlay: { flex: 1, justifyContent: "flex-end", backgroundColor: "rgba(0,0,0,0.6)" },
  modalSheet: { backgroundColor: colors.surface, borderTopLeftRadius: radius.xxl, borderTopRightRadius: radius.xxl, padding: spacing.lg, paddingBottom: spacing.xl },
  modalTitle: { fontSize: typography.md, fontWeight: "700", color: colors.head, marginBottom: spacing.lg },
  inputLabel: { fontSize: typography.sm, color: colors.dim, marginBottom: spacing.sm },
  input: {
    backgroundColor: colors.raised, borderWidth: 1, borderColor: colors.border,
    borderRadius: radius.lg, paddingHorizontal: spacing.md, paddingVertical: spacing.sm,
    fontSize: typography.base, color: colors.body, marginBottom: spacing.sm,
  },
  modalBtns: { flexDirection: "row", gap: spacing.sm, marginTop: spacing.sm },
  cancelBtn: { flex: 1, height: 48, borderRadius: radius.lg, borderWidth: 1, borderColor: colors.border, alignItems: "center", justifyContent: "center" },
  cancelBtnText: { color: colors.sub, fontSize: typography.base },
  confirmBtn: { flex: 1, height: 48, borderRadius: radius.lg, backgroundColor: colors.accent, alignItems: "center", justifyContent: "center" },
  confirmBtnText: { color: "#000", fontSize: typography.base, fontWeight: "700" },
  disabledBtn: { opacity: 0.5 },
  communityBtn: {
    flexDirection: "row", alignItems: "center", gap: 6,
    paddingVertical: spacing.sm, paddingHorizontal: spacing.md,
    borderRadius: radius.md, borderWidth: 1, borderColor: colors.accent,
    alignSelf: "flex-start", marginTop: spacing.md,
  },
  communityBtnText: { fontSize: typography.sm, color: colors.accent, fontWeight: "600" },
});
