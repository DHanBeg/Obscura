import { useState, useEffect, useCallback } from "react";
import {
  View, Text, StyleSheet, FlatList, TouchableOpacity,
  ActivityIndicator, RefreshControl,
} from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, font, typography, shadow } from "@/lib/theme";
import { api } from "@/lib/api";

interface SequencerCandidate {
  node_id: string;
  did: string;
  stake: number;
  peer_addr?: string;
  registered_at: string;
}

interface Batch {
  id: string;
  sequencer_id: string;
  state_root: string;
  tx_count: number;
  submitted_at: string;
}

function timeSince(iso: string): string {
  const sec = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (sec < 60) return `${sec}s önce`;
  if (sec < 3600) return `${Math.floor(sec / 60)}d önce`;
  if (sec < 86400) return `${Math.floor(sec / 3600)}s önce`;
  return `${Math.floor(sec / 86400)}g önce`;
}

export default function SequencerScreen() {
  const [candidates, setCandidates] = useState<SequencerCandidate[]>([]);
  const [active, setActive]         = useState<SequencerCandidate | null>(null);
  const [batches, setBatches]       = useState<Batch[]>([]);
  const [loading, setLoading]       = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [tab, setTab]               = useState<"candidates" | "batches">("candidates");

  const load = useCallback(async (silent = false) => {
    if (!silent) setLoading(true);
    try {
      const [seqData, batchData] = await Promise.all([
        api.sequencerList(),
        api.sequencerBatches(),
      ]);
      setCandidates(seqData?.candidates || []);
      setActive(seqData?.active || null);
      setBatches(batchData?.batches || []);
    } catch {}
    finally { setLoading(false); setRefreshing(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const totalStake = candidates.reduce((s, c) => s + (c.stake || 0), 0);

  const renderCandidate = ({ item: c, index: i }: { item: SequencerCandidate; index: number }) => {
    const isActive = active?.node_id === c.node_id;
    const stakePct = totalStake > 0 ? Math.round((c.stake / totalStake) * 100) : 0;
    return (
      <View style={[styles.card, isActive && styles.cardActive]}>
        <View style={styles.cardRow}>
          <View style={{ flex: 1 }}>
            <View style={styles.cardRowInner}>
              <Text style={styles.rank}>#{i + 1}</Text>
              <Text style={styles.nodeId} numberOfLines={1}>{c.node_id}</Text>
              {isActive && (
                <View style={styles.activeBadge}>
                  <Text style={styles.activeBadgeText}>Aktif</Text>
                </View>
              )}
            </View>
            <Text style={styles.peerAddr} numberOfLines={1}>{c.peer_addr || "—"}</Text>
          </View>
          <View style={{ alignItems: "flex-end" }}>
            <Text style={styles.stakeAmt}>{c.stake.toLocaleString()}</Text>
            <Text style={styles.stakeLabel}>OBS</Text>
          </View>
        </View>
        <View style={styles.stakeBar}>
          <View style={[styles.stakeBarFill, { width: `${stakePct}%` as any }, isActive && styles.stakeBarFillActive]} />
        </View>
        <Text style={styles.stakeWeight}>%{stakePct} ağırlık · {timeSince(c.registered_at)}</Text>
      </View>
    );
  };

  const renderBatch = ({ item: b }: { item: Batch }) => (
    <View style={styles.card}>
      <View style={styles.cardRow}>
        <Ionicons name="checkmark-circle" size={16} color="#4ade80" />
        <View style={{ flex: 1, marginLeft: spacing.xs }}>
          <Text style={styles.stateRoot} numberOfLines={1}>{b.state_root}</Text>
          <Text style={styles.stakeLabel}>seq: {b.sequencer_id} · {b.tx_count} tx</Text>
        </View>
        <Text style={styles.stakeLabel}>{timeSince(b.submitted_at)}</Text>
      </View>
    </View>
  );

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <Text style={styles.title}>Sequencer</Text>
        <Text style={styles.subtitle}>VRF stake-ağırlıklı rotasyon · 4s epoch</Text>
      </View>

      {/* Active sequencer card */}
      {active && (
        <View style={styles.activeCard}>
          <View style={styles.activeCardHeader}>
            <View style={styles.activeDot} />
            <Text style={styles.activeLabel}>AKTİF SEQUENCER</Text>
          </View>
          <Text style={styles.activeNodeId}>{active.node_id}</Text>
          <Text style={styles.activeDid} numberOfLines={1}>{active.did}</Text>
          <View style={styles.activeStats}>
            <View>
              <Text style={styles.activeStatLabel}>Stake</Text>
              <Text style={styles.activeStatValue}>{active.stake.toLocaleString()} OBS</Text>
            </View>
            <View>
              <Text style={styles.activeStatLabel}>Kayıt</Text>
              <Text style={styles.activeStatValue}>{timeSince(active.registered_at)}</Text>
            </View>
          </View>
        </View>
      )}

      {/* Stats row */}
      <View style={styles.statsRow}>
        {[
          { label: "Aday", value: String(candidates.length) },
          { label: "Toplam Stake", value: `${(totalStake / 1000).toFixed(1)}k OBS` },
          { label: "Son Batch", value: batches.length > 0 ? timeSince(batches[0].submitted_at) : "—" },
        ].map(s => (
          <View key={s.label} style={styles.statCard}>
            <Text style={styles.statLabel}>{s.label}</Text>
            <Text style={styles.statValue}>{s.value}</Text>
          </View>
        ))}
      </View>

      {/* Tabs */}
      <View style={styles.tabs}>
        {(["candidates", "batches"] as const).map(t => (
          <TouchableOpacity
            key={t}
            style={[styles.tab, tab === t && styles.tabActive]}
            onPress={() => setTab(t)}
          >
            <Text style={[styles.tabText, tab === t && styles.tabTextActive]}>
              {t === "candidates" ? "Adaylar" : "Batch'ler"}
            </Text>
          </TouchableOpacity>
        ))}
      </View>

      {loading ? (
        <View style={styles.center}>
          <ActivityIndicator size="large" color={colors.accent} />
        </View>
      ) : tab === "candidates" ? (
        <FlatList
          data={candidates}
          keyExtractor={c => c.node_id}
          renderItem={renderCandidate}
          contentContainerStyle={{ padding: spacing.md, paddingBottom: spacing.xxl * 2 }}
          ItemSeparatorComponent={() => <View style={{ height: spacing.sm }} />}
          refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => { setRefreshing(true); load(true); }} tintColor={colors.accent} />}
          ListEmptyComponent={
            <View style={styles.center}>
              <Ionicons name="layers-outline" size={40} color={colors.dim} />
              <Text style={styles.emptyText}>Aday sequencer yok</Text>
            </View>
          }
        />
      ) : (
        <FlatList
          data={batches}
          keyExtractor={b => b.id}
          renderItem={renderBatch}
          contentContainerStyle={{ padding: spacing.md, paddingBottom: spacing.xxl * 2 }}
          ItemSeparatorComponent={() => <View style={{ height: spacing.sm }} />}
          refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => { setRefreshing(true); load(true); }} tintColor={colors.accent} />}
          ListEmptyComponent={
            <View style={styles.center}>
              <Ionicons name="cube-outline" size={40} color={colors.dim} />
              <Text style={styles.emptyText}>Henüz batch yok</Text>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },
  header: { padding: spacing.md, paddingTop: spacing.xl },
  title: { fontSize: typography.xl, fontWeight: "700", color: colors.head, fontFamily: font.bold },
  subtitle: { fontSize: typography.xs, color: colors.sub, marginTop: 2, fontFamily: font.regular },
  activeCard: { marginHorizontal: spacing.md, marginBottom: spacing.sm, backgroundColor: colors.accent + "0e", borderWidth: 1, borderColor: colors.accent + "33", borderRadius: radius.xl, padding: spacing.md, ...shadow.accent },
  activeCardHeader: { flexDirection: "row", alignItems: "center", gap: spacing.xs, marginBottom: spacing.xs },
  activeDot: { width: 8, height: 8, borderRadius: 4, backgroundColor: colors.accent },
  activeLabel: { fontSize: typography.xs, fontWeight: "700", color: colors.accent, letterSpacing: 1, fontFamily: font.bold },
  activeNodeId: { fontSize: typography.base, fontWeight: "700", color: colors.head, fontFamily: "monospace" },
  activeDid: { fontSize: typography.xs, color: colors.sub, fontFamily: "monospace", marginTop: 2 },
  activeStats: { flexDirection: "row", gap: spacing.lg, marginTop: spacing.sm },
  activeStatLabel: { fontSize: typography.xs, color: colors.dim, textTransform: "uppercase", letterSpacing: 0.5, fontFamily: font.regular },
  activeStatValue: { fontSize: typography.base, fontWeight: "600", color: colors.head, fontFamily: font.bold },
  statsRow: { flexDirection: "row", gap: spacing.sm, paddingHorizontal: spacing.md, marginBottom: spacing.sm },
  statCard: { flex: 1, backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.border, borderRadius: radius.md, padding: spacing.sm },
  statLabel: { fontSize: typography.xs, color: colors.dim, textTransform: "uppercase", letterSpacing: 0.5, fontFamily: font.regular },
  statValue: { fontSize: typography.base, fontWeight: "600", color: colors.head, marginTop: 2, fontFamily: font.bold },
  tabs: { flexDirection: "row", gap: spacing.sm, paddingHorizontal: spacing.md, marginBottom: spacing.sm },
  tab: { flex: 1, height: 36, borderRadius: radius.md, backgroundColor: colors.raised, alignItems: "center", justifyContent: "center" },
  tabActive: { backgroundColor: colors.accent + "18", borderWidth: 1, borderColor: colors.accent + "44" },
  tabText: { fontSize: typography.sm, fontWeight: "600", color: colors.sub, fontFamily: font.bold },
  tabTextActive: { color: colors.accent },
  card: { backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.border, borderRadius: radius.xl, padding: spacing.md },
  cardActive: { borderColor: colors.accent + "44" },
  cardRow: { flexDirection: "row", alignItems: "center", gap: spacing.xs },
  cardRowInner: { flexDirection: "row", alignItems: "center", gap: spacing.xs },
  rank: { fontSize: typography.xs, color: colors.dim, fontFamily: font.regular },
  nodeId: { fontSize: typography.sm, fontWeight: "600", color: colors.head, fontFamily: "monospace", flex: 1 },
  peerAddr: { fontSize: typography.xs, color: colors.sub, marginTop: 2, fontFamily: "monospace" },
  stakeAmt: { fontSize: typography.base, fontWeight: "700", color: colors.head, fontFamily: font.bold },
  stakeLabel: { fontSize: typography.xs, color: colors.dim, fontFamily: font.regular },
  stakeBar: { height: 4, borderRadius: 2, backgroundColor: colors.raised, overflow: "hidden", marginTop: spacing.xs },
  stakeBarFill: { height: "100%", backgroundColor: colors.accent + "55", borderRadius: 2 },
  stakeBarFillActive: { backgroundColor: colors.accent },
  stakeWeight: { fontSize: typography.xs, color: colors.dim, marginTop: 3, fontFamily: font.regular },
  activeBadge: { paddingHorizontal: 6, paddingVertical: 2, borderRadius: radius.full, backgroundColor: colors.accent + "20", borderWidth: 1, borderColor: colors.accent + "44" },
  activeBadgeText: { fontSize: 9, fontWeight: "700", color: colors.accent, fontFamily: font.bold },
  stateRoot: { fontSize: typography.xs, color: colors.head, fontFamily: "monospace" },
  center: { flex: 1, alignItems: "center", justifyContent: "center", paddingTop: spacing.xxl + spacing.xl, gap: spacing.sm },
  emptyText: { fontSize: typography.base, color: colors.sub, fontFamily: font.regular },
});
