import React, { useEffect, useState } from "react";
import {
  View, Text, TouchableOpacity, StyleSheet, ScrollView,
  ActivityIndicator, Alert, StatusBar,
} from "react-native";
import { useLocalSearchParams, router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { Avatar } from "@/components/ui/Avatar";
import { TierBadge } from "@/components/ui/TierBadge";
import { api } from "@/lib/api";
import { useStore } from "@/lib/store";
import { displayIdentifier } from "@/lib/odi-display";

interface UserProfile {
  did: string;
  odi?: string;
  username?: string;
  display_name?: string;
  avatar_url?: string;
  tier?: number;
  credit_score?: number;
  is_active?: number;
}

export default function UserProfileScreen() {
  const { did, name, convId } = useLocalSearchParams<{ did: string; name: string; convId?: string }>();
  const insets = useSafeAreaInsets();
  const currentUser = useStore((s) => s.user);

  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!did) { setError("Kullanıcı bulunamadı"); setLoading(false); return; }
    api.getUser(did)
      .then((data: UserProfile) => setProfile(data))
      .catch(() => setError("Profil yüklenemedi"))
      .finally(() => setLoading(false));
  }, [did]);

  const displayName = profile?.display_name || profile?.username || name || did?.slice(8, 20) || "?";
  const tier = profile?.tier ?? 0;
  const creditScore = profile?.credit_score ?? 0;

  const handleMessage = async () => {
    if (!did) return;
    try {
      const res = await api.createConversation({ peer_did: did });
      const cId = convId || res?.conv_id;
      if (cId) {
        router.replace(`/(main)/chat/${cId}` as any);
      }
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Sohbet açılamadı");
    }
  };

  const handleVoiceCall = () => {
    if (!did) return;
    router.push(`/(main)/voice-call?peer=${did}&peerName=${encodeURIComponent(displayName)}&convId=${convId || ""}` as any);
  };

  const handleVideoCall = () => {
    if (!did) return;
    router.push(`/(main)/video-call?peer=${did}&peerName=${encodeURIComponent(displayName)}&convId=${convId || ""}` as any);
  };

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />

      {/* Header */}
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Profil</Text>
        <View style={styles.backBtn} />
      </View>

      {loading ? (
        <View style={styles.center}>
          <ActivityIndicator color={colors.accent} />
        </View>
      ) : error ? (
        <View style={styles.center}>
          <Ionicons name="alert-circle-outline" size={40} color={colors.red} />
          <Text style={styles.errorText}>{error}</Text>
        </View>
      ) : (
        <ScrollView contentContainerStyle={styles.content}>
          {/* Avatar + name */}
          <View style={styles.heroSection}>
            <View style={styles.avatarWrap}>
              <Avatar name={displayName} size="xl" />
              {(profile?.is_active === 1) && <View style={styles.onlineDot} />}
            </View>
            <Text style={styles.displayName}>{displayName}</Text>
            {profile?.username && profile.username !== profile.display_name && (
              <Text style={styles.username}>@{profile.username}</Text>
            )}
            <TierBadge tier={tier} />
          </View>

          {/* Stats */}
          <View style={styles.statsRow}>
            <View style={styles.statItem}>
              <Text style={styles.statValue}>{creditScore.toFixed(0)}</Text>
              <Text style={styles.statLabel}>Kredi Skoru</Text>
            </View>
            <View style={styles.statDivider} />
            <View style={styles.statItem}>
              <Text style={styles.statValue}>Seviye {tier}</Text>
              <Text style={styles.statLabel}>Güven</Text>
            </View>
            <View style={styles.statDivider} />
            <View style={styles.statItem}>
              <Ionicons name="lock-closed" size={14} color={colors.accent} />
              <Text style={styles.statLabel}>E2E Şifreli</Text>
            </View>
          </View>

          {/* ODI */}
          <View style={styles.didCard}>
            <Text style={styles.didLabel}>Görünen Kimlik Kodu</Text>
            <Text style={styles.didValue} numberOfLines={2} selectable>
              {displayIdentifier({ odi: profile?.odi, did })}
            </Text>
          </View>

          {/* Actions — kendi profilinde gösterme */}
          {did !== currentUser?.did && (
            <View style={styles.actions}>
              <TouchableOpacity style={styles.actionBtn} onPress={handleMessage} activeOpacity={0.8}>
                <Ionicons name="chatbubble-outline" size={22} color={colors.void} />
                <Text style={styles.actionLabel}>Mesaj</Text>
              </TouchableOpacity>

              <TouchableOpacity style={[styles.actionBtn, styles.actionBtnSecondary]} onPress={handleVoiceCall} activeOpacity={0.8}>
                <Ionicons name="call-outline" size={22} color={colors.head} />
                <Text style={[styles.actionLabel, { color: colors.head }]}>Sesli Ara</Text>
              </TouchableOpacity>

              <TouchableOpacity style={[styles.actionBtn, styles.actionBtnSecondary]} onPress={handleVideoCall} activeOpacity={0.8}>
                <Ionicons name="videocam-outline" size={22} color={colors.head} />
                <Text style={[styles.actionLabel, { color: colors.head }]}>Video Ara</Text>
              </TouchableOpacity>
            </View>
          )}
        </ScrollView>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },
  header: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    paddingHorizontal: spacing.lg, paddingVertical: spacing.md,
    borderBottomWidth: 1, borderBottomColor: colors.border,
  },
  backBtn: { width: 40, height: 40, alignItems: "center", justifyContent: "center" },
  headerTitle: { fontSize: typography.md, fontWeight: "700", color: colors.head },
  center: { flex: 1, alignItems: "center", justifyContent: "center", gap: 12 },
  errorText: { fontSize: typography.sm, color: colors.red },
  content: { padding: spacing.xl, gap: spacing.lg, alignItems: "center" },
  heroSection: { alignItems: "center", gap: spacing.sm, paddingTop: spacing.lg },
  avatarWrap: { position: "relative" },
  onlineDot: {
    position: "absolute", bottom: 2, right: 2,
    width: 14, height: 14, borderRadius: 7,
    backgroundColor: colors.accent, borderWidth: 2, borderColor: colors.void,
  },
  displayName: { fontSize: typography.xxl, fontWeight: "700", color: colors.head, textAlign: "center" },
  username: { fontSize: typography.sm, color: colors.sub },
  statsRow: {
    flexDirection: "row", width: "100%",
    backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border, padding: spacing.md,
  },
  statItem: { flex: 1, alignItems: "center", gap: 4 },
  statDivider: { width: 1, backgroundColor: colors.border, alignSelf: "stretch" },
  statValue: { fontSize: typography.base, fontWeight: "700", color: colors.head },
  statLabel: { fontSize: 10, color: colors.sub },
  didCard: {
    width: "100%", backgroundColor: colors.surface,
    borderRadius: radius.xl, borderWidth: 1, borderColor: colors.border,
    padding: spacing.md, gap: 6,
  },
  didLabel: { fontSize: 10, color: colors.dim, textTransform: "uppercase", letterSpacing: 1 },
  didValue: { fontSize: 11, color: colors.sub, fontFamily: "monospace", lineHeight: 16 },
  actions: { flexDirection: "row", gap: spacing.sm, width: "100%" },
  actionBtn: {
    flex: 1, flexDirection: "column", alignItems: "center", justifyContent: "center",
    gap: 6, paddingVertical: 14, borderRadius: radius.xl,
    backgroundColor: colors.accent,
  },
  actionBtnSecondary: {
    backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.border,
  },
  actionLabel: { fontSize: 11, fontWeight: "600", color: colors.void },
});
