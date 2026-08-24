import React, { useCallback, useEffect, useState } from "react";
import { View, Text, StyleSheet, ScrollView, TouchableOpacity, ActivityIndicator, Alert } from "react-native";
import { router, useLocalSearchParams } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { mpColors, mpSpacing, mpRadius, mpTypography } from "@/lib/marketplace-theme";
import { getListing, purchaseListing, type Listing } from "@/lib/marketplace-api";
import { useStore } from "@/lib/store";

function fmtPrice(raw: string): string {
  try {
    const n = BigInt(raw);
    return (n / 1000000000000000000n).toString();
  } catch {
    return raw;
  }
}

const STATUS_LABELS: Record<string, string> = {
  active: "Satışta", pending_purchase: "İşlemde", sold: "Satıldı", removed: "Kaldırıldı", flagged: "İncelemede",
};

export default function MarketplaceListingScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const insets = useSafeAreaInsets();
  const { user } = useStore();
  const [listing, setListing] = useState<Listing | null>(null);
  const [loading, setLoading] = useState(true);
  const [purchasing, setPurchasing] = useState(false);
  const [loadError, setLoadError] = useState(false);

  const load = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    try {
      const l = await getListing(id);
      setListing(l);
      setLoadError(false);
    } catch {
      setLoadError(true);
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => { load(); }, [load]);

  const isOwnListing = listing?.seller_did === user?.did;
  const canPurchase = listing?.status === "active" && !isOwnListing;

  const handlePurchase = useCallback(async () => {
    if (!listing) return;
    setPurchasing(true);
    try {
      const result = await purchaseListing(listing.id);
      Alert.alert(
        "Satın alındı",
        "Ödeme escrow'da tutuluyor. Teslimatı onaylayınca satıcıya geçecek.",
        [{ text: "Siparişe git", onPress: () => router.replace(`/(main)/marketplace-order/${result.transaction_id}` as any) }]
      );
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Satın alma başarısız.");
    } finally {
      setPurchasing(false);
    }
  }, [listing]);

  if (loading) {
    return (
      <View style={[styles.container, styles.center, { paddingTop: insets.top }]}>
        <ActivityIndicator color={mpColors.accent} size="large" />
      </View>
    );
  }

  if (loadError || !listing) {
    return (
      <View style={[styles.container, styles.center, { paddingTop: insets.top }]}>
        <Ionicons name="alert-circle-outline" size={40} color={mpColors.dim} />
        <Text style={styles.emptyText}>İlan bulunamadı</Text>
        <TouchableOpacity onPress={() => router.back()} style={styles.backLink}>
          <Text style={styles.backLinkText}>Geri dön</Text>
        </TouchableOpacity>
      </View>
    );
  }

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={mpColors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle} numberOfLines={1}>İlan</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.scroll}>
        <View style={styles.statusRow}>
          <View style={[styles.statusPill, listing.status !== "active" && styles.statusPillClosed]}>
            <Text style={styles.statusPillText}>{STATUS_LABELS[listing.status] || listing.status}</Text>
          </View>
          <View style={styles.categoryPill}>
            <Text style={styles.categoryPillText}>{listing.category}</Text>
          </View>
        </View>

        <Text style={styles.title}>{listing.title}</Text>
        <Text style={styles.price}>{fmtPrice(listing.price)} OBS</Text>
        <Text style={styles.description}>{listing.description}</Text>

        <View style={styles.sellerRow}>
          <Ionicons name="person-circle-outline" size={18} color={mpColors.dim} />
          <Text style={styles.sellerText} numberOfLines={1}>{listing.seller_did}</Text>
        </View>
      </ScrollView>

      <View style={[styles.footer, { paddingBottom: insets.bottom + mpSpacing.sm }]}>
        {isOwnListing ? (
          <View style={styles.ownNote}>
            <Text style={styles.ownNoteText}>Bu sizin ilanınız</Text>
          </View>
        ) : (
          <TouchableOpacity
            style={[styles.purchaseBtn, !canPurchase && styles.purchaseBtnDisabled]}
            onPress={handlePurchase}
            disabled={!canPurchase || purchasing}
          >
            {purchasing ? (
              <ActivityIndicator color={mpColors.void} />
            ) : (
              <Text style={styles.purchaseBtnText}>
                {canPurchase ? `Satın Al · ${fmtPrice(listing.price)} OBS` : "Satışta Değil"}
              </Text>
            )}
          </TouchableOpacity>
        )}
      </View>
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
  statusRow: { flexDirection: "row", gap: mpSpacing.xs, marginBottom: mpSpacing.md },
  statusPill: { paddingHorizontal: mpSpacing.sm, paddingVertical: 4, borderRadius: mpRadius.sm, backgroundColor: mpColors.accentDeep },
  statusPillClosed: { backgroundColor: mpColors.muted },
  statusPillText: { fontSize: 11, fontWeight: "700", color: mpColors.accent },
  categoryPill: { paddingHorizontal: mpSpacing.sm, paddingVertical: 4, borderRadius: mpRadius.sm, backgroundColor: mpColors.muted },
  categoryPillText: { fontSize: 11, fontWeight: "600", color: mpColors.sub },

  title: { fontSize: mpTypography.xl, fontWeight: "700", color: mpColors.head, marginBottom: mpSpacing.xs },
  price: { fontSize: mpTypography.lg, fontWeight: "700", color: mpColors.accent, marginBottom: mpSpacing.md },
  description: { fontSize: mpTypography.base, color: mpColors.body, lineHeight: 22, marginBottom: mpSpacing.lg },

  sellerRow: { flexDirection: "row", alignItems: "center", gap: mpSpacing.xs },
  sellerText: { fontSize: mpTypography.xs, color: mpColors.dim, flex: 1 },

  footer: { padding: mpSpacing.md, borderTopWidth: 1, borderTopColor: mpColors.border },
  purchaseBtn: {
    height: 52, borderRadius: mpRadius.full, backgroundColor: mpColors.accent,
    alignItems: "center", justifyContent: "center",
  },
  purchaseBtnDisabled: { backgroundColor: mpColors.muted },
  purchaseBtnText: { fontSize: mpTypography.base, fontWeight: "700", color: mpColors.void },
  ownNote: { height: 52, alignItems: "center", justifyContent: "center" },
  ownNoteText: { fontSize: mpTypography.sm, color: mpColors.dim },

  emptyText: { fontSize: mpTypography.base, color: mpColors.dim },
  backLink: { marginTop: mpSpacing.sm },
  backLinkText: { fontSize: mpTypography.sm, color: mpColors.accent },
});
