import React, { useState, useEffect, useCallback } from "react";
import {
  View, Text, StyleSheet, FlatList, TouchableOpacity,
  ActivityIndicator, RefreshControl,
} from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";

// İlke 5 (docs/spec/obscura_denetim_topluluk_katmani.md Bölüm 0): sistem ön
// eleyici, ciddi kararı insan verir. Bu ekran o kararın verildiği yer.
// Backend zaten fail-closed yetkilendiriyor (OBSCURA_ADMIN_DIDS allowlist,
// bkz. AdminMiddleware) — burada ayrı bir client-side rol kontrolü yok, web
// (frontend/app/admin/review/page.tsx) ile AYNI yaklaşım: dene, 403 gelirse
// göster.

interface ReviewQueueItem {
  id: string;
  reason: string;
  status: string;
  source: "user_report" | "auto_scan";
  target_type?: string;
  target_id?: string;
  created_at: string;
  reporter_did?: string;
  reported_did?: string;
  category?: string;
  evidence_verified?: boolean;
}

const PAGE_SIZE = 20;

const SOURCE_META: Record<string, { label: string; icon: keyof typeof Ionicons.glyphMap; color: string }> = {
  user_report: { label: "Kullanıcı Şikayeti", icon: "flag-outline", color: colors.amber },
  auto_scan: { label: "Otomatik Tarama", icon: "hardware-chip-outline", color: colors.tier3 },
};

const CATEGORY_LABELS: Record<string, string> = {
  spam: "Spam",
  scam: "Dolandırıcılık",
  harassment: "Taciz",
  copyright: "Telif",
  illegal_sale: "Yasadışı Satış",
  child_safety: "Çocuk Güvenliği",
};

type ActionType = "dismiss" | "confirm_remove" | "confirm_warn";

function ReviewCard({
  item,
  busyAction,
  onAction,
}: {
  item: ReviewQueueItem;
  busyAction: ActionType | null;
  onAction: (action: ActionType) => void;
}) {
  const meta = SOURCE_META[item.source] ?? SOURCE_META.auto_scan;
  const anyBusy = busyAction !== null;

  return (
    <View style={styles.card}>
      <View style={styles.cardHeaderRow}>
        <View style={[styles.sourceIcon, { backgroundColor: meta.color + "18" }]}>
          <Ionicons name={meta.icon} size={15} color={meta.color} />
        </View>
        <View style={{ flex: 1 }}>
          <View style={styles.badgeRow}>
            <Text style={[styles.badge, { color: meta.color, borderColor: meta.color + "40" }]}>{meta.label}</Text>
            {item.category ? (
              <Text style={styles.badgeNeutral}>{CATEGORY_LABELS[item.category] ?? item.category}</Text>
            ) : null}
            {item.source === "user_report" && item.evidence_verified ? (
              <Text style={[styles.badge, { color: colors.accent, borderColor: colors.accentDim }]}>Kanıt doğrulandı</Text>
            ) : null}
          </View>
          <Text style={styles.reason}>{item.reason}</Text>
        </View>
      </View>

      {item.source === "user_report" ? (
        <View style={styles.metaBlock}>
          <Text style={styles.metaLine}>Şikayetçi: <Text style={styles.metaCode}>{item.reporter_did ? item.reporter_did.slice(0, 24) + "…" : "—"}</Text></Text>
          <Text style={styles.metaLine}>Hedef: <Text style={styles.metaCode}>{item.reported_did ? item.reported_did.slice(0, 24) + "…" : "—"}</Text></Text>
        </View>
      ) : (
        <View style={styles.metaBlock}>
          <Text style={styles.metaLine}>Hedef tipi: <Text style={styles.metaBold}>{item.target_type || "—"}</Text></Text>
          <Text style={styles.metaLine}>Hedef ID: <Text style={styles.metaCode}>{item.target_id ? item.target_id.slice(0, 24) + "…" : "—"}</Text></Text>
        </View>
      )}

      <Text style={styles.timestamp}>
        {new Date(item.created_at).toLocaleString("tr-TR", { dateStyle: "short", timeStyle: "short" })}
      </Text>

      <View style={styles.actionRow}>
        <TouchableOpacity
          style={[styles.actionBtn, { backgroundColor: "rgba(239,68,68,0.1)", borderColor: "rgba(239,68,68,0.25)" }]}
          disabled={anyBusy}
          onPress={() => onAction("confirm_remove")}
        >
          {busyAction === "confirm_remove" ? (
            <ActivityIndicator size="small" color={colors.red} />
          ) : (
            <Ionicons name="ban-outline" size={14} color={colors.red} />
          )}
          <Text style={[styles.actionText, { color: colors.red }]}>Kaldır</Text>
        </TouchableOpacity>

        {item.source === "user_report" && (
          <TouchableOpacity
            style={[styles.actionBtn, { backgroundColor: "rgba(245,158,11,0.1)", borderColor: "rgba(245,158,11,0.25)" }]}
            disabled={anyBusy}
            onPress={() => onAction("confirm_warn")}
          >
            {busyAction === "confirm_warn" ? (
              <ActivityIndicator size="small" color={colors.amber} />
            ) : (
              <Ionicons name="warning-outline" size={14} color={colors.amber} />
            )}
            <Text style={[styles.actionText, { color: colors.amber }]}>Uyar</Text>
          </TouchableOpacity>
        )}

        <TouchableOpacity
          style={[styles.actionBtn, { backgroundColor: colors.muted, borderColor: colors.border }]}
          disabled={anyBusy}
          onPress={() => onAction("dismiss")}
        >
          {busyAction === "dismiss" ? (
            <ActivityIndicator size="small" color={colors.head} />
          ) : (
            <Ionicons name="checkmark-circle-outline" size={14} color={colors.head} />
          )}
          <Text style={[styles.actionText, { color: colors.head }]}>Reddet</Text>
        </TouchableOpacity>
      </View>
    </View>
  );
}

