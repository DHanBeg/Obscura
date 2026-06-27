import React, { useState, useEffect, useCallback } from "react";
import {
  View, Text, StyleSheet, ScrollView, TouchableOpacity,
  Modal, TextInput, ActivityIndicator, Alert, RefreshControl,
} from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors } from "@/lib/theme";
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
        <View style={styles.balanceCard}>
          <Text style={styles.balanceLabel}>OBS BAKİYE</Text>
          <View style={styles.balanceRow}>
            <Text style={styles.balanceAmount}>{fmt(balance?.transparent_balance || "0")}</Text>
            <Text style={styles.balanceCurrency}>OBS</Text>
          </View>
          {user?.did && (
            <Text style={styles.did} numberOfLines={1}>
              {user.did.slice(0, 20)}...{user.did.slice(-6)}
            </Text>
          )}
        </View>

        {/* Actions */}
        <View style={{ flexDirection: "row", gap: 10, marginBottom: 4 }}>
          <TouchableOpacity style={[styles.sendBtn, { flex: 1 }]} onPress={() => setModalVisible(true)}>
            <Ionicons name="arrow-up" size={18} color="#000" />
            <Text style={styles.sendBtnText}>Gönder</Text>
          </TouchableOpacity>
          <TouchableOpacity
            style={[styles.sendBtn, { flex: 1, backgroundColor: "rgba(74,222,128,0.12)", borderWidth: 1, borderColor: "rgba(74,222,128,0.3)" }]}
            onPress={() => router.push("/(main)/wallet-shielded" as any)}
          >
            <Ionicons name="shield-checkmark-outline" size={18} color={colors.accent} />
            <Text style={[styles.sendBtnText, { color: colors.accent }]}>Gizli Havuz</Text>
          </TouchableOpacity>
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
  scroll: { padding: 16, paddingBottom: 100 },
  title: { fontSize: 26, fontWeight: "700", color: colors.head, marginBottom: 16 },
  balanceCard: {
    backgroundColor: "rgba(99,102,241,0.12)",
    borderRadius: 20,
    padding: 20,
    marginBottom: 12,
    borderWidth: 1,
    borderColor: "rgba(99,102,241,0.2)",
  },
  balanceLabel: { fontSize: 11, color: colors.dim, letterSpacing: 1.5, fontWeight: "600", marginBottom: 8 },
  balanceRow: { flexDirection: "row", alignItems: "baseline", gap: 6 },
  balanceAmount: { fontSize: 38, fontWeight: "800", color: colors.head },
  balanceCurrency: { fontSize: 18, color: colors.dim, fontWeight: "500" },
  did: { fontSize: 11, color: colors.dim, fontFamily: "Courier", marginTop: 12 },
  sendBtn: {
    flexDirection: "row", alignItems: "center", justifyContent: "center",
    gap: 8, backgroundColor: colors.accent, borderRadius: 16,
    height: 48, marginBottom: 20,
  },
  sendBtnText: { fontSize: 15, fontWeight: "700", color: "#000" },
  sectionTitle: { fontSize: 11, color: colors.dim, letterSpacing: 1.5, fontWeight: "600", marginBottom: 8 },
  card: { backgroundColor: colors.surface, borderRadius: 20, borderWidth: 1, borderColor: colors.border, overflow: "hidden" },
  empty: { padding: 24, textAlign: "center", color: colors.dim, fontSize: 14 },
  txRow: { flexDirection: "row", alignItems: "center", padding: 14, gap: 10 },
  txBorder: { borderBottomWidth: 1, borderBottomColor: "rgba(255,255,255,0.06)" },
  txIcon: { width: 36, height: 36, borderRadius: 12, alignItems: "center", justifyContent: "center" },
  txInfo: { flex: 1 },
  txTitle: { fontSize: 13, fontWeight: "600", color: colors.body },
  txSub: { fontSize: 11, color: colors.dim, marginTop: 1 },
  txAmount: { fontSize: 13, fontWeight: "700" },
  modalOverlay: { flex: 1, justifyContent: "flex-end", backgroundColor: "rgba(0,0,0,0.6)" },
  modalSheet: { backgroundColor: colors.surface, borderTopLeftRadius: 28, borderTopRightRadius: 28, padding: 24, paddingBottom: 40 },
  modalTitle: { fontSize: 18, fontWeight: "700", color: colors.head, marginBottom: 20 },
  inputLabel: { fontSize: 12, color: colors.dim, marginBottom: 6 },
  input: {
    backgroundColor: colors.raised, borderWidth: 1, borderColor: colors.border,
    borderRadius: 14, paddingHorizontal: 14, paddingVertical: 12,
    fontSize: 14, color: colors.body, marginBottom: 12,
  },
  modalBtns: { flexDirection: "row", gap: 10, marginTop: 8 },
  cancelBtn: { flex: 1, height: 48, borderRadius: 14, borderWidth: 1, borderColor: colors.border, alignItems: "center", justifyContent: "center" },
  cancelBtnText: { color: colors.sub, fontSize: 15 },
  confirmBtn: { flex: 1, height: 48, borderRadius: 14, backgroundColor: colors.accent, alignItems: "center", justifyContent: "center" },
  confirmBtnText: { color: "#000", fontSize: 15, fontWeight: "700" },
  disabledBtn: { opacity: 0.5 },
  red: "#ef4444",
});
