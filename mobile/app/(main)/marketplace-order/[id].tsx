import React, { useCallback, useEffect, useState } from "react";
import { View, Text, StyleSheet, ScrollView, TouchableOpacity, ActivityIndicator, Alert, TextInput, Modal } from "react-native";
import { router, useLocalSearchParams } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { mpColors, mpSpacing, mpRadius, mpTypography } from "@/lib/marketplace-theme";
import { getTransaction, releaseTransaction, openDispute, getDispute, type Transaction, type Dispute } from "@/lib/marketplace-api";
import { asyncStore } from "@/lib/keyValueStore";
import { useStore } from "@/lib/store";

const STATUS_LABELS: Record<string, string> = {
  held: "Beklemede (escrow)", released: "Tamamlandı", refunded: "İade edildi", completed: "Tamamlandı",
};

function fmtPrice(raw: string): string {
  try {
    return (BigInt(raw) / 1000000000000000000n).toString();
  } catch {
    return raw;
  }
}

// #30 — GET /v1/marketplace/disputes/{id} sadece dispute ID biliniyorsa
// çalışır (dispute id, transaction id'den bağımsız üretiliyor, backend'de
// transaction→dispute'a giden bir liste endpoint'i yok — bkz. Faz 1
// envanteri). openDispute() başarılı olduğunda döndürdüğü id'yi burada
// yerel önbelleğe yazıyoruz ki ekran yeniden açıldığında dispute durumu
// hâlâ görülebilsin.
function disputeCacheKey(transactionId: string): string {
  return `obscura_mp_dispute_for_tx_${transactionId}`;
}

