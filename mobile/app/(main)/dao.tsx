import { useState, useEffect, useCallback } from "react";
import {
  View, Text, StyleSheet, FlatList, TouchableOpacity,
  ActivityIndicator, RefreshControl, TextInput, Alert, ScrollView,
} from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, font } from "@/lib/theme";
import { api } from "@/lib/api";

interface DAOProposal {
  id: string;
  title: string;
  description: string;
  category: string;
  status: string;
  yes_weight: number;
  no_weight: number;
  abstain_weight: number;
  veto_count: number;
  voting_ends_at: string;
}

const STATUS_COLORS: Record<string, string> = {
  active: colors.tier2,
  passed: colors.tier4,
  rejected: "#ef4444",
  timelocked: "#60a5fa",
  executed: colors.tier3,
  vetoed: "#f97316",
  draft: colors.dim,
};

const STATUS_LABELS: Record<string, string> = {
  active: "Oylamada", passed: "Kabul", rejected: "Red",
  timelocked: "Timelock", executed: "Uygulandı", vetoed: "Veto", draft: "Taslak",
};

const CATEGORY_LABELS: Record<string, string> = {
  param: "Parametre", protocol: "Protokol",
  treasury: "Hazine", guardian: "Guardian", emergency: "Acil",
};

function timeSince(iso: string): string {
  const sec = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (sec < 60) return `${sec}s`;
  if (sec < 3600) return `${Math.floor(sec / 60)}d`;
  if (sec < 86400) return `${Math.floor(sec / 3600)}s`;
  return `${Math.floor(sec / 86400)}g`;
}

