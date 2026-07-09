import React, { useState } from "react";
import { View, Text, TouchableOpacity, StyleSheet, ActivityIndicator, ScrollView } from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, typography, radius, shadow } from "@/lib/theme";
import { api } from "@/lib/api";

const HUB_ITEMS: { icon: React.ComponentProps<typeof Ionicons>["name"]; label: string; sub: string; route: string }[] = [
  { icon: "podium-outline", label: "DAO", sub: "Öneriler ve oylama", route: "/(main)/dao" },
  { icon: "layers-outline", label: "Sequencer", sub: "Ağ sıralayıcı durumu", route: "/(main)/sequencer" },
  { icon: "trending-up-outline", label: "Staking", sub: "OBS kilitle, ödül kazan", route: "/(main)/staking" },
  { icon: "hardware-chip-outline", label: "Node Gezgini", sub: "Ağdaki node'lar", route: "/(main)/nodes" },
  { icon: "swap-horizontal-outline", label: "Bridge", sub: "Zincirler arası transfer", route: "/(main)/bridge" },
];

export default function GovernanceScreen() {
  const insets = useSafeAreaInsets();
  const [joining, setJoining] = useState(false);
  const [joinError, setJoinError] = useState("");

  const joinCommunity = async () => {
    setJoining(true);
    setJoinError("");
    try {
      const { conv_id } = await api.joinOBSCommunity();
      router.push(`/(main)/chat/${conv_id}` as any);
    } catch (e: any) {
      setJoinError(
        e?.message?.includes("en az 1 OBS") || e?.message?.includes("403")
          ? "Bu alana erişmek için en az 1 OBS gereklidir."
          : "Bağlantı hatası. Tekrar dene."
      );
    } finally {
      setJoining(false);
    }
  };

  return (
    <View style={[styles.root, { paddingTop: insets.top }]}>
      <ScrollView contentContainerStyle={styles.scroll}>
        <Text style={styles.title}>Yönetim</Text>

        {/* OBS Topluluk — artık ekrana girer girmez değil, dokununca katılıyor */}
        <TouchableOpacity style={styles.communityCard} onPress={joinCommunity} disabled={joining}>
          <Ionicons name="people-outline" size={22} color={colors.accent} />
          <View style={{ flex: 1 }}>
            <Text style={styles.communityTitle}>OBS Topluluğu</Text>
            <Text style={styles.communitySub}>Tüm OBS sahiplerinin ortak sohbeti</Text>
          </View>
          {joining ? (
            <ActivityIndicator size="small" color={colors.accent} />
          ) : (
            <Ionicons name="chevron-forward" size={18} color={colors.dim} />
          )}
        </TouchableOpacity>
        {joinError ? <Text style={styles.errorText}>{joinError}</Text> : null}

        <Text style={styles.sectionLabel}>Ağ ve Yönetişim</Text>
        {HUB_ITEMS.map((item) => (
          <TouchableOpacity
            key={item.route}
            style={styles.row}
            onPress={() => router.push(item.route as any)}
          >
            <View style={styles.rowIcon}>
              <Ionicons name={item.icon} size={20} color={colors.accent} />
            </View>
            <View style={{ flex: 1 }}>
              <Text style={styles.rowLabel}>{item.label}</Text>
              <Text style={styles.rowSub}>{item.sub}</Text>
            </View>
            <Ionicons name="chevron-forward" size={18} color={colors.dim} />
          </TouchableOpacity>
        ))}
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: colors.void },
  scroll: { padding: spacing.md, paddingBottom: spacing.xxl },
  title: { fontSize: typography.xl, fontWeight: "700", color: colors.head, marginBottom: spacing.md },
  communityCard: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm,
    backgroundColor: colors.surface, borderRadius: radius.lg, padding: spacing.md,
    borderWidth: 1, borderColor: "rgba(74,222,128,0.18)",
    ...shadow.sm,
  },
  communityTitle: { fontSize: typography.base, fontWeight: "700", color: colors.head },
  communitySub: { fontSize: typography.xs, color: colors.sub, marginTop: 2 },
  errorText: { fontSize: typography.xs, color: colors.red, marginTop: spacing.xs, textAlign: "center" },
  sectionLabel: {
    fontSize: typography.xs, fontWeight: "700", color: colors.sub, textTransform: "uppercase",
    letterSpacing: 0.5, marginTop: spacing.lg, marginBottom: spacing.sm,
  },
  row: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm,
    backgroundColor: colors.surface, borderRadius: radius.lg, padding: spacing.md, marginBottom: spacing.sm,
  },
  rowIcon: {
    width: 36, height: 36, borderRadius: radius.md, backgroundColor: "rgba(74,222,128,0.1)",
    alignItems: "center", justifyContent: "center",
  },
  rowLabel: { fontSize: typography.base, fontWeight: "600", color: colors.head },
  rowSub: { fontSize: typography.xs, color: colors.sub, marginTop: 2 },
});
