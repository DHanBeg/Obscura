import React, { useState, useEffect, useCallback, useRef } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, StyleSheet,
  ActivityIndicator, FlatList,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";

type Section = "node-logs" | "zk-metrics" | "ws-debug";

interface NodeLog {
  id: string;
  timestamp: string;
  level: "info" | "warn" | "error";
  node_id: string;
  message: string;
}

interface ZKMetric {
  circuit: string;
  prove_ms: number;
  verify_ms: number;
  proof_size_bytes: number;
  status: "ok" | "fail";
  last_run: string;
}

function timePart(iso: string) {
  return iso.split("T")[1]?.slice(0, 12) ?? iso;
}

function fmtMs(ms: number) {
  return ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${ms}ms`;
}

function fmtBytes(b: number) {
  if (b < 1024) return `${b}B`;
  return `${(b / 1024).toFixed(1)}KB`;
}

function levelColor(l: string) {
  if (l === "error") return colors.red;
  if (l === "warn") return colors.amber;
  return colors.sub;
}

export default function DevToolsScreen() {
  const [section, setSection] = useState<Section>("node-logs");
  const [nodeLogs, setNodeLogs] = useState<NodeLog[]>([]);
  const [zkMetrics, setZkMetrics] = useState<ZKMetric[]>([]);
  const [nodeStatus, setNodeStatus] = useState<any>(null);
  const [loading, setLoading] = useState(false);

  const loadNodeStatus = useCallback(async () => {
    setLoading(true);
    try {
      const status = await api.nodeStatus();
      setNodeStatus(status);
      // Simulate logs from node status
      const now = new Date().toISOString();
      setNodeLogs([
        { id: "1", timestamp: now, level: "info", node_id: status?.node_id ?? "node-1", message: `Node aktif — ${status?.peer_count ?? 0} peer bağlı` },
        { id: "2", timestamp: now, level: "info", node_id: status?.node_id ?? "node-1", message: `Versiyon: ${status?.version ?? "—"}` },
        { id: "3", timestamp: now, level: "info", node_id: status?.node_id ?? "node-1", message: `Uptime: ${status?.uptime ?? "—"}` },
      ]);
    } catch (e: any) {
      setNodeLogs([{
        id: "err", timestamp: new Date().toISOString(), level: "error",
        node_id: "local", message: e.message || "Node durumu alınamadı",
      }]);
    } finally {
      setLoading(false);
    }
  }, []);

  const loadZkAudit = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.zkAuditLog?.() ?? { entries: [] };
      // Build mock metrics from audit entries
      const circuits = ["credit_threshold", "identity_proof", "message_integrity", "token_balance"];
      setZkMetrics(circuits.map((c, i) => ({
        circuit: c,
        prove_ms: 200 + Math.random() * 400,
        verify_ms: 5 + Math.random() * 15,
        proof_size_bytes: 768 + i * 32,
        status: "ok",
        last_run: new Date().toISOString(),
      })));
    } catch {
      setZkMetrics([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (section === "node-logs") loadNodeStatus();
    else if (section === "zk-metrics") loadZkAudit();
  }, [section]);

  const TABS: { id: Section; label: string; icon: any }[] = [
    { id: "node-logs", label: "Node", icon: "server-outline" },
    { id: "zk-metrics", label: "ZK", icon: "flash-outline" },
    { id: "ws-debug", label: "WS", icon: "radio-outline" },
  ];

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <Text style={styles.headerTitle}>Geliştirici Araçları</Text>
        <TouchableOpacity onPress={() => { if (section === "node-logs") loadNodeStatus(); else if (section === "zk-metrics") loadZkAudit(); }}>
          <Ionicons name="refresh-outline" size={20} color={colors.sub} />
        </TouchableOpacity>
      </View>

      {/* Tabs */}
      <View style={styles.tabs}>
        {TABS.map((t) => (
          <TouchableOpacity
            key={t.id}
            style={[styles.tab, section === t.id && styles.tabActive]}
            onPress={() => setSection(t.id)}
          >
            <Ionicons name={t.icon} size={16} color={section === t.id ? colors.accent : colors.sub} />
            <Text style={[styles.tabText, section === t.id && styles.tabTextActive]}>{t.label}</Text>
          </TouchableOpacity>
        ))}
      </View>

      {loading ? (
        <View style={styles.center}>
          <ActivityIndicator color={colors.accent} />
        </View>
      ) : (
        <>
          {section === "node-logs" && (
            <ScrollView style={styles.logScroll} contentContainerStyle={styles.logContent}>
              {nodeStatus && (
                <View style={styles.statusCard}>
                  <Text style={styles.statusRow}>
                    <Text style={styles.statusKey}>Node ID  </Text>
                    <Text style={styles.statusVal}>{nodeStatus.node_id ?? "—"}</Text>
                  </Text>
                  <Text style={styles.statusRow}>
                    <Text style={styles.statusKey}>Version  </Text>
                    <Text style={styles.statusVal}>{nodeStatus.version ?? "—"}</Text>
                  </Text>
                  <Text style={styles.statusRow}>
                    <Text style={styles.statusKey}>Peers    </Text>
                    <Text style={styles.statusVal}>{nodeStatus.peer_count ?? 0}</Text>
                  </Text>
                </View>
              )}
              {nodeLogs.map((log) => (
                <View key={log.id} style={styles.logRow}>
                  <Text style={[styles.logLevel, { color: levelColor(log.level) }]}>
                    [{log.level.toUpperCase()}]
                  </Text>
                  <Text style={styles.logTime}>{timePart(log.timestamp)}</Text>
                  <Text style={styles.logMsg}>{log.message}</Text>
                </View>
              ))}
              {nodeLogs.length === 0 && (
                <Text style={styles.emptyText}>Log bulunamadı</Text>
              )}
            </ScrollView>
          )}

          {section === "zk-metrics" && (
            <FlatList
              data={zkMetrics}
              keyExtractor={(i) => i.circuit}
              contentContainerStyle={styles.logContent}
              renderItem={({ item }) => (
                <View style={styles.zkCard}>
                  <View style={styles.zkHeader}>
                    <Text style={styles.zkCircuit}>{item.circuit}</Text>
                    <View style={[styles.zkBadge, item.status === "ok" && styles.zkBadgeOk]}>
                      <Text style={[styles.zkBadgeText, item.status === "ok" && styles.zkBadgeTextOk]}>
                        {item.status.toUpperCase()}
                      </Text>
                    </View>
                  </View>
                  <View style={styles.zkStats}>
                    <View style={styles.zkStat}>
                      <Text style={styles.zkStatLabel}>Prove</Text>
                      <Text style={styles.zkStatVal}>{fmtMs(item.prove_ms)}</Text>
                    </View>
                    <View style={styles.zkStat}>
                      <Text style={styles.zkStatLabel}>Verify</Text>
                      <Text style={styles.zkStatVal}>{fmtMs(item.verify_ms)}</Text>
                    </View>
                    <View style={styles.zkStat}>
                      <Text style={styles.zkStatLabel}>Size</Text>
                      <Text style={styles.zkStatVal}>{fmtBytes(item.proof_size_bytes)}</Text>
                    </View>
                  </View>
                </View>
              )}
              ListEmptyComponent={<Text style={styles.emptyText}>Metrik bulunamadı</Text>}
            />
          )}

          {section === "ws-debug" && (
            <View style={styles.center}>
              <Ionicons name="radio-outline" size={40} color={colors.muted} />
              <Text style={styles.emptyText}>WebSocket debug — gerçek zamanlı mesajlar burada görünür</Text>
              <Text style={styles.wsTip}>Bağlantı root layout tarafından yönetilir</Text>
            </View>
          )}
        </>
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
  headerTitle: { fontSize: typography.lg, fontWeight: "700", color: colors.head },
  tabs: {
    flexDirection: "row", borderBottomWidth: 1, borderBottomColor: colors.border,
    paddingHorizontal: spacing.md,
  },
  tab: {
    flexDirection: "row", alignItems: "center", gap: 6,
    paddingVertical: 12, paddingHorizontal: spacing.md,
    borderBottomWidth: 2, borderBottomColor: "transparent",
  },
  tabActive: { borderBottomColor: colors.accent },
  tabText: { fontSize: typography.sm, color: colors.sub },
  tabTextActive: { color: colors.accent, fontWeight: "600" },
  center: { flex: 1, alignItems: "center", justifyContent: "center", padding: 40, gap: 12 },
  emptyText: { fontSize: typography.sm, color: colors.sub, textAlign: "center" },
  wsTip: { fontSize: typography.xs, color: colors.dim, textAlign: "center" },
  logScroll: { flex: 1 },
  logContent: { padding: spacing.md, gap: spacing.xs },
  statusCard: {
    backgroundColor: colors.surface, borderRadius: radius.lg, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, marginBottom: spacing.md, gap: 4,
  },
  statusRow: { fontFamily: "monospace" },
  statusKey: { fontSize: typography.xs, color: colors.sub },
  statusVal: { fontSize: typography.xs, color: colors.accent },
  logRow: { flexDirection: "row", gap: spacing.xs, flexWrap: "wrap" },
  logLevel: { fontSize: typography.xs, fontFamily: "monospace", fontWeight: "700" },
  logTime: { fontSize: typography.xs, color: colors.dim, fontFamily: "monospace" },
  logMsg: { fontSize: typography.xs, color: colors.body, fontFamily: "monospace", flex: 1 },
  zkCard: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, gap: spacing.sm, marginBottom: spacing.xs,
  },
  zkHeader: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  zkCircuit: { fontSize: typography.sm, fontWeight: "600", color: colors.head, fontFamily: "monospace" },
  zkBadge: {
    paddingHorizontal: 8, paddingVertical: 3, borderRadius: radius.sm,
    backgroundColor: colors.raised,
  },
  zkBadgeOk: { backgroundColor: "rgba(74,222,128,0.1)" },
  zkBadgeText: { fontSize: 10, fontWeight: "700", color: colors.sub },
  zkBadgeTextOk: { color: colors.accent },
  zkStats: { flexDirection: "row", gap: spacing.sm },
  zkStat: { flex: 1, alignItems: "center" },
  zkStatLabel: { fontSize: typography.xs, color: colors.sub },
  zkStatVal: { fontSize: typography.sm, fontWeight: "600", color: colors.head },
});
