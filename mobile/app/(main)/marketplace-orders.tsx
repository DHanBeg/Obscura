import React, { useCallback, useEffect, useState } from "react";
import { View, Text, FlatList, TouchableOpacity, StyleSheet, ActivityIndicator, RefreshControl } from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { mpColors, mpSpacing, mpRadius, mpTypography } from "@/lib/marketplace-theme";
import { listMyTransactions, type Transaction } from "@/lib/marketplace-api";
import { useStore } from "@/lib/store";

const STATUS_LABELS: Record<string, string> = {
  held: "Beklemede (escrow)", released: "Tamamlandı", refunded: "İade edildi", completed: "Tamamlandı",
};
const STATUS_COLORS: Record<string, string> = {
  held: mpColors.amber, released: mpColors.positive, refunded: mpColors.red, completed: mpColors.positive,
};

function fmtPrice(raw: string): string {
  try {
    return (BigInt(raw) / 1000000000000000000n).toString();
  } catch {
    return raw;
  }
}

export default function MarketplaceOrdersScreen() {
  const insets = useSafeAreaInsets();
  const { user } = useStore();
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [loadError, setLoadError] = useState(false);

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true); else setLoading(true);
    try {
      const res = await listMyTransactions();
      setTransactions(res.transactions || []);
      setLoadError(false);
    } catch {
      setLoadError(true);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const renderItem = ({ item }: { item: Transaction }) => {
    const isBuyer = item.buyer_did === user?.did;
    return (
      <TouchableOpacity
        style={styles.card}
        activeOpacity={0.7}
        onPress={() => router.push(`/(main)/marketplace-order/${item.id}` as any)}
      >
        <View style={styles.cardTop}>
          <View style={styles.roleWrap}>
            <Ionicons name={isBuyer ? "arrow-down-circle-outline" : "arrow-up-circle-outline"} size={16} color={mpColors.dim} />
            <Text style={styles.roleText}>{isBuyer ? "Satın alım" : "Satış"}</Text>
          </View>
          <Text style={[styles.statusText, { color: STATUS_COLORS[item.status] || mpColors.dim }]}>
            {STATUS_LABELS[item.status] || item.status}
          </Text>
        </View>
        <Text style={styles.amount}>{fmtPrice(item.amount)} OBS</Text>
        <Text style={styles.date}>{new Date(item.created_at).toLocaleDateString("tr-TR")}</Text>
      </TouchableOpacity>
    );
  };

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={mpColors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Siparişlerim</Text>
        <View style={{ width: 40 }} />
      </View>

      {loading ? (
        <View style={styles.center}><ActivityIndicator color={mpColors.accent} size="large" /></View>
      ) : (
        <FlatList
          data={transactions}
          keyExtractor={(t) => t.id}
          renderItem={renderItem}
          contentContainerStyle={styles.list}
          refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => load(true)} tintColor={mpColors.accent} />}
          ListEmptyComponent={
            <View style={styles.emptyWrap}>
              <Ionicons name={loadError ? "cloud-offline-outline" : "receipt-outline"} size={40} color={mpColors.dim} />
              <Text style={styles.emptyText}>{loadError ? "Siparişler yüklenemedi" : "Henüz sipariş yok"}</Text>
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

  list: { padding: mpSpacing.md, paddingBottom: mpSpacing.xxl },
  card: {
    backgroundColor: mpColors.surface, borderRadius: mpRadius.lg,
    borderWidth: 1, borderColor: mpColors.border,
    padding: mpSpacing.md, marginBottom: mpSpacing.sm,
  },
  cardTop: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", marginBottom: 6 },
  roleWrap: { flexDirection: "row", alignItems: "center", gap: 6 },
  roleText: { fontSize: mpTypography.xs, color: mpColors.dim, fontWeight: "600" },
  statusText: { fontSize: mpTypography.xs, fontWeight: "700" },
  amount: { fontSize: mpTypography.lg, fontWeight: "700", color: mpColors.head },
  date: { fontSize: 11, color: mpColors.dim, marginTop: 2 },

  emptyWrap: { alignItems: "center", paddingTop: mpSpacing.xxl, gap: mpSpacing.sm },
  emptyText: { fontSize: mpTypography.base, color: mpColors.dim },
});
