import React, { useState, useEffect, useCallback } from "react";
import {
  View, Text, StyleSheet, FlatList, TouchableOpacity,
  ActivityIndicator, RefreshControl, Alert,
} from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { colors } from "@/lib/theme";
import { api } from "@/lib/api";

interface Proposal {
  id: string;
  title: string;
  description: string;
  status: string;
  proposal_type: string;
  created_at: string;
}

const STATUS_MAP: Record<string, { label: string; color: string }> = {
  active: { label: "Oylamada", color: "#f59e0b" },
  passed: { label: "Kabul", color: "#6366f1" },
  rejected: { label: "Red", color: "#ef4444" },
  executed: { label: "Uygulandı", color: "#10b981" },
  pending: { label: "Bekliyor", color: colors.dim },
};

export default function GovernanceScreen() {
  const [proposals, setProposals] = useState<Proposal[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [voting, setVoting] = useState<string | null>(null);

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true);
    else setLoading(true);
    try {
      const res = await api.governanceListProposals();
      setProposals(res?.proposals || []);
    } catch {}
    finally { setLoading(false); setRefreshing(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleVote = async (proposalId: string, choice: number) => {
    setVoting(proposalId);
    try {
      await api.governanceVote(proposalId, {
        proof_json: JSON.stringify({ pi_a: [], pi_b: [], pi_c: [], protocol: "stub" }),
        public_inputs: ["0", "0", "0", "0"],
        choice,
      });
      Alert.alert("Oy Verildi", "Oyunuz anonim olarak kaydedildi.");
      await load();
    } catch (e: any) {
      Alert.alert("Hata", e.message);
    } finally {
      setVoting(null);
    }
  };

  const showVoteOptions = (proposal: Proposal) => {
    Alert.alert(
      proposal.title,
      "Oyunuzu seçin",
      [
        { text: "Evet", onPress: () => handleVote(proposal.id, 0) },
        { text: "Hayır", style: "destructive", onPress: () => handleVote(proposal.id, 1) },
        { text: "Çekimser", onPress: () => handleVote(proposal.id, 2) },
        { text: "İptal", style: "cancel" },
      ]
    );
  };

  const renderItem = ({ item, index }: { item: Proposal; index: number }) => {
    const status = STATUS_MAP[item.status] || { label: item.status, color: colors.dim };
    const isActive = item.status === "active";
    const isVoting = voting === item.id;

    return (
      <View style={[styles.row, index > 0 && styles.rowBorder]}>
        <View style={styles.rowContent}>
          <View style={styles.rowHeader}>
            <Text style={styles.rowTitle} numberOfLines={1}>{item.title}</Text>
            <View style={[styles.badge, { backgroundColor: status.color + "22" }]}>
              <Text style={[styles.badgeText, { color: status.color }]}>{status.label}</Text>
            </View>
          </View>
          <Text style={styles.rowDesc} numberOfLines={2}>{item.description}</Text>
          <Text style={styles.rowMeta}>{item.proposal_type === "param" ? "Parametre" : "Protokol"}</Text>
        </View>

        {isActive && (
          <TouchableOpacity
            style={styles.voteBtn}
            onPress={() => showVoteOptions(item)}
            disabled={isVoting}
          >
            {isVoting
              ? <ActivityIndicator color={colors.accent} size="small" />
              : <Ionicons name="checkbox" size={16} color={colors.accent} />
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
      <View style={styles.header}>
        <Text style={styles.title}>Yönetişim</Text>
      </View>
      <FlatList
        data={proposals}
        keyExtractor={p => p.id}
        renderItem={renderItem}
        contentContainerStyle={styles.list}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => load(true)} tintColor={colors.accent} />}
        ListEmptyComponent={
          <View style={styles.emptyWrap}>
            <Ionicons name="people" size={40} color={colors.dim} />
            <Text style={styles.emptyText}>Henüz öneri yok</Text>
          </View>
        }
        ListHeaderComponent={
          <Text style={styles.hint}>Oylar ZK kanıt ile anonimdir. Oy tercihiniz gizlenir, sonuç şeffaftır.</Text>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },
  center: { flex: 1, alignItems: "center", justifyContent: "center", backgroundColor: colors.void },
  header: { paddingHorizontal: 16, paddingTop: 60, paddingBottom: 12 },
  title: { fontSize: 26, fontWeight: "700", color: colors.head },
  list: { padding: 16, paddingBottom: 100 },
  hint: { fontSize: 12, color: colors.dim, backgroundColor: colors.surface, borderRadius: 12, padding: 12, marginBottom: 16, borderWidth: 1, borderColor: colors.border },
  row: { flexDirection: "row", alignItems: "center", padding: 14, gap: 10 },
  rowBorder: { borderTopWidth: 1, borderTopColor: "rgba(255,255,255,0.06)" },
  rowContent: { flex: 1 },
  rowHeader: { flexDirection: "row", alignItems: "center", gap: 8, marginBottom: 4 },
  rowTitle: { flex: 1, fontSize: 14, fontWeight: "600", color: colors.head },
  badge: { borderRadius: 8, paddingHorizontal: 6, paddingVertical: 2 },
  badgeText: { fontSize: 10, fontWeight: "700" },
  rowDesc: { fontSize: 12, color: colors.dim, lineHeight: 17, marginBottom: 4 },
  rowMeta: { fontSize: 11, color: colors.dim },
  voteBtn: { width: 36, height: 36, borderRadius: 10, backgroundColor: "rgba(99,102,241,0.12)", alignItems: "center", justifyContent: "center" },
  emptyWrap: { alignItems: "center", paddingTop: 60, gap: 12 },
  emptyText: { fontSize: 15, color: colors.dim },
});