export default function MarketplaceOrderScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const insets = useSafeAreaInsets();
  const { user } = useStore();
  const [txn, setTxn] = useState<Transaction | null>(null);
  const [dispute, setDispute] = useState<Dispute | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [busy, setBusy] = useState(false);
  const [disputeModalOpen, setDisputeModalOpen] = useState(false);
  const [disputeReason, setDisputeReason] = useState("");

  const load = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    try {
      const t = await getTransaction(id);
      setTxn(t);
      setLoadError(false);

      const cachedDisputeId = await asyncStore.getItem(disputeCacheKey(id));
      if (cachedDisputeId) {
        try {
          const d = await getDispute(cachedDisputeId);
          setDispute(d);
        } catch { /* dispute id stale/erişilemez — sessizce yok say */ }
      }
    } catch {
      setLoadError(true);
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => { load(); }, [load]);

  const isBuyer = txn?.buyer_did === user?.did;
  const canAct = txn?.status === "held" && isBuyer && !dispute;

  const handleRelease = useCallback(async () => {
    if (!txn) return;
    Alert.alert("Teslimatı Onayla", "Ödeme satıcıya geçecek. Emin misiniz?", [
      { text: "İptal", style: "cancel" },
      {
        text: "Onayla", onPress: async () => {
          setBusy(true);
          try {
            await releaseTransaction(txn.id);
            await load();
          } catch (e: any) {
            Alert.alert("Hata", e?.message || "İşlem başarısız.");
          } finally {
            setBusy(false);
          }
        },
      },
    ]);
  }, [txn, load]);

  const handleOpenDispute = useCallback(async () => {
    if (!txn || !disputeReason.trim()) return;
    setBusy(true);
    try {
      const d = await openDispute(txn.id, disputeReason.trim());
      await asyncStore.setItem(disputeCacheKey(txn.id), d.id);
      setDispute(d);
      setDisputeModalOpen(false);
      setDisputeReason("");
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Dispute açılamadı.");
    } finally {
      setBusy(false);
    }
  }, [txn, disputeReason]);

  if (loading) {
    return (
      <View style={[styles.container, styles.center, { paddingTop: insets.top }]}>
        <ActivityIndicator color={mpColors.accent} size="large" />
      </View>
    );
  }

  if (loadError || !txn) {
    return (
      <View style={[styles.container, styles.center, { paddingTop: insets.top }]}>
        <Ionicons name="alert-circle-outline" size={40} color={mpColors.dim} />
        <Text style={styles.emptyText}>Sipariş bulunamadı</Text>
      </View>
    );
  }

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={mpColors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Sipariş Detayı</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.scroll}>
        <Text style={styles.amount}>{fmtPrice(txn.amount)} OBS</Text>
        <Text style={styles.status}>{STATUS_LABELS[txn.status] || txn.status}</Text>

        <View style={styles.infoCard}>
          <View style={styles.infoRow}>
            <Text style={styles.infoLabel}>Rol</Text>
            <Text style={styles.infoValue}>{isBuyer ? "Alıcı" : "Satıcı"}</Text>
          </View>
          <View style={styles.infoRow}>
            <Text style={styles.infoLabel}>{isBuyer ? "Satıcı" : "Alıcı"}</Text>
            <Text style={styles.infoValue} numberOfLines={1}>{isBuyer ? txn.seller_did : txn.buyer_did}</Text>
          </View>
          <View style={styles.infoRow}>
            <Text style={styles.infoLabel}>Tarih</Text>
            <Text style={styles.infoValue}>{new Date(txn.created_at).toLocaleString("tr-TR")}</Text>
          </View>
        </View>

        {dispute && (
          <View style={styles.disputeCard}>
            <View style={styles.disputeHeader}>
              <Ionicons name="warning-outline" size={16} color={mpColors.amber} />
              <Text style={styles.disputeTitle}>Dispute {dispute.status === "open" ? "açık" : "çözüldü"}</Text>
            </View>
            <Text style={styles.disputeReason}>{dispute.reason}</Text>
          </View>
        )}
      </ScrollView>

      {canAct && (
        <View style={[styles.footer, { paddingBottom: insets.bottom + mpSpacing.sm }]}>
          <TouchableOpacity style={styles.disputeBtn} onPress={() => setDisputeModalOpen(true)} disabled={busy}>
            <Text style={styles.disputeBtnText}>Dispute Aç</Text>
          </TouchableOpacity>
          <TouchableOpacity style={styles.releaseBtn} onPress={handleRelease} disabled={busy}>
            {busy ? <ActivityIndicator color={mpColors.void} /> : <Text style={styles.releaseBtnText}>Teslimatı Onayla</Text>}
          </TouchableOpacity>
        </View>
      )}

      <Modal visible={disputeModalOpen} transparent animationType="slide">
        <View style={styles.modalOverlay}>
          <View style={styles.modalSheet}>
            <Text style={styles.modalTitle}>Dispute Aç</Text>
            <TextInput
              style={styles.modalInput}
              value={disputeReason}
              onChangeText={setDisputeReason}
              placeholder="Sorunu açıklayın..."
              placeholderTextColor={mpColors.dim}
              multiline
            />
            <View style={styles.modalActions}>
              <TouchableOpacity style={styles.modalCancelBtn} onPress={() => setDisputeModalOpen(false)}>
                <Text style={styles.modalCancelText}>İptal</Text>
              </TouchableOpacity>
              <TouchableOpacity
                style={[styles.modalSubmitBtn, !disputeReason.trim() && styles.modalSubmitBtnDisabled]}
                onPress={handleOpenDispute}
                disabled={!disputeReason.trim() || busy}
              >
                {busy ? <ActivityIndicator color={mpColors.void} /> : <Text style={styles.modalSubmitText}>Gönder</Text>}
              </TouchableOpacity>
            </View>
          </View>
        </View>
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: mpColors.void },
  center: { flex: 1, alignItems: "center", justifyContent: "center", gap: mpSpacing.sm },
  header: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    paddingHorizontal: mpSpacing.lg, paddingVertical: mpSpacing.md,
    borderBottomWidth: 1, borderBottomColor: mpColors.border,
  },
  backBtn: { padding: 4, width: 40 },
  headerTitle: { flex: 1, textAlign: "center", fontSize: mpTypography.base, fontWeight: "700", color: mpColors.head },

  scroll: { padding: mpSpacing.lg, paddingBottom: mpSpacing.xxl },
  amount: { fontSize: mpTypography.xxl, fontWeight: "700", color: mpColors.head, textAlign: "center" },
  status: { fontSize: mpTypography.sm, color: mpColors.amber, textAlign: "center", fontWeight: "600", marginTop: 4, marginBottom: mpSpacing.lg },

  infoCard: {
    backgroundColor: mpColors.surface, borderRadius: mpRadius.lg, borderWidth: 1, borderColor: mpColors.border,
    padding: mpSpacing.md, gap: mpSpacing.sm,
  },
  infoRow: { flexDirection: "row", justifyContent: "space-between" },
  infoLabel: { fontSize: mpTypography.sm, color: mpColors.dim },
  infoValue: { fontSize: mpTypography.sm, color: mpColors.head, flexShrink: 1, marginLeft: mpSpacing.md },

  disputeCard: {
    marginTop: mpSpacing.md, backgroundColor: "rgba(245,158,11,0.08)", borderRadius: mpRadius.lg,
    borderWidth: 1, borderColor: "rgba(245,158,11,0.25)", padding: mpSpacing.md,
  },
  disputeHeader: { flexDirection: "row", alignItems: "center", gap: 6, marginBottom: 4 },
  disputeTitle: { fontSize: mpTypography.sm, fontWeight: "700", color: mpColors.amber },
  disputeReason: { fontSize: mpTypography.sm, color: mpColors.body },

  footer: { flexDirection: "row", gap: mpSpacing.sm, padding: mpSpacing.md, borderTopWidth: 1, borderTopColor: mpColors.border },
  disputeBtn: {
    flex: 1, height: 48, borderRadius: mpRadius.full, alignItems: "center", justifyContent: "center",
    borderWidth: 1, borderColor: mpColors.red,
  },
  disputeBtnText: { fontSize: mpTypography.sm, fontWeight: "700", color: mpColors.red },
  releaseBtn: { flex: 2, height: 48, borderRadius: mpRadius.full, backgroundColor: mpColors.accent, alignItems: "center", justifyContent: "center" },
  releaseBtnText: { fontSize: mpTypography.sm, fontWeight: "700", color: mpColors.void },

  emptyText: { fontSize: mpTypography.base, color: mpColors.dim },

  modalOverlay: { flex: 1, backgroundColor: "rgba(0,0,0,0.6)", justifyContent: "flex-end" },
  modalSheet: { backgroundColor: mpColors.surface, borderTopLeftRadius: mpRadius.xl, borderTopRightRadius: mpRadius.xl, padding: mpSpacing.lg },
  modalTitle: { fontSize: mpTypography.lg, fontWeight: "700", color: mpColors.head, marginBottom: mpSpacing.md },
  modalInput: {
    backgroundColor: mpColors.ground, borderRadius: mpRadius.md, borderWidth: 1, borderColor: mpColors.border,
    color: mpColors.head, padding: mpSpacing.md, minHeight: 90, textAlignVertical: "top", marginBottom: mpSpacing.md,
  },
  modalActions: { flexDirection: "row", gap: mpSpacing.sm },
  modalCancelBtn: { flex: 1, height: 46, borderRadius: mpRadius.full, alignItems: "center", justifyContent: "center", borderWidth: 1, borderColor: mpColors.border },
  modalCancelText: { fontSize: mpTypography.sm, fontWeight: "600", color: mpColors.dim },
  modalSubmitBtn: { flex: 1, height: 46, borderRadius: mpRadius.full, alignItems: "center", justifyContent: "center", backgroundColor: mpColors.accent },
  modalSubmitBtnDisabled: { backgroundColor: mpColors.muted },
  modalSubmitText: { fontSize: mpTypography.sm, fontWeight: "700", color: mpColors.void },
});