export default function AdminReviewQueueScreen() {
  const insets = useSafeAreaInsets();
  const [items, setItems] = useState<ReviewQueueItem[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [busy, setBusy] = useState<{ id: string; action: ActionType } | null>(null);
  const [loadError, setLoadError] = useState("");
  const [actionError, setActionError] = useState("");

  const load = useCallback(async (newOffset: number, isRefresh = false) => {
    if (isRefresh) setRefreshing(true);
    else setLoading(true);
    setLoadError("");
    try {
      const res = await api.adminListReviewQueue({ status: "pending", limit: PAGE_SIZE, offset: newOffset });
      setItems(res?.items || []);
      setTotal(res?.total || 0);
      setOffset(newOffset);
    } catch (e: any) {
      // Backend 403'te "Bu işlem için yönetici yetkisi gerekli" gibi net bir
      // mesaj döndürüyor — web ile aynı, ayrı bir client-side yetki kontrolü
      // yazmadan hatayı doğrudan gösteriyoruz.
      setLoadError(e?.message || "İnceleme kuyruğu yüklenemedi");
      setItems([]);
      setTotal(0);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => { load(0); }, [load]);

  const handleAction = async (item: ReviewQueueItem, action: ActionType) => {
    setBusy({ id: item.id, action });
    setActionError("");
    try {
      await api.adminResolveReviewQueue(item.id, action);
      setItems((prev) => prev.filter((i) => i.id !== item.id));
      setTotal((t) => Math.max(0, t - 1));
    } catch (e: any) {
      setActionError(e?.message || "İşlem başarısız");
    } finally {
      setBusy(null);
    }
  };

  if (loading) {
    return <View style={styles.center}><ActivityIndicator color={colors.accent} size="large" /></View>;
  }

  if (loadError) {
    return (
      <View style={[styles.container, { paddingTop: insets.top }]}>
        <View style={styles.header}><Text style={styles.title}>İnceleme Kuyruğu</Text></View>
        <View style={styles.emptyWrap}>
          <Ionicons name="shield-outline" size={40} color={colors.red} />
          <Text style={styles.emptyTitle}>Yetkiniz yok</Text>
          <Text style={styles.emptyText}>{loadError}</Text>
        </View>
      </View>
    );
  }

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <FlatList
        data={items}
        keyExtractor={(i) => i.id}
        renderItem={({ item }) => (
          <ReviewCard
            item={item}
            busyAction={busy?.id === item.id ? busy.action : null}
            onAction={(action) => handleAction(item, action)}
          />
        )}
        contentContainerStyle={styles.list}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => load(offset, true)} tintColor={colors.accent} />}
        ListHeaderComponent={
          <View style={styles.header}>
            <Text style={styles.title}>İnceleme Kuyruğu</Text>
            <Text style={styles.subtitle}>İlke 5 — ciddi kararı insan verir</Text>
            <View style={styles.stats}>
              <View style={styles.statItem}>
                <Text style={styles.statValue}>{total}</Text>
                <Text style={styles.statLabel}>Bekleyen</Text>
              </View>
              <View style={[styles.statItem, styles.statDivider]}>
                <Text style={styles.statValue}>{items.length}</Text>
                <Text style={styles.statLabel}>Bu sayfa</Text>
              </View>
            </View>
            {actionError ? (
              <View style={styles.errorBanner}><Text style={styles.errorBannerText}>{actionError}</Text></View>
            ) : null}
          </View>
        }
        ListEmptyComponent={
          <View style={styles.emptyWrap}>
            <Ionicons name="checkmark-circle-outline" size={40} color={colors.dim} />
            <Text style={styles.emptyTitle}>Bekleyen kayıt yok</Text>
            <Text style={styles.emptyText}>İnceleme kuyruğu temiz.</Text>
          </View>
        }
        ListFooterComponent={
          total > PAGE_SIZE ? (
            <View style={styles.pager}>
              <TouchableOpacity
                disabled={offset === 0}
                onPress={() => load(Math.max(0, offset - PAGE_SIZE))}
                style={[styles.pagerBtn, offset === 0 && styles.pagerBtnDisabled]}
              >
                <Ionicons name="chevron-back" size={16} color={offset === 0 ? colors.dim : colors.head} />
              </TouchableOpacity>
              <Text style={styles.pagerText}>{offset + 1}–{Math.min(offset + PAGE_SIZE, total)} / {total}</Text>
              <TouchableOpacity
                disabled={offset + PAGE_SIZE >= total}
                onPress={() => load(offset + PAGE_SIZE)}
                style={[styles.pagerBtn, offset + PAGE_SIZE >= total && styles.pagerBtnDisabled]}
              >
                <Ionicons name="chevron-forward" size={16} color={offset + PAGE_SIZE >= total ? colors.dim : colors.head} />
              </TouchableOpacity>
            </View>
          ) : null
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },
  center: { flex: 1, alignItems: "center", justifyContent: "center", backgroundColor: colors.void },
  list: { paddingBottom: 100 },
  header: { padding: spacing.md, paddingTop: spacing.sm },
  title: { fontSize: 26, fontWeight: "700", color: colors.head },
  subtitle: { fontSize: 12, color: colors.dim, marginTop: 2, marginBottom: spacing.md },
  stats: {
    flexDirection: "row",
    backgroundColor: colors.surface,
    borderRadius: radius.xl,
    borderWidth: 1,
    borderColor: colors.border,
    overflow: "hidden",
    marginBottom: spacing.sm,
  },
  statItem: { flex: 1, alignItems: "center", paddingVertical: 12 },
  statDivider: { borderLeftWidth: 1, borderColor: colors.border },
  statValue: { fontSize: 20, fontWeight: "800", color: colors.head },
  statLabel: { fontSize: 11, color: colors.dim, marginTop: 2 },
  errorBanner: {
    backgroundColor: "rgba(239,68,68,0.08)", borderWidth: 1, borderColor: "rgba(239,68,68,0.2)",
    borderRadius: radius.lg, padding: spacing.sm, marginTop: spacing.sm,
  },
  errorBannerText: { color: colors.red, fontSize: 12 },

  card: {
    marginHorizontal: spacing.md, marginBottom: spacing.sm,
    backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border, padding: spacing.md,
  },
  cardHeaderRow: { flexDirection: "row", gap: spacing.sm, marginBottom: spacing.sm },
  sourceIcon: { width: 32, height: 32, borderRadius: radius.md, alignItems: "center", justifyContent: "center" },
  badgeRow: { flexDirection: "row", flexWrap: "wrap", gap: 6, marginBottom: 4 },
  badge: { fontSize: 10, fontWeight: "600", borderWidth: 1, borderRadius: radius.sm, paddingHorizontal: 6, paddingVertical: 2 },
  badgeNeutral: { fontSize: 10, fontWeight: "600", color: colors.dim, borderWidth: 1, borderColor: colors.border, borderRadius: radius.sm, paddingHorizontal: 6, paddingVertical: 2 },
  reason: { fontSize: 12, color: colors.body, lineHeight: 17 },
  metaBlock: { marginBottom: spacing.sm, gap: 2 },
  metaLine: { fontSize: 11, color: colors.dim },
  metaCode: { fontFamily: "Courier", color: colors.sub },
  metaBold: { fontWeight: "700", color: colors.sub },
  timestamp: { fontSize: 10, color: colors.dim, marginBottom: spacing.sm },
  actionRow: { flexDirection: "row", gap: spacing.xs },
  actionBtn: {
    flex: 1, flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 5,
    height: 36, borderRadius: radius.lg, borderWidth: 1,
  },
  actionText: { fontSize: 12, fontWeight: "600" },

  emptyWrap: { alignItems: "center", paddingTop: 60, gap: 8, paddingHorizontal: spacing.lg },
  emptyTitle: { fontSize: 15, fontWeight: "600", color: colors.head },
  emptyText: { fontSize: 12, color: colors.dim, textAlign: "center" },

  pager: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingHorizontal: spacing.md, marginTop: spacing.sm },
  pagerBtn: { width: 32, height: 32, borderRadius: radius.md, alignItems: "center", justifyContent: "center", backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.border },
  pagerBtnDisabled: { opacity: 0.4 },
  pagerText: { fontSize: 12, color: colors.dim },
});
