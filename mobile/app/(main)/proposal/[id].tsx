import React, { useState, useEffect, useCallback } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, StyleSheet,
  ActivityIndicator, Alert,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router, useLocalSearchParams } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";

interface Tally { yes: number; no: number; abstain: number; veto: number; total: number; }
interface Proposal {
  id: string; title: string; description: string;
  proposal_type: string; status: string; created_at: string;
  voting_ends_at?: string; execution_at?: string;
  proposer_did: string; execution_payload?: string;
}

const CHOICES = [
  { value: 0, label: "Evet", icon: "thumbs-up-outline", activeColor: colors.accent, activeBg: "rgba(74,222,128,0.08)" },
  { value: 1, label: "Hayır", icon: "thumbs-down-outline", activeColor: colors.red, activeBg: "rgba(239,68,68,0.08)" },
  { value: 2, label: "Çekimser", icon: "remove-outline", activeColor: colors.sub, activeBg: colors.raised },
  { value: 3, label: "Veto", icon: "warning-outline", activeColor: colors.amber, activeBg: "rgba(245,158,11,0.08)" },
] as const;

function formatTime(iso: string) {
  try { return new Date(iso).toLocaleDateString("tr-TR", { day: "numeric", month: "short", year: "numeric" }); }
  catch { return iso; }
}

function TallyBar({ tally }: { tally: Tally }) {
  const pct = (n: number) => tally.total ? Math.round((n / tally.total) * 100) : 0;
  return (
    <View>
      <View style={styles.tallyBarTrack}>
        <View style={[styles.tallySegment, { flex: pct(tally.yes), backgroundColor: colors.accent }]} />
        <View style={[styles.tallySegment, { flex: pct(tally.no), backgroundColor: colors.red }]} />
        <View style={[styles.tallySegment, { flex: pct(tally.abstain), backgroundColor: colors.muted }]} />
        <View style={[styles.tallySegment, { flex: pct(tally.veto), backgroundColor: colors.amber }]} />
      </View>
      <View style={styles.tallyGrid}>
        {[
          { label: "Evet", count: tally.yes, color: colors.accent },
          { label: "Hayır", count: tally.no, color: colors.red },
          { label: "Çekimser", count: tally.abstain, color: colors.sub },
          { label: "Veto", count: tally.veto, color: colors.amber },
        ].map(({ label, count, color }) => (
          <View key={label} style={styles.tallyCell}>
            <Text style={[styles.tallyCellCount, { color }]}>{count}</Text>
            <Text style={styles.tallyCellLabel}>{label}</Text>
          </View>
        ))}
      </View>
    </View>
  );
}

