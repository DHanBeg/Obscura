import React, { useState, useCallback, useEffect } from "react";
import {
  View, Text, TouchableOpacity, StyleSheet, FlatList, ActivityIndicator,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { useStore } from "@/lib/store";
import { getWelcomes, type PendingWelcome } from "@/lib/mls/mlsApi";
import { acceptMlsWelcome } from "@/lib/mls/joinGroupFlow";
import { ensureInvitable } from "@/lib/mls/inviteBootstrap";

// L2 Tuğla 5c — bekleyen MLS grup davetleri. Envanter (5c Adım 0): bu
// ekrandan ÖNCE davet/bildirim UI'ı hiç yoktu (getWelcomes/joinFromWelcomeWire
// app/ dizininde sıfır çağrı). Kabul akışının koordinasyon mantığı (sıra,
// local-önce, kısmi-başarısızlık) burada YOK — lib/mls/joinGroupFlow.ts'te,
// mock'larla test edilmiş. Bu ekran sadece listeler + tetikler.
export default function MlsInvitesScreen() {
  const { user } = useStore();
  const [welcomes, setWelcomes] = useState<PendingWelcome[]>([]);
  const [loading, setLoading] = useState(true);
  const [acceptingId, setAcceptingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [inviteBootstrapWarning, setInviteBootstrapWarning] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    // Bob'un kendi KeyPackage'ı backend'de yoksa (hiç yüklenmemiş ya da
    // tüketilmiş) başkaları onu davet EDEMEZ. _layout.tsx'te girişte sessizce
    // deneniyor (bkz. Tuğla 5c bölüm 2) — burada TEKRAR deneniyor ve
    // başarısızsa kullanıcıya GÖRÜNÜR bir uyarı veriliyor (fail-closed: sessiz
    // kalıp "davet edilebilirim" sanılmasına izin vermiyoruz).
    if (user) {
      try {
        await ensureInvitable(user.did);
        setInviteBootstrapWarning(null);
      } catch {
        setInviteBootstrapWarning("Davet edilebilir değilsiniz (KeyPackage yüklenemedi) — tekrar deneyin");
      }
    }
    try {
      const list = await getWelcomes();
      setWelcomes(list || []);
    } catch {
      setError("Davetler yüklenemedi");
    } finally {
      setLoading(false);
    }
  }, [user]);

  useEffect(() => { load(); }, [load]);

  const accept = useCallback(async (welcome: PendingWelcome) => {
    if (!user || acceptingId) return;
    setAcceptingId(welcome.id);
    setError(null);
    try {
      await acceptMlsWelcome({ ownDid: user.did, welcome });
      // Kabul edilen davet listeden düşer — grup artık conversations
      // listesinde görünür (Alice'in create-group çağrısı Bob'u zaten
      // conv_members'a eklemişti, bkz. 5b-2/5c envanteri).
      setWelcomes((prev) => prev.filter((w) => w.id !== welcome.id));
    } catch {
      // MÜHÜR: başarısızlıkta local state SİLİNMEZ (joinGroupFlow.ts) —
      // burada sadece kullanıcıya hata gösterilir, davet listede kalır,
      // tekrar denenebilir.
      setError("Davet kabul edilemedi, tekrar deneyin");
    } finally {
      setAcceptingId(null);
    }
  }, [user, acceptingId]);

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Grup Davetleri</Text>
        <View style={{ width: 40 }} />
      </View>

      {loading ? (
        <View style={styles.centerWrap}>
          <ActivityIndicator size="small" color={colors.accent} />
        </View>
      ) : (
        <FlatList
          data={welcomes}
          keyExtractor={(w) => w.id}
          renderItem={({ item }) => (
            <View style={styles.row}>
              <View style={styles.iconWrap}>
                <Ionicons name="people-outline" size={20} color={colors.accent} />
              </View>
              <Text style={styles.rowText} numberOfLines={1}>{item.group_id}</Text>
              <TouchableOpacity
                style={styles.acceptBtn}
                onPress={() => accept(item)}
                disabled={acceptingId === item.id}
              >
                {acceptingId === item.id
                  ? <ActivityIndicator size="small" color={colors.void} />
                  : <Text style={styles.acceptBtnText}>Katıl</Text>}
              </TouchableOpacity>
            </View>
          )}
          contentContainerStyle={{ paddingHorizontal: spacing.md }}
          ListEmptyComponent={
            <Text style={styles.noResult}>Bekleyen davet yok</Text>
          }
        />
      )}
      {inviteBootstrapWarning && <Text style={styles.errorText}>{inviteBootstrapWarning}</Text>}
      {error && <Text style={styles.errorText}>{error}</Text>}
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
  centerWrap: { flex: 1, alignItems: "center", justifyContent: "center" },
  row: {
    flexDirection: "row", alignItems: "center", gap: spacing.md,
    paddingVertical: 12, borderBottomWidth: 1, borderBottomColor: colors.border,
  },
  iconWrap: {
    width: 40, height: 40, borderRadius: 20,
    backgroundColor: "rgba(74,222,128,0.1)", alignItems: "center", justifyContent: "center",
  },
  rowText: { flex: 1, fontSize: typography.sm, color: colors.head },
  acceptBtn: {
    paddingHorizontal: spacing.md, paddingVertical: 8,
    backgroundColor: colors.accent, borderRadius: radius.full,
  },
  acceptBtnText: { fontSize: typography.xs, fontWeight: "700", color: colors.void },
  noResult: { textAlign: "center", padding: 24, color: colors.sub, fontSize: typography.sm },
  errorText: { textAlign: "center", padding: spacing.sm, color: colors.red, fontSize: typography.xs },
});
