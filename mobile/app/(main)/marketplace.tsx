import React, { useCallback, useEffect, useState } from "react";
import {
  View, Text, FlatList, TouchableOpacity, StyleSheet,
  TextInput, RefreshControl, ActivityIndicator,
} from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { mpColors, mpSpacing, mpRadius, mpTypography } from "@/lib/marketplace-theme";
import { listListings, type Listing } from "@/lib/marketplace-api";

const CATEGORIES = ["all", "goods", "services", "digital", "misc"] as const;
const CATEGORY_LABELS: Record<string, string> = {
  all: "Tümü", goods: "Ürün", services: "Hizmet", digital: "Dijital", misc: "Diğer",
};

// price alanı obs_token'ın en küçük birimi (18 ondalık) — wallet.tsx'in fmt()
// deseniyle aynı ölçek, ayrı bir sabit yerine burada da inline (tek satır,
// paylaşılan bir util'e taşımaya değecek kadar tekrar yok).
function fmtPrice(raw: string): string {
  try {
    const n = BigInt(raw);
    const whole = n / 1000000000000000000n;
    return whole.toString();
  } catch {
    return raw;
  }
}

export default function MarketplaceScreen() {
  const insets = useSafeAreaInsets();
  const [listings, setListings] = useState<Listing[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState<string>("all");
  const [loadError, setLoadError] = useState(false);

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true); else setLoading(true);
    try {
      const res = await listListings({ status: "active" });
      setListings(res.listings || []);
      setLoadError(false);
    } catch {
      setLoadError(true);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const filtered = listings.filter((l) => {
    if (category !== "all" && l.category !== category) return false;
    if (!search) return true;
    return (l.title + l.description).toLowerCase().includes(search.toLowerCase());
  });

  const renderItem = ({ item }: { item: Listing }) => (
    <TouchableOpacity
      style={styles.card}
      activeOpacity={0.7}
      onPress={() => router.push(`/(main)/marketplace/${item.id}` as any)}
    >
      <View style={styles.cardHeader}>
        <Text style={styles.cardTitle} numberOfLines={1}>{item.title}</Text>
        <Text style={styles.cardPrice}>{fmtPrice(item.price)} OBS</Text>
      </View>
      <Text style={styles.cardDesc} numberOfLines={2}>{item.description}</Text>
      <View style={styles.cardFooter}>
        <View style={styles.categoryPill}>
          <Text style={styles.categoryPillText}>{CATEGORY_LABELS[item.category] || item.category}</Text>
        </View>
      </View>
    </TouchableOpacity>
  );

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={mpColors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Pazar</Text>
        <TouchableOpacity
          style={styles.newBtn}
          onPress={() => router.push("/(main)/marketplace-new-listing" as any)}
        >
          <Ionicons name="add" size={22} color={mpColors.accent} />
        </TouchableOpacity>
      </View>

      <View style={styles.searchRow}>
        <Ionicons name="search" size={16} color={mpColors.dim} />
        <TextInput
          style={styles.searchInput}
          value={search}
          onChangeText={setSearch}
          placeholder="İlan ara..."
          placeholderTextColor={mpColors.dim}
        />
        <TouchableOpacity onPress={() => router.push("/(main)/marketplace-orders" as any)}>
          <Ionicons name="receipt-outline" size={20} color={mpColors.sub} />
        </TouchableOpacity>
      </View>

      <View style={styles.categoryRow}>
        {CATEGORIES.map((c) => (
          <TouchableOpacity
            key={c}
            style={[styles.categoryChip, category === c && styles.categoryChipActive]}
            onPress={() => setCategory(c)}
          >
            <Text style={[styles.categoryChipText, category === c && styles.categoryChipTextActive]}>
              {CATEGORY_LABELS[c]}
            </Text>
          </TouchableOpacity>
        ))}
      </View>

      {loading ? (
        <View style={styles.center}><ActivityIndicator color={mpColors.accent} size="large" /></View>
      ) : (
        <FlatList
          data={filtered}
          keyExtractor={(l) => l.id}
          renderItem={renderItem}
          contentContainerStyle={styles.list}
          refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => load(true)} tintColor={mpColors.accent} />}
          ListEmptyComponent={
            <View style={styles.emptyWrap}>
              <Ionicons name={loadError ? "cloud-offline-outline" : "storefront-outline"} size={40} color={mpColors.dim} />
              <Text style={styles.emptyText}>
                {loadError ? "İlanlar yüklenemedi" : "Henüz ilan yok"}
              </Text>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: mpColors.void },
  center: { flex: 1, alignItems: "center", justifyContent: "center" },
  header: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    paddingHorizontal: mpSpacing.lg, paddingVertical: mpSpacing.md,
    borderBottomWidth: 1, borderBottomColor: mpColors.border,
  },
  backBtn: { padding: 4, width: 40 },
  headerTitle: { fontSize: mpTypography.base, fontWeight: "700", color: mpColors.head },
  newBtn: { width: 40, alignItems: "flex-end", padding: 4 },

  searchRow: {
    flexDirection: "row", alignItems: "center", gap: mpSpacing.sm,
    marginHorizontal: mpSpacing.md, marginTop: mpSpacing.sm,
    backgroundColor: mpColors.surface, borderRadius: mpRadius.lg,
    borderWidth: 1, borderColor: mpColors.border,
    paddingHorizontal: mpSpacing.md, height: 44,
  },
  searchInput: { flex: 1, color: mpColors.head, fontSize: mpTypography.sm },

  categoryRow: {
    flexDirection: "row", gap: mpSpacing.xs,
    paddingHorizontal: mpSpacing.md, paddingVertical: mpSpacing.sm,
  },
  categoryChip: {
    paddingHorizontal: mpSpacing.sm, paddingVertical: 6,
    borderRadius: mpRadius.full, borderWidth: 1, borderColor: mpColors.border,
    backgroundColor: mpColors.surface,
  },
  categoryChipActive: { backgroundColor: mpColors.accentDeep, borderColor: mpColors.accentDim },
  categoryChipText: { fontSize: mpTypography.xs, color: mpColors.dim, fontWeight: "600" },
  categoryChipTextActive: { color: mpColors.accent },

  list: { padding: mpSpacing.md, paddingBottom: mpSpacing.xxl },
  card: {
    backgroundColor: mpColors.surface, borderRadius: mpRadius.lg,
    borderWidth: 1, borderColor: mpColors.border,
    padding: mpSpacing.md, marginBottom: mpSpacing.sm,
  },
  cardHeader: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", marginBottom: 4 },
  cardTitle: { flex: 1, fontSize: mpTypography.base, fontWeight: "700", color: mpColors.head, marginRight: mpSpacing.sm },
  cardPrice: { fontSize: mpTypography.sm, fontWeight: "700", color: mpColors.accent },
  cardDesc: { fontSize: mpTypography.sm, color: mpColors.dim, lineHeight: 18 },
  cardFooter: { flexDirection: "row", marginTop: mpSpacing.sm },
  categoryPill: {
    paddingHorizontal: mpSpacing.sm, paddingVertical: 3,
    borderRadius: mpRadius.sm, backgroundColor: mpColors.muted,
  },
  categoryPillText: { fontSize: 11, color: mpColors.sub, fontWeight: "600" },

  emptyWrap: { alignItems: "center", paddingTop: mpSpacing.xxl, gap: mpSpacing.sm },
  emptyText: { fontSize: mpTypography.base, color: mpColors.dim },
});