export default function ProposalDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const [proposal, setProposal] = useState<Proposal | null>(null);
  const [tally, setTally] = useState<Tally | null>(null);
  const [loading, setLoading] = useState(true);
  const [selectedChoice, setSelectedChoice] = useState<number | null>(null);
  const [voting, setVoting] = useState(false);
  const [voteSuccess, setVoteSuccess] = useState(false);
  const [finalizing, setFinalizing] = useState(false);

  const load = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    try {
      const res = await api.governanceGetProposal(id);
      setProposal(res?.proposal ?? null);
      setTally(res?.tally ?? null);
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Öneri yüklenemedi");
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => { load(); }, [load]);

  const handleVote = async () => {
    if (selectedChoice === null) return;
    setVoting(true);
    try {
      // Mobile uses stub proof — ZK circuit not available in RN yet
      await api.governanceVote(id, {
        proof_json: JSON.stringify({ stub: true }),
        public_inputs: ["0"],
        choice: selectedChoice,
      });
      setVoteSuccess(true);
      await load();
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Oy kullanılamadı");
    } finally {
      setVoting(false);
    }
  };

  const handleFinalize = async () => {
    Alert.alert("Oylama Sonuçlandır", "Oylama dönemini kapatmak istediğinizden emin misiniz?", [
      { text: "İptal", style: "cancel" },
      {
        text: "Sonuçlandır", style: "default",
        onPress: async () => {
          setFinalizing(true);
          try { await api.governanceFinalize(id); await load(); }
          catch (e: any) { Alert.alert("Hata", e?.message || "İşlem başarısız"); }
          finally { setFinalizing(false); }
        },
      },
    ]);
  };

  const handleExecute = async () => {
    Alert.alert("Öneriyi Uygula", "Bu işlem geri alınamaz. Devam etmek istiyor musunuz?", [
      { text: "İptal", style: "cancel" },
      {
        text: "Uygula", style: "destructive",
        onPress: async () => {
          setFinalizing(true);
          try { await api.governanceExecute(id); await load(); }
          catch (e: any) { Alert.alert("Hata", e?.message || "Uygulama başarısız"); }
          finally { setFinalizing(false); }
        },
      },
    ]);
  };

  const timelockHours = proposal?.execution_at
    ? Math.max(0, Math.round((new Date(proposal.execution_at).getTime() - Date.now()) / 3_600_000))
    : null;

  if (loading) {
    return (
      <SafeAreaView style={styles.root} edges={["top"]}>
        <View style={styles.header}>
          <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
            <Ionicons name="chevron-back" size={24} color={colors.head} />
          </TouchableOpacity>
          <Text style={styles.headerTitle}>Öneri</Text>
          <View style={{ width: 40 }} />
        </View>
        <View style={styles.center}>
          <ActivityIndicator size="large" color={colors.accent} />
        </View>
      </SafeAreaView>
    );
  }

  if (!proposal) {
    return (
      <SafeAreaView style={styles.root} edges={["top"]}>
        <View style={styles.header}>
          <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
            <Ionicons name="chevron-back" size={24} color={colors.head} />
          </TouchableOpacity>
          <Text style={styles.headerTitle}>Öneri</Text>
          <View style={{ width: 40 }} />
        </View>
        <View style={styles.center}>
          <Text style={styles.emptyText}>Öneri bulunamadı</Text>
        </View>
      </SafeAreaView>
    );
  }

  const isActive = proposal.status === "active";
  const isPassed = proposal.status === "passed";
  const statusColor = isActive ? colors.amber : isPassed ? colors.accent : colors.red;
  const statusLabel = isActive ? "Oylamada" : isPassed ? "Kabul Edildi" : "Reddedildi";

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle} numberOfLines={1}>Öneri</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        {/* Header */}
        <View style={styles.card}>
          <View style={styles.badgeRow}>
            <View style={styles.typeBadge}>
              <Text style={styles.typeBadgeText}>
                {proposal.proposal_type === "param" ? "Parametre" : "Protokol"}
              </Text>
            </View>
            <View style={[styles.statusBadge, { backgroundColor: `${statusColor}18`, borderColor: `${statusColor}40` }]}>
              <Text style={[styles.statusBadgeText, { color: statusColor }]}>{statusLabel}</Text>
            </View>
          </View>

          <Text style={styles.proposalTitle}>{proposal.title}</Text>
          <Text style={styles.proposalMeta}>
            {formatTime(proposal.created_at)} · {proposal.proposer_did?.slice(0, 18)}…
          </Text>
        </View>

        {/* Description */}
        <View style={styles.card}>
          <Text style={styles.proposalDesc}>{proposal.description}</Text>
        </View>

        {/* Tally */}
        {tally && (
          <View style={styles.card}>
            <View style={styles.tallyHeader}>
              <Text style={styles.sectionLabel}>OY SONUÇLARI</Text>
              <Text style={styles.tallyTotal}>{tally.total} oy</Text>
            </View>
            <TallyBar tally={tally} />
          </View>
        )}

        {/* Vote panel */}
        {isActive && !voteSuccess && (
          <View style={styles.card}>
            <View style={styles.voteHeader}>
              <Text style={styles.sectionLabel}>OYUNU KULLAN</Text>
              <View style={styles.zkBadge}>
                <Ionicons name="shield-checkmark-outline" size={10} color={colors.accent} />
                <Text style={styles.zkBadgeText}>ZK Anonim</Text>
              </View>
            </View>

            <View style={styles.choicesGrid}>
              {CHOICES.map((c) => {
                const active = selectedChoice === c.value;
                return (
                  <TouchableOpacity
                    key={c.value}
                    style={[
                      styles.choiceBtn,
                      active && { backgroundColor: c.activeBg, borderColor: c.activeColor + "66" },
                    ]}
                    onPress={() => setSelectedChoice(c.value)}
                  >
                    <Ionicons
                      name={c.icon as any}
                      size={16}
                      color={active ? c.activeColor : colors.sub}
                    />
                    <Text style={[styles.choiceBtnText, active && { color: c.activeColor }]}>
                      {c.label}
                    </Text>
                  </TouchableOpacity>
                );
              })}
            </View>

            <TouchableOpacity
              style={[styles.voteBtn, (voting || selectedChoice === null) && styles.voteBtnDisabled]}
              onPress={handleVote}
              disabled={voting || selectedChoice === null}
            >
              {voting
                ? <ActivityIndicator size="small" color={colors.void} />
                : <Ionicons name="checkmark-circle-outline" size={16} color={colors.void} />}
              <Text style={styles.voteBtnText}>
                {voting ? "ZK Kanıtı Oluşturuluyor…" : "ZK Kanıtı ile Oy Ver"}
              </Text>
            </TouchableOpacity>
          </View>
        )}

        {/* Vote success */}
        {voteSuccess && (
          <View style={styles.successBanner}>
            <Ionicons name="checkmark-circle" size={22} color={colors.accent} />
            <View>
              <Text style={styles.successTitle}>Oyunuz kaydedildi</Text>
              <Text style={styles.successSub}>ZK kanıtı ile anonim olarak işleme alındı.</Text>
            </View>
          </View>
        )}

        {/* Timelock */}
        {timelockHours !== null && isPassed && (
          <View style={styles.timelockBanner}>
            <Ionicons name="flash-outline" size={15} color={colors.amber} />
            <Text style={styles.timelockText}>
              Yürütme: <Text style={{ fontWeight: "700" }}>{timelockHours} saat sonra</Text> (48s timelock)
            </Text>
          </View>
        )}

        {/* Admin actions */}
        {(isActive || isPassed) && (
          <View style={styles.adminRow}>
            {isActive && (
              <TouchableOpacity
                style={[styles.adminBtn, finalizing && styles.adminBtnDisabled]}
                onPress={handleFinalize}
                disabled={finalizing}
              >
                {finalizing ? <ActivityIndicator size="small" color={colors.head} /> : null}
                <Text style={styles.adminBtnText}>Sonuçlandır</Text>
              </TouchableOpacity>
            )}
            {isPassed && (
              <TouchableOpacity
                style={[styles.adminBtnPrimary, finalizing && styles.adminBtnDisabled]}
                onPress={handleExecute}
                disabled={finalizing}
              >
                {finalizing ? <ActivityIndicator size="small" color={colors.void} /> : null}
                <Text style={styles.adminBtnPrimaryText}>Uygula</Text>
              </TouchableOpacity>
            )}
          </View>
        )}
      </ScrollView>
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
  headerTitle: { fontSize: typography.base, fontWeight: "700", color: colors.head, flex: 1, textAlign: "center" },
  center: { flex: 1, alignItems: "center", justifyContent: "center" },
  emptyText: { fontSize: typography.base, color: colors.sub },
  content: { padding: spacing.lg, gap: spacing.md, paddingBottom: 48 },

  card: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, gap: spacing.sm,
  },
  badgeRow: { flexDirection: "row", gap: spacing.sm, flexWrap: "wrap" },
  typeBadge: {
    paddingHorizontal: spacing.sm, paddingVertical: 3,
    backgroundColor: "rgba(77,168,255,0.1)", borderRadius: radius.full,
    borderWidth: 1, borderColor: "rgba(77,168,255,0.25)",
  },
  typeBadgeText: { fontSize: 11, fontWeight: "600", color: colors.tier2 },
  statusBadge: {
    paddingHorizontal: spacing.sm, paddingVertical: 3, borderRadius: radius.full, borderWidth: 1,
  },
  statusBadgeText: { fontSize: 11, fontWeight: "600" },
  proposalTitle: { fontSize: typography.lg, fontWeight: "700", color: colors.head, lineHeight: 26 },
  proposalMeta: { fontSize: typography.xs, color: colors.sub },
  proposalDesc: { fontSize: typography.sm, color: colors.body, lineHeight: 22 },

  tallyHeader: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  sectionLabel: { fontSize: typography.xs, color: colors.sub, fontWeight: "600", textTransform: "uppercase", letterSpacing: 1 },
  tallyTotal: { fontSize: typography.xs, color: colors.sub },
  tallyBarTrack: { flexDirection: "row", height: 16, borderRadius: radius.full, overflow: "hidden", backgroundColor: colors.raised, marginVertical: spacing.sm },
  tallySegment: {},
  tallyGrid: { flexDirection: "row", gap: spacing.sm },
  tallyCell: {
    flex: 1, backgroundColor: colors.raised, borderRadius: radius.md,
    padding: spacing.sm, alignItems: "center", gap: 2, borderWidth: 1, borderColor: colors.border,
  },
  tallyCellCount: { fontSize: typography.lg, fontWeight: "700" },
  tallyCellLabel: { fontSize: 10, color: colors.sub },

  voteHeader: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  zkBadge: {
    flexDirection: "row", alignItems: "center", gap: 3,
    paddingHorizontal: spacing.sm, paddingVertical: 2,
    backgroundColor: "rgba(74,222,128,0.08)", borderRadius: radius.full,
    borderWidth: 1, borderColor: "rgba(74,222,128,0.2)",
  },
  zkBadgeText: { fontSize: 10, color: colors.accent, fontWeight: "600" },
  choicesGrid: { flexDirection: "row", flexWrap: "wrap", gap: spacing.sm },
  choiceBtn: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm,
    flex: 1, minWidth: "44%", paddingHorizontal: spacing.md, paddingVertical: spacing.sm,
    borderRadius: radius.lg, backgroundColor: colors.raised, borderWidth: 1, borderColor: colors.border,
  },
  choiceBtnText: { fontSize: typography.sm, fontWeight: "600", color: colors.sub },
  voteBtn: {
    flexDirection: "row", alignItems: "center", justifyContent: "center", gap: spacing.sm,
    backgroundColor: colors.accent, borderRadius: radius.lg, padding: spacing.md,
  },
  voteBtnDisabled: { opacity: 0.4 },
  voteBtnText: { fontSize: typography.base, fontWeight: "700", color: colors.void },

  successBanner: {
    flexDirection: "row", alignItems: "flex-start", gap: spacing.sm,
    backgroundColor: "rgba(74,222,128,0.08)", borderRadius: radius.xl,
    borderWidth: 1, borderColor: "rgba(74,222,128,0.2)", padding: spacing.md,
  },
  successTitle: { fontSize: typography.sm, fontWeight: "600", color: colors.accent },
  successSub: { fontSize: typography.xs, color: colors.body, marginTop: 2 },

  timelockBanner: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm,
    backgroundColor: "rgba(245,158,11,0.06)", borderRadius: radius.lg,
    borderWidth: 1, borderColor: "rgba(245,158,11,0.15)", padding: spacing.md,
  },
  timelockText: { fontSize: typography.sm, color: colors.amber, flex: 1 },

  adminRow: { flexDirection: "row", gap: spacing.sm },
  adminBtn: {
    flex: 1, flexDirection: "row", alignItems: "center", justifyContent: "center", gap: spacing.sm,
    backgroundColor: colors.surface, borderRadius: radius.lg, padding: spacing.md,
    borderWidth: 1, borderColor: colors.border,
  },
  adminBtnDisabled: { opacity: 0.5 },
  adminBtnText: { fontSize: typography.base, fontWeight: "600", color: colors.head },
  adminBtnPrimary: {
    flex: 1, flexDirection: "row", alignItems: "center", justifyContent: "center", gap: spacing.sm,
    backgroundColor: colors.accent, borderRadius: radius.lg, padding: spacing.md,
  },
  adminBtnPrimaryText: { fontSize: typography.base, fontWeight: "700", color: colors.void },
});