export default function DAOScreen() {
  const [proposals, setProposals] = useState<DAOProposal[]>([]);
  const [loading, setLoading]     = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm]            = useState({ title: "", description: "", category: "param" });
  const [creating, setCreating]    = useState(false);
  const [actionBusy, setActionBusy] = useState<string | null>(null);

  const load = useCallback(async (silent = false) => {
    if (!silent) setLoading(true);
    try {
      const res = await api.daoListProposals();
      setProposals(res?.proposals || []);
    } catch {}
    finally { setLoading(false); setRefreshing(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleCreate = async () => {
    if (!form.title.trim() || !form.description.trim()) return;
    setCreating(true);
    try {
      await api.daoCreateProposal(form);
      setForm({ title: "", description: "", category: "param" });
      setShowCreate(false);
      await load(true);
    } catch (e: any) {
      Alert.alert("Hata", e.message);
    } finally {
      setCreating(false);
    }
  };

  const doAction = async (id: string, action: () => Promise<any>, label: string) => {
    setActionBusy(id + label);
    try {
      await action();
      await load(true);
    } catch (e: any) {
      Alert.alert("Hata", e.message);
    } finally {
      setActionBusy(null);
    }
  };

  const tallyTotal = (p: DAOProposal) => p.yes_weight + p.no_weight + p.abstain_weight || 1;
  const yesPct = (p: DAOProposal) => Math.round((p.yes_weight / tallyTotal(p)) * 100);
  const noPct  = (p: DAOProposal) => Math.round((p.no_weight  / tallyTotal(p)) * 100);

  const renderItem = ({ item: p }: { item: DAOProposal }) => {
    const statusColor = STATUS_COLORS[p.status] ?? colors.dim;
    const hasTally = (p.yes_weight + p.no_weight + p.abstain_weight) > 0;
    return (
      <View style={styles.card}>
        <View style={styles.cardHeader}>
          <View style={{ flex: 1 }}>
            <Text style={styles.cardTitle} numberOfLines={1}>{p.title}</Text>
            <View style={styles.badgeRow}>
              <View style={[styles.badge, { borderColor: statusColor + "44", backgroundColor: statusColor + "18" }]}>
                <Text style={[styles.badgeText, { color: statusColor }]}>{STATUS_LABELS[p.status] ?? p.status}</Text>
              </View>
              <View style={styles.categoryBadge}>
                <Text style={styles.categoryText}>{CATEGORY_LABELS[p.category] ?? p.category}</Text>
              </View>
            </View>
            <Text style={styles.cardDesc} numberOfLines={2}>{p.description}</Text>
          </View>
          {p.veto_count > 0 && (
            <View style={styles.vetoRow}>
              <Ionicons name="shield" size={11} color="#ef4444" />
              <Text style={styles.vetoText}>Veto ×{p.veto_count}</Text>
            </View>
          )}
        </View>

        {hasTally && (
          <View style={styles.tallyWrap}>
            <View style={styles.tallyBar}>
              <View style={[styles.tallyYes, { flex: yesPct(p) }]} />
              <View style={[styles.tallyNo, { flex: noPct(p) }]} />
            </View>
            <View style={styles.tallyLabels}>
              <Text style={styles.tallyLabel}>Evet %{yesPct(p)}</Text>
              <Text style={styles.tallyLabel}>Hayır %{noPct(p)}</Text>
            </View>
          </View>
        )}

        <Text style={styles.ends}>Bitiş: {timeSince(p.voting_ends_at)} kaldı</Text>

        {p.status === "active" && (
          <TouchableOpacity
            style={styles.actionBtn}
            disabled={!!actionBusy}
            onPress={() => doAction(p.id, () => api.daoFinalize(p.id), "finalize")}
          >
            <Ionicons name="checkmark-circle-outline" size={13} color={colors.dim} />
            <Text style={styles.actionBtnText}>Sonuçlandır</Text>
          </TouchableOpacity>
        )}
        {p.status === "timelocked" && (
          <View style={styles.actionRow}>
            <TouchableOpacity
              style={[styles.actionBtn, styles.actionBtnAccent, { flex: 1 }]}
              disabled={!!actionBusy}
              onPress={() => doAction(p.id, () => api.daoExecute(p.id), "execute")}
            >
              <Ionicons name="play-circle-outline" size={13} color={colors.accent} />
              <Text style={[styles.actionBtnText, { color: colors.accent }]}>Uygula</Text>
            </TouchableOpacity>
            <TouchableOpacity
              style={[styles.actionBtn, styles.actionBtnVeto, { flex: 1 }]}
              disabled={!!actionBusy}
              onPress={() => Alert.alert("Veto", "Öneriyi veto etmek istediğinize emin misiniz?", [
                { text: "İptal", style: "cancel" },
                { text: "Veto", style: "destructive", onPress: () => doAction(p.id, () => api.daoVeto(p.id, "Guardian veto"), "veto") },
              ])}
            >
              <Ionicons name="close-circle-outline" size={13} color="#ef4444" />
              <Text style={[styles.actionBtnText, { color: "#ef4444" }]}>Veto</Text>
            </TouchableOpacity>
          </View>
        )}
      </View>
    );
  };

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <View>
          <Text style={styles.title}>DAO Yönetimi</Text>
          <Text style={styles.subtitle}>Timelock · Guardian veto · Süper çoğunluk</Text>
        </View>
        <TouchableOpacity style={styles.addBtn} onPress={() => setShowCreate(v => !v)}>
          <Ionicons name={showCreate ? "close" : "add"} size={18} color={colors.accent} />
        </TouchableOpacity>
      </View>

      {showCreate && (
        <View style={styles.createCard}>
          <Text style={styles.createLabel}>YENİ ÖNERİ</Text>
          <TextInput
            style={styles.input}
            placeholder="Başlık"
            placeholderTextColor={colors.dim}
            value={form.title}
            onChangeText={t => setForm(f => ({ ...f, title: t }))}
          />
          <TextInput
            style={[styles.input, styles.inputMulti]}
            placeholder="Açıklama..."
            placeholderTextColor={colors.dim}
            value={form.description}
            onChangeText={t => setForm(f => ({ ...f, description: t }))}
            multiline
            numberOfLines={3}
            textAlignVertical="top"
          />
          <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ marginBottom: spacing.sm }}>
            <View style={{ flexDirection: "row", gap: spacing.xs }}>
              {Object.entries(CATEGORY_LABELS).map(([k, v]) => (
                <TouchableOpacity
                  key={k}
                  onPress={() => setForm(f => ({ ...f, category: k }))}
                  style={[
                    styles.catChip,
                    form.category === k && styles.catChipActive,
                  ]}
                >
                  <Text style={[styles.catChipText, form.category === k && { color: colors.accent }]}>{v}</Text>
                </TouchableOpacity>
              ))}
            </View>
          </ScrollView>
          <TouchableOpacity
            style={[styles.submitBtn, (creating || !form.title || !form.description) && { opacity: 0.4 }]}
            onPress={handleCreate}
            disabled={creating || !form.title.trim() || !form.description.trim()}
          >
            {creating
              ? <ActivityIndicator size="small" color={colors.void} />
              : <Text style={styles.submitBtnText}>Öneri Gönder</Text>
            }
          </TouchableOpacity>
        </View>
      )}

      {loading ? (
        <View style={styles.center}>
          <ActivityIndicator size="large" color={colors.accent} />
        </View>
      ) : (
        <FlatList
          data={proposals}
          keyExtractor={p => p.id}
          renderItem={renderItem}
          contentContainerStyle={{ padding: spacing.md, paddingBottom: 100 }}
          ItemSeparatorComponent={() => <View style={{ height: spacing.sm }} />}
          refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => { setRefreshing(true); load(true); }} tintColor={colors.accent} />}
          ListEmptyComponent={
            <View style={styles.center}>
              <Ionicons name="scale-outline" size={40} color={colors.dim} />
              <Text style={styles.emptyText}>Henüz öneri yok</Text>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },
  header: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", padding: spacing.md, paddingTop: spacing.xl },
  title: { fontSize: 22, fontWeight: "700", color: colors.head, fontFamily: font.bold },
  subtitle: { fontSize: 11, color: colors.sub, marginTop: 2, fontFamily: font.regular },
  addBtn: { width: 36, height: 36, borderRadius: 18, backgroundColor: colors.accent + "18", borderWidth: 1, borderColor: colors.accent + "44", alignItems: "center", justifyContent: "center" },
  createCard: { marginHorizontal: spacing.md, marginBottom: spacing.sm, backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.border, borderRadius: radius.xl, padding: spacing.md },
  createLabel: { fontSize: 10, fontWeight: "700", color: colors.sub, letterSpacing: 1, marginBottom: spacing.sm, fontFamily: font.bold },
  input: { backgroundColor: colors.raised, borderWidth: 1, borderColor: colors.border, borderRadius: radius.md, paddingHorizontal: spacing.md, paddingVertical: 10, fontSize: 14, color: colors.body, marginBottom: spacing.sm, fontFamily: font.regular },
  inputMulti: { height: 72, paddingTop: 10 },
  catChip: { paddingHorizontal: spacing.sm, paddingVertical: 5, borderRadius: radius.sm, borderWidth: 1, borderColor: colors.border, backgroundColor: colors.raised },
  catChipActive: { borderColor: colors.accent + "55", backgroundColor: colors.accent + "18" },
  catChipText: { fontSize: 11, color: colors.sub, fontFamily: font.regular },
  submitBtn: { height: 40, borderRadius: radius.md, backgroundColor: colors.accent, alignItems: "center", justifyContent: "center" },
  submitBtnText: { fontSize: 14, fontWeight: "700", color: colors.void, fontFamily: font.bold },
  card: { backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.border, borderRadius: radius.xl, padding: spacing.md },
  cardHeader: { flexDirection: "row", gap: spacing.sm },
  cardTitle: { fontSize: 14, fontWeight: "600", color: colors.head, fontFamily: font.bold },
  badgeRow: { flexDirection: "row", gap: spacing.xs, marginTop: 4, flexWrap: "wrap" },
  badge: { paddingHorizontal: 6, paddingVertical: 2, borderRadius: 20, borderWidth: 1 },
  badgeText: { fontSize: 10, fontWeight: "700", fontFamily: font.bold },
  categoryBadge: { paddingHorizontal: 6, paddingVertical: 2, borderRadius: 20, borderWidth: 1, borderColor: colors.border },
  categoryText: { fontSize: 10, color: colors.sub, fontFamily: font.regular },
  cardDesc: { fontSize: 12, color: colors.sub, marginTop: 4, lineHeight: 17, fontFamily: font.regular },
  vetoRow: { flexDirection: "row", gap: 3, alignItems: "center" },
  vetoText: { fontSize: 11, color: "#ef4444", fontFamily: font.regular },
  tallyWrap: { marginTop: spacing.sm },
  tallyBar: { height: 8, borderRadius: 4, backgroundColor: colors.raised, overflow: "hidden", flexDirection: "row" },
  tallyYes: { backgroundColor: "#4ade8088", height: "100%" },
  tallyNo: { backgroundColor: "#ef444488", height: "100%" },
  tallyLabels: { flexDirection: "row", justifyContent: "space-between", marginTop: 2 },
  tallyLabel: { fontSize: 10, color: colors.sub, fontFamily: font.regular },
  ends: { fontSize: 10, color: colors.dim, marginTop: spacing.xs, fontFamily: font.regular },
  actionRow: { flexDirection: "row", gap: spacing.xs, marginTop: spacing.sm },
  actionBtn: { flexDirection: "row", gap: 4, alignItems: "center", justifyContent: "center", height: 32, borderRadius: radius.sm, backgroundColor: colors.raised, marginTop: spacing.sm, paddingHorizontal: spacing.sm },
  actionBtnAccent: { backgroundColor: colors.accent + "18", borderWidth: 1, borderColor: colors.accent + "40", marginTop: 0 },
  actionBtnVeto: { backgroundColor: "#ef444418", borderWidth: 1, borderColor: "#ef444440", marginTop: 0 },
  actionBtnText: { fontSize: 12, fontWeight: "600", color: colors.sub, fontFamily: font.bold },
  center: { flex: 1, alignItems: "center", justifyContent: "center", paddingTop: 80, gap: spacing.sm },
  emptyText: { fontSize: 14, color: colors.sub, fontFamily: font.regular },
});
