import React, { useState, useEffect, useCallback } from "react";
import {
  View, Text, FlatList, TouchableOpacity, StyleSheet,
  ActivityIndicator, Modal, TextInput, Alert,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";
import { useStore } from "@/lib/store";

interface Campaign {
  id: string;
  title: string;
  total_amount: string;
  per_claim: string;
  min_tier: number;
  claimed_count: number;
  max_claims: number;
  ends_at: string;
  status: string;
  user_claimed?: boolean;
}

function tierLabel(t: number) {
  return ["—", "Bronze", "Silver", "Gold", "Platinum", "Diamond"][t] ?? `Tier ${t}`;
}

function formatDate(iso: string) {
  try {
    return new Date(iso).toLocaleDateString("tr-TR", { day: "numeric", month: "short" });
  } catch {
    return iso;
  }
}

export default function AirdropScreen() {
  const { user } = useStore();
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [loading, setLoading] = useState(true);
  const [claiming, setClaiming] = useState<string | null>(null);
  const [pendingCampaign, setPendingCampaign] = useState<Campaign | null>(null);
  const [mnemonicInput, setMnemonicInput] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.airdropListCampaigns();
      setCampaigns(res?.campaigns || []);
    } catch {}
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleClaim = (campaign: Campaign) => {
    setPendingCampaign(campaign);
    setMnemonicInput("");
  };

  const submitClaim = async () => {
    if (!pendingCampaign || !mnemonicInput.trim()) return;
    const campaign = pendingCampaign;
    setPendingCampaign(null);
    setClaiming(campaign.id);
    try {
      const derived = await api.deriveIdentity(mnemonicInput.trim());
      const identitySecretHex: string = derived?.identity_secret_hex ?? "";
      const phone: string = user?.phone ?? "";
      if (!identitySecretHex || !phone) throw new Error("Kimlik türetme başarısız");
      await api.airdropClaim(campaign.id, { proof_json: identitySecretHex, public_inputs: [phone] });
      Alert.alert("Başarılı", `${campaign.per_claim} OBS talep edildi!`);
      await load();
    } catch (e: any) {
      Alert.alert("Hata", e.message || "Talep başarısız");
    } finally {
      setClaiming(null);
    }
  };

  const userTier = user?.tier || 0;

  const renderCampaign = ({ item }: { item: Campaign }) => {
    const locked = userTier < item.min_tier;
    const claimed = item.user_claimed;
    const full = item.claimed_count >= item.max_claims;
    const active = item.status === "active";
    const progress = item.max_claims > 0 ? item.claimed_count / item.max_claims : 0;

    return (
      <View style={[styles.card, (locked || claimed || !active) && styles.cardDim]}>
        <View style={styles.cardHeader}>
          <View style={styles.giftIcon}>
            <Ionicons name="gift-outline" size={20} color={claimed ? colors.sub : colors.accent} />
          </View>
          <View style={{ flex: 1 }}>
            <Text style={styles.cardTitle}>{item.title}</Text>
            <Text style={styles.cardSub}>
              {item.per_claim} OBS · {tierLabel(item.min_tier)} gerekli
            </Text>
          </View>
          {claimed && (
            <Ionicons name="checkmark-circle" size={20} color={colors.accent} />
          )}
          {locked && !claimed && (
            <Ionicons name="lock-closed-outline" size={18} color={colors.sub} />
          )}
        </View>

        {/* Progress bar */}
        <View style={styles.progressBg}>
          <View style={[styles.progressFill, { width: `${Math.min(progress * 100, 100)}%` as any }]} />
        </View>
        <Text style={styles.progressLabel}>
          {item.claimed_count}/{item.max_claims} talep · {formatDate(item.ends_at)}'e kadar
        </Text>

        {active && !locked && !claimed && !full && (
          <TouchableOpacity
            style={[styles.claimBtn, claiming === item.id && styles.claimBtnDim]}
            onPress={() => handleClaim(item)}
            disabled={claiming === item.id}
          >
            {claiming === item.id
              ? <ActivityIndicator size="small" color={colors.void} />
              : <Text style={styles.claimBtnText}>Talep Et</Text>}
          </TouchableOpacity>
        )}
        {(!active || full) && (
          <Text style={styles.statusLabel}>{full ? "Tükendi" : "Sona Erdi"}</Text>
        )}
      </View>
    );
  };

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <Text style={styles.headerTitle}>Airdrop</Text>
        <TouchableOpacity onPress={load} style={styles.refreshBtn}>
          <Ionicons name="refresh-outline" size={20} color={colors.sub} />
        </TouchableOpacity>
      </View>

      {loading ? (
        <View style={styles.center}>
          <ActivityIndicator color={colors.accent} />
        </View>
      ) : (
        <FlatList
          data={campaigns}
          keyExtractor={(i) => i.id}
          renderItem={renderCampaign}
          contentContainerStyle={styles.list}
          ListEmptyComponent={
            <View style={styles.center}>
              <Ionicons name="gift-outline" size={40} color={colors.muted} />
              <Text style={styles.emptyText}>Aktif kampanya yok</Text>
            </View>
          }
        />
      )}

      {/* Mnemonic Modal */}
      <Modal visible={!!pendingCampaign} transparent animationType="slide">
        <View style={styles.modalOverlay}>
          <View style={styles.modalBox}>
            <Text style={styles.modalTitle}>Kimlik Doğrula</Text>
            <Text style={styles.modalSub}>
              Talep için 12 kelimelik kurtarma ifadenizi girin.
            </Text>
            <TextInput
              style={styles.mnemonicInput}
              value={mnemonicInput}
              onChangeText={setMnemonicInput}
              placeholder="kelime1 kelime2 kelime3 ..."
              placeholderTextColor={colors.dim}
              multiline
              numberOfLines={3}
              autoCorrect={false}
              autoCapitalize="none"
            />
            <View style={styles.modalActions}>
              <TouchableOpacity
                style={styles.modalCancel}
                onPress={() => setPendingCampaign(null)}
              >
                <Text style={styles.modalCancelText}>İptal</Text>
              </TouchableOpacity>
              <TouchableOpacity style={styles.modalConfirm} onPress={submitClaim}>
                <Text style={styles.modalConfirmText}>Talep Et</Text>
              </TouchableOpacity>
            </View>
          </View>
        </View>
      </Modal>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: colors.void },
  header: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: spacing.lg,
    paddingVertical: spacing.md,
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
  },
  headerTitle: {
    fontSize: typography.lg,
    fontWeight: "700",
    color: colors.head,
  },
  refreshBtn: { padding: 4 },
  list: { padding: spacing.md, gap: spacing.sm },
  center: { flex: 1, alignItems: "center", justifyContent: "center", padding: 40, gap: 12 },
  emptyText: { fontSize: typography.sm, color: colors.sub },
  card: {
    backgroundColor: colors.surface,
    borderRadius: radius.xl,
    padding: spacing.md,
    borderWidth: 1,
    borderColor: colors.border,
    gap: spacing.sm,
  },
  cardDim: { opacity: 0.6 },
  cardHeader: { flexDirection: "row", alignItems: "center", gap: spacing.sm },
  giftIcon: {
    width: 40, height: 40, borderRadius: radius.lg,
    backgroundColor: "rgba(74,222,128,0.08)",
    alignItems: "center", justifyContent: "center",
  },
  cardTitle: { fontSize: typography.base, fontWeight: "600", color: colors.head },
  cardSub: { fontSize: typography.xs, color: colors.sub, marginTop: 2 },
  progressBg: { height: 4, backgroundColor: colors.raised, borderRadius: 2 },
  progressFill: { height: 4, backgroundColor: colors.accent, borderRadius: 2 },
  progressLabel: { fontSize: typography.xs, color: colors.dim },
  claimBtn: {
    height: 40, borderRadius: radius.md, backgroundColor: colors.accent,
    alignItems: "center", justifyContent: "center",
  },
  claimBtnDim: { opacity: 0.6 },
  claimBtnText: { fontSize: typography.sm, fontWeight: "700", color: colors.void },
  statusLabel: { fontSize: typography.xs, color: colors.sub, textAlign: "center" },
  modalOverlay: {
    flex: 1, backgroundColor: "rgba(0,0,0,0.7)",
    justifyContent: "flex-end",
  },
  modalBox: {
    backgroundColor: colors.ground, borderTopLeftRadius: radius.xxl,
    borderTopRightRadius: radius.xxl, padding: spacing.xl, gap: spacing.md,
  },
  modalTitle: { fontSize: typography.md, fontWeight: "700", color: colors.head },
  modalSub: { fontSize: typography.sm, color: colors.sub },
  mnemonicInput: {
    backgroundColor: colors.surface, borderRadius: radius.lg, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, color: colors.head,
    fontSize: typography.sm, minHeight: 80, textAlignVertical: "top",
  },
  modalActions: { flexDirection: "row", gap: spacing.sm },
  modalCancel: {
    flex: 1, height: 48, borderRadius: radius.lg, backgroundColor: colors.raised,
    alignItems: "center", justifyContent: "center",
  },
  modalCancelText: { fontSize: typography.sm, color: colors.body },
  modalConfirm: {
    flex: 1, height: 48, borderRadius: radius.lg, backgroundColor: colors.accent,
    alignItems: "center", justifyContent: "center",
  },
  modalConfirmText: { fontSize: typography.sm, fontWeight: "700", color: colors.void },
});
