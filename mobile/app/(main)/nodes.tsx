import React, { useState, useEffect, useCallback } from "react";
import {
  View, Text, StyleSheet, FlatList, TouchableOpacity,
  ActivityIndicator, RefreshControl,
} from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { colors } from "@/lib/theme";
import { api } from "@/lib/api";

interface FederationNode {
  node_id: string;
  peer_addr: string;
  version: string;
  region: string;
  last_seen: string;
  status: string;
}

function timeSince(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const secs = Math.floor(diff / 1000);
  if (secs < 60) return `${secs}s`;
  if (secs < 3600) return `${Math.floor(secs / 60)}dk`;
  if (secs < 86400) return `${Math.floor(secs / 3600)}sa`;
  return `${Math.floor(secs / 86400)}g`;
}

const STATUS_COLORS: Record<string, string> = {
  active:   colors.accent,
  inactive: colors.dim,
  banned:   colors.red,
};

export default function NodesScreen() {
  const [nodes, setNodes]       = useState<FederationNode[]>([]);
  const [loading, setLoading]   = useState(true);
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true);
    else setLoading(true);
    try {
      const res = await api.nodeList();
      setNodes(res?.nodes || []);
    } catch {}
    finally { setLoading(false); setRefreshing(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const active = nodes.filter(n => n.status === "active").length;

  const renderItem = ({ item, index }: { item: FederationNode; index: number }) => {
    const col = STATUS_COLORS[item.status] || colors.dim;
    return (
      <View style={[styles.row, index > 0 && styles.rowBorder]}>
        <View style={[styles.icon, { backgroundColor: col + "15" }]}>
          <Ionicons name="server" size={16} color={col} />
        </View>
        <View style={styles.rowInfo}>
          <View style={styles.rowHeader}>
            <Text style={styles.nodeID} numberOfLines={1}>{item.node_id.slice(0, 18)}…</Text>
            {item.version ? <Text style={styles.version}>v{item.version}</Text> : null}
          </View>
          <Text style={styles.region}>{item.region || "Bilinmiyor"} · {timeSince(item.last_seen)} önce</Text>
          {item.peer_addr ? <Text style={styles.addr} numberOfLines={1}>{item.peer_addr}</Text> : null}
        </View>
        <View style={[styles.dot, { backgroundColor: col }]} />
      </View>
    );
  };

  if (loading) {
    return <View style={styles.center}><ActivityIndicator color={colors.accent} size="large" /></View>;
  }

  return (
    <View style={styles.container}>
      <FlatList
        data={nodes}
        keyExtractor={n => n.node_id}
        renderItem={renderItem}
        contentContainerStyle={styles.list}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => load(true)} tintColor={colors.accent} />}
        ListHeaderComponent={
          <View style={styles.header}>
            <Text style={styles.title}>Node Explorer</Text>
            <View style={styles.stats}>
              <View style={styles.statItem}>
                <Text style={styles.statValue}>{nodes.length}</Text>
                <Text style={styles.statLabel}>Toplam</Text>
              </View>
              <View style={[styles.statItem, styles.statDivider]}>
                <Text style={[styles.statValue, { color: colors.accent }]}>{active}</Text>
                <Text style={styles.statLabel}>Aktif</Text>
              </View>
              <View style={styles.statItem}>
                <Text style={[styles.statValue, { color: colors.dim }]}>{nodes.length - active}</Text>
                <Text style={styles.statLabel}>İnaktif</Text>
              </View>
            </View>
          </View>
        }
        ListEmptyComponent={
          <View style={styles.emptyWrap}>
            <Ionicons name="globe-outline" size={40} color={colors.dim} />
            <Text style={styles.emptyText}>Henüz kayıtlı node yok</Text>
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
  header:      { padding: 16, paddingTop: 60 },
  title:       { fontSize: 26, fontWeight: "700", color: colors.head, marginBottom: 16 },
  stats: {
    flexDirection: "row",
    backgroundColor: colors.surface,
    borderRadius: 16,
    borderWidth: 1,
    borderColor: colors.border,
    marginBottom: 8,
    overflow: "hidden",
  },
  statItem:    { flex: 1, alignItems: "center", paddingVertical: 12 },
  statDivider: { borderLeftWidth: 1, borderRightWidth: 1, borderColor: colors.border },
  statValue:   { fontSize: 20, fontWeight: "800", color: colors.head },
  statLabel:   { fontSize: 11, color: colors.dim, marginTop: 2 },
  row:         { flexDirection: "row", alignItems: "center", paddingHorizontal: 16, paddingVertical: 12, gap: 12 },
  rowBorder:   { borderTopWidth: 1, borderTopColor: "rgba(255,255,255,0.05)" },
  icon:        { width: 38, height: 38, borderRadius: 12, alignItems: "center", justifyContent: "center" },
  rowInfo:     { flex: 1 },
  rowHeader:   { flexDirection: "row", alignItems: "center", gap: 8, marginBottom: 3 },
  nodeID:      { flex: 1, fontSize: 13, fontWeight: "700", color: colors.head, fontFamily: "Courier" },
  version:     { fontSize: 11, color: colors.dim },
  region:      { fontSize: 12, color: colors.dim },
  addr:        { fontSize: 11, color: colors.dim, fontFamily: "Courier", marginTop: 2 },
  dot:         { width: 8, height: 8, borderRadius: 4 },
  emptyWrap:   { alignItems: "center", paddingTop: 60, gap: 12 },
  emptyText:   { fontSize: 15, color: colors.dim },
});
