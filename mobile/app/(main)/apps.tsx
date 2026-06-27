import React, { useState, useEffect, useCallback } from "react";
import {
  View, Text, StyleSheet, FlatList, TouchableOpacity,
  ActivityIndicator, Alert, RefreshControl,
} from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors } from "@/lib/theme";
import { api } from "@/lib/api";
import { useStore } from "@/lib/store";

interface MiniApp {
  id: string;
  name: string;
  description: string;
  version: string;
  author_did: string;
  tier_required: number;
  installed?: boolean;
}

const TIER_NAMES = ["Ücretsiz", "Bronze", "Silver", "Gold", "Platinum", "Diamond"];
const TIER_COLORS = [colors.dim, colors.tier1, colors.tier2, colors.tier3, colors.tier4, colors.tier5];

export default function AppsScreen() {
  const { user } = useStore();
  const [apps, setApps]     = useState<MiniApp[]>([]);
  const [loading, setLoading]     = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [busy, setBusy]           = useState<string | null>(null);

  const userTier = (user as any)?.tier ?? 0;

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true);
    else setLoading(true);
    try {
      const res = await api.listApps();
      setApps(res?.apps || []);
    } catch {}
    finally { setLoading(false); setRefreshing(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleInstall = async (app: MiniApp) => {
    if (app.tier_required > userTier) {
      Alert.alert(
        "Yetersiz Tier",
        `Bu uygulama ${TIER_NAMES[app.tier_required]} tier gerektirir.`,
      );
      return;
    }
    setBusy(app.id);
    try {
      await api.installApp(app.id);
      await load();
    } catch (e: any) {
      Alert.alert("Hata", e.message);
    } finally {
      setBusy(null);
    }
  };

  const handleUninstall = async (app: MiniApp) => {
    Alert.alert(
      "Kaldır",
      `"${app.name}" kaldırılsın mı?`,
      [
        { text: "İptal", style: "cancel" },
        {
          text: "Kaldır", style: "destructive",
          onPress: async () => {
            setBusy(app.id + "_rm");
            try {
              await api.uninstallApp(app.id);
              await load();
            } catch (e: any) {
              Alert.alert("Hata", e.message);
            } finally {
              setBusy(null);
            }
          },
        },
      ],
    );
  };

  const renderItem = ({ item, index }: { item: MiniApp; index: number }) => {
    const locked    = item.tier_required > userTier;
    const isBusy    = busy === item.id || busy === item.id + "_rm";
    const tierColor = TIER_COLORS[Math.min(item.tier_required, TIER_COLORS.length - 1)];

    return (
      <View style={[styles.row, index > 0 && styles.rowBorder]}>
        {/* App icon placeholder */}
        <View style={[styles.appIcon, locked && styles.appIconLocked]}>
          {locked
            ? <Ionicons name="lock-closed" size={18} color={colors.dim} />
            : <Ionicons name="apps" size={18} color={colors.accent} />
          }
        </View>

        {/* Info */}
        <View style={styles.rowInfo}>
          <View style={styles.rowHeader}>
            <Text style={[styles.appName, locked && styles.dimText]} numberOfLines={1}>{item.name}</Text>
            {item.tier_required > 0 && (
              <View style={[styles.tierBadge, { borderColor: tierColor + "55" }]}>
                <Text style={[styles.tierText, { color: tierColor }]}>{TIER_NAMES[item.tier_required]}</Text>
              </View>
            )}
          </View>
          <Text style={styles.appDesc} numberOfLines={2}>{item.description}</Text>
          <Text style={styles.appVersion}>v{item.version}</Text>
        </View>

        {/* Action */}
        {isBusy ? (
          <ActivityIndicator color={colors.accent} size="small" style={styles.actionBtn} />
        ) : item.installed ? (
          <TouchableOpacity
            style={[styles.actionBtn, styles.removeBtn]}
            onPress={() => handleUninstall(item)}
          >
            <Ionicons name="trash-outline" size={15} color={colors.red} />
          </TouchableOpacity>
        ) : (
          <TouchableOpacity
            style={[styles.actionBtn, styles.installBtn, locked && styles.lockedBtn]}
            onPress={() => handleInstall(item)}
            disabled={locked}
          >
            {locked
              ? <Ionicons name="lock-closed" size={14} color={colors.dim} />
              : <Ionicons name="download-outline" size={15} color={colors.accent} />
            }
          </TouchableOpacity>
        )}
      </View>
    );
  };

  if (loading) {
    return <View style={styles.center}><ActivityIndicator color={colors.accent} size="large" /></View>;
  }

  return (
    <View style={styles.container}>
      <FlatList
        data={apps}
        keyExtractor={a => a.id}
        renderItem={renderItem}
        contentContainerStyle={styles.list}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => load(true)} tintColor={colors.accent} />}
        ListHeaderComponent={
          <View style={styles.header}>
            <Text style={styles.title}>Mini Uygulamalar</Text>
            <Text style={styles.subtitle}>Tier seviyenize göre uygulamaları keşfedin ve kurun</Text>
            <View style={{ flexDirection: "row", gap: 8, marginTop: 12 }}>
              <TouchableOpacity
                style={{ flex: 1, flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 6, backgroundColor: "rgba(74,222,128,0.1)", borderRadius: 12, padding: 10, borderWidth: 1, borderColor: "rgba(74,222,128,0.25)" }}
                onPress={() => router.push("/(main)/apps-new" as any)}
              >
                <Ionicons name="add-circle-outline" size={16} color={colors.accent} />
                <Text style={{ fontSize: 13, fontWeight: "600", color: colors.accent }}>Yeni</Text>
              </TouchableOpacity>
              <TouchableOpacity
                style={{ flex: 1, flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 6, backgroundColor: colors.surface, borderRadius: 12, padding: 10, borderWidth: 1, borderColor: colors.border }}
                onPress={() => router.push("/(main)/apps-my" as any)}
              >
                <Ionicons name="cube-outline" size={16} color={colors.sub} />
                <Text style={{ fontSize: 13, fontWeight: "600", color: colors.sub }}>Uygulamalarım</Text>
              </TouchableOpacity>
              <TouchableOpacity
                style={{ flex: 1, flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 6, backgroundColor: colors.surface, borderRadius: 12, padding: 10, borderWidth: 1, borderColor: colors.border }}
                onPress={() => router.push("/(main)/apps-api-connect" as any)}
              >
                <Ionicons name="key-outline" size={16} color={colors.sub} />
                <Text style={{ fontSize: 13, fontWeight: "600", color: colors.sub }}>API</Text>
              </TouchableOpacity>
            </View>
          </View>
        }
        ListEmptyComponent={
          <View style={styles.emptyWrap}>
            <Ionicons name="apps" size={40} color={colors.dim} />
            <Text style={styles.emptyText}>Henüz uygulama yok</Text>
          </View>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container:   { flex: 1, backgroundColor: colors.void },
  center:      { flex: 1, alignItems: "center", justifyContent: "center", backgroundColor: colors.void },
  list:        { paddingBottom: 100 },

  header:      { padding: 16, paddingTop: 60, paddingBottom: 8 },
  title:       { fontSize: 26, fontWeight: "700", color: colors.head, marginBottom: 4 },
  subtitle:    { fontSize: 13, color: colors.dim },

  row:         { flexDirection: "row", alignItems: "center", paddingHorizontal: 16, paddingVertical: 12, gap: 12 },
  rowBorder:   { borderTopWidth: 1, borderTopColor: "rgba(255,255,255,0.05)" },
  rowInfo:     { flex: 1 },
  rowHeader:   { flexDirection: "row", alignItems: "center", gap: 6, marginBottom: 3 },

  appIcon: {
    width: 46, height: 46, borderRadius: 14,
    backgroundColor: colors.accentDeep,
    alignItems: "center", justifyContent: "center",
    borderWidth: 1, borderColor: colors.accentDim,
  },
  appIconLocked: { backgroundColor: colors.surface, borderColor: colors.border },

  appName:     { flex: 1, fontSize: 15, fontWeight: "700", color: colors.head },
  dimText:     { color: colors.sub },
  appDesc:     { fontSize: 12, color: colors.dim, lineHeight: 17 },
  appVersion:  { fontSize: 11, color: colors.dim, marginTop: 3 },

  tierBadge:   { borderRadius: 6, borderWidth: 1, paddingHorizontal: 5, paddingVertical: 1 },
  tierText:    { fontSize: 10, fontWeight: "700" },

  actionBtn:   { width: 38, height: 38, borderRadius: 12, alignItems: "center", justifyContent: "center" },
  installBtn:  { backgroundColor: colors.accentDeep, borderWidth: 1, borderColor: colors.accentDim },
  removeBtn:   { backgroundColor: "rgba(239,68,68,0.1)", borderWidth: 1, borderColor: "rgba(239,68,68,0.2)" },
  lockedBtn:   { backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.border },

  emptyWrap:   { alignItems: "center", paddingTop: 60, gap: 12 },
  emptyText:   { fontSize: 15, color: colors.dim },
});
