import React, { useState, useEffect, useCallback } from "react";
import {
  View, Text, FlatList, TouchableOpacity, StyleSheet,
  ActivityIndicator,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";

interface MiniApp {
  id: string;
  manifest?: { name?: string; description?: string; icon?: string };
  install_count?: number;
  installed?: boolean;
  publisher_did?: string;
}

const ICON_COLORS = [
  { bg: "rgba(74,222,128,0.15)", icon: "#4ade80" },
  { bg: "rgba(167,139,250,0.15)", icon: "#a78bfa" },
  { bg: "rgba(250,204,21,0.15)", icon: "#facc15" },
  { bg: "rgba(77,168,255,0.15)", icon: "#4da8ff" },
  { bg: "rgba(239,68,68,0.12)", icon: "#ef4444" },
];

const PLACEHOLDER_ICONS = ["flash-outline", "shield-outline", "wallet-outline", "chatbubble-outline", "bar-chart-outline"] as const;

function AppCard({ app, index, onEdit }: { app: MiniApp; index: number; onEdit: () => void }) {
  const ic = ICON_COLORS[index % ICON_COLORS.length];
  const iconName = PLACEHOLDER_ICONS[index % PLACEHOLDER_ICONS.length];
  const name = app.manifest?.name || "Uygulama";
  const desc = app.manifest?.description || "Açıklama yok";
  const users = app.install_count ?? 0;

  function formatCount(n: number) {
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
    if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
    return String(n);
  }

  const status = app.installed ? "Aktif" : users > 0 ? "İncelemede" : "Taslak";
  const statusColor = app.installed ? colors.accent : users > 0 ? colors.amber : colors.sub;

  return (
    <View style={styles.appCard}>
      <View style={[styles.appIconBox, { backgroundColor: ic.bg }]}>
        {app.manifest?.icon
          ? <Text style={{ fontSize: 20 }}>{app.manifest.icon}</Text>
          : <Ionicons name={iconName} size={20} color={ic.icon} />}
      </View>
      <View style={{ flex: 1, minWidth: 0 }}>
        <View style={styles.appNameRow}>
          <Text style={styles.appName} numberOfLines={1}>{name}</Text>
          <Text style={[styles.statusBadge, { color: statusColor }]}>{status}</Text>
        </View>
        <Text style={styles.appDesc} numberOfLines={1}>{desc}</Text>
        <View style={styles.appMeta}>
          <Ionicons name="people-outline" size={11} color={colors.sub} />
          <Text style={styles.appMetaText}>{formatCount(users)} günlük kullanıcı</Text>
        </View>
      </View>
      <TouchableOpacity onPress={onEdit} style={styles.editBtn}>
        <Ionicons name="pencil-outline" size={16} color={colors.sub} />
      </TouchableOpacity>
    </View>
  );
}

export default function AppsMyScreen() {
  const [apps, setApps] = useState<MiniApp[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const result = await api.listApps();
      const list = Array.isArray(result) ? result : result?.apps ?? [];
      setApps(list);
    } catch (e: any) {
      setError(e?.message || "Uygulamalar yüklenemedi");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Uygulamalarım</Text>
        <TouchableOpacity
          onPress={() => router.push("/(main)/apps-new")}
          style={styles.addBtn}
        >
          <Ionicons name="add" size={22} color={colors.head} />
        </TouchableOpacity>
      </View>

      {error ? (
        <View style={styles.errorBox}>
          <Text style={styles.errorText}>{error}</Text>
        </View>
      ) : null}

      {loading ? (
        <View style={styles.center}>
          <ActivityIndicator size="large" color={colors.accent} />
        </View>
      ) : apps.length === 0 ? (
        <View style={styles.emptyState}>
          <View style={styles.emptyIcon}>
            <Ionicons name="cube-outline" size={28} color={colors.sub} />
          </View>
          <Text style={styles.emptyTitle}>Henüz uygulama oluşturmadınız</Text>
          <Text style={styles.emptySub}>İlk mini uygulamanızı oluşturun ve mağazada yayınlayın.</Text>
          <TouchableOpacity style={styles.createBtn} onPress={() => router.push("/(main)/apps-new")}>
            <Ionicons name="add" size={16} color={colors.void} />
            <Text style={styles.createBtnText}>Yeni Uygulama Oluştur</Text>
          </TouchableOpacity>
        </View>
      ) : (
        <FlatList
          data={apps}
          keyExtractor={(item) => item.id}
          contentContainerStyle={styles.list}
          ListHeaderComponent={
            <Text style={styles.countLabel}>{apps.length} uygulama</Text>
          }
          renderItem={({ item, index }) => (
            <AppCard
              app={item}
              index={index}
              onEdit={() => router.push("/(main)/apps-new")}
            />
          )}
          ListFooterComponent={
            <TouchableOpacity
              style={styles.addMoreBtn}
              onPress={() => router.push("/(main)/apps-new")}
            >
              <Ionicons name="add" size={15} color={colors.sub} />
              <Text style={styles.addMoreText}>Yeni Uygulama Ekle</Text>
            </TouchableOpacity>
          }
        />
      )}
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
  addBtn: { padding: 4, width: 40, alignItems: "flex-end" },
  errorBox: {
    margin: spacing.lg, padding: spacing.md, borderRadius: radius.lg,
    backgroundColor: "rgba(239,68,68,0.08)", borderWidth: 1, borderColor: "rgba(239,68,68,0.2)",
  },
  errorText: { fontSize: typography.sm, color: colors.red },
  center: { flex: 1, alignItems: "center", justifyContent: "center" },
  emptyState: { flex: 1, alignItems: "center", justifyContent: "center", padding: spacing.xxl, gap: spacing.md },
  emptyIcon: {
    width: 64, height: 64, borderRadius: radius.xl, backgroundColor: colors.surface,
    borderWidth: 1, borderColor: colors.border, alignItems: "center", justifyContent: "center",
  },
  emptyTitle: { fontSize: typography.base, fontWeight: "600", color: colors.head, textAlign: "center" },
  emptySub: { fontSize: typography.sm, color: colors.sub, textAlign: "center", lineHeight: 20 },
  createBtn: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm,
    backgroundColor: colors.accent, borderRadius: radius.lg,
    paddingHorizontal: spacing.xl, paddingVertical: spacing.md,
  },
  createBtnText: { fontSize: typography.base, fontWeight: "700", color: colors.void },

  list: { padding: spacing.lg, gap: spacing.sm },
  countLabel: {
    fontSize: typography.xs, color: colors.sub, fontWeight: "600",
    textTransform: "uppercase", letterSpacing: 1, marginBottom: spacing.sm,
  },
  appCard: {
    flexDirection: "row", alignItems: "center", gap: spacing.md,
    backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border, padding: spacing.md,
  },
  appIconBox: {
    width: 48, height: 48, borderRadius: radius.lg,
    alignItems: "center", justifyContent: "center",
    borderWidth: 1, borderColor: colors.border,
  },
  appNameRow: { flexDirection: "row", alignItems: "center", gap: spacing.sm, marginBottom: 2 },
  appName: { fontSize: typography.sm, fontWeight: "700", color: colors.head, flex: 1 },
  statusBadge: { fontSize: 10, fontWeight: "600" },
  appDesc: { fontSize: typography.xs, color: colors.sub, marginBottom: 4 },
  appMeta: { flexDirection: "row", alignItems: "center", gap: spacing.xs },
  appMetaText: { fontSize: 11, color: colors.sub },
  editBtn: { padding: spacing.sm },

  addMoreBtn: {
    flexDirection: "row", alignItems: "center", justifyContent: "center",
    gap: spacing.sm, marginTop: spacing.sm,
    borderRadius: radius.lg, padding: spacing.md,
    borderWidth: 1, borderColor: colors.border, borderStyle: "dashed",
    backgroundColor: colors.surface,
  },
  addMoreText: { fontSize: typography.sm, fontWeight: "600", color: colors.sub },
});
