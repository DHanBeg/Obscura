import React from "react";
import {
  View, Text, StyleSheet, StatusBar, TouchableOpacity, ScrollView,
} from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { EncryptionBadge } from "@/components/ui/EncryptionBadge";
import { useStore } from "@/lib/store";
import { Avatar } from "@/components/ui/Avatar";

export default function CallsScreen() {
  const insets = useSafeAreaInsets();
  const conversations = useStore((s) => s.conversations);
  const user = useStore((s) => s.user);

  const peers = conversations
    .filter((c) => c.peer_did && c.peer_did !== user?.did)
    .slice(0, 6);

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />

      <View style={styles.header}>
        <Text style={styles.headerTitle}>Aramalar</Text>
        <EncryptionBadge showLabel />
      </View>

      <ScrollView
        contentContainerStyle={{ paddingBottom: insets.bottom + 100 }}
        showsVerticalScrollIndicator={false}
      >
        {/* E2E badge */}
        <View style={styles.infoBanner}>
          <Ionicons name="lock-closed" size={13} color={colors.accent} />
          <Text style={styles.infoText}>
            Tüm aramalar uçtan uca şifreli ve doğrudan iletilir. Kayıt tutulmaz.
          </Text>
        </View>

        {/* Call type cards */}
        <View style={styles.callCards}>
          <TouchableOpacity
            style={[styles.callCard, styles.callCardVoice]}
            onPress={() => router.push("/(main)/chats")}
            activeOpacity={0.85}
          >
            <View style={[styles.callCardIcon, { backgroundColor: "rgba(74,222,128,0.12)" }]}>
              <Ionicons name="call" size={24} color={colors.accent} />
            </View>
            <Text style={styles.callCardTitle}>Sesli Arama</Text>
            <Text style={styles.callCardSub}>Sohbetten ara</Text>
          </TouchableOpacity>

          <TouchableOpacity
            style={[styles.callCard, styles.callCardVideo]}
            onPress={() => router.push("/(main)/chats")}
            activeOpacity={0.85}
          >
            <View style={[styles.callCardIcon, { backgroundColor: "rgba(99,102,241,0.12)" }]}>
              <Ionicons name="videocam" size={24} color="#818cf8" />
            </View>
            <Text style={styles.callCardTitle}>Görüntülü Arama</Text>
            <Text style={styles.callCardSub}>Sohbetten ara</Text>
          </TouchableOpacity>
        </View>

        {/* Quick call from recent chats */}
        {peers.length > 0 && (
          <>
            <Text style={styles.sectionHeader}>HIZLI ARA</Text>
            <View style={styles.quickList}>
              {peers.map((conv) => {
                const name = conv.name || conv.peer_name || "Kullanıcı";
                return (
                  <View key={conv.id} style={styles.quickItem}>
                    <Avatar name={name} size="md" />
                    <View style={styles.quickInfo}>
                      <Text style={styles.quickName} numberOfLines={1}>{name}</Text>
                      <Text style={styles.quickSub}>Kişisel sohbet</Text>
                    </View>
                    <TouchableOpacity
                      style={styles.quickCallBtn}
                      onPress={() =>
                        router.push(`/(main)/voice-call?peer=${conv.peer_did}&peerName=${encodeURIComponent(name)}&convId=${conv.id}` as any)
                      }
                      activeOpacity={0.8}
                    >
                      <Ionicons name="call-outline" size={18} color={colors.accent} />
                    </TouchableOpacity>
                    <TouchableOpacity
                      style={[styles.quickCallBtn, { backgroundColor: "rgba(99,102,241,0.1)", borderColor: "rgba(99,102,241,0.2)" }]}
                      onPress={() =>
                        router.push(`/(main)/video-call?peer=${conv.peer_did}&peerName=${encodeURIComponent(name)}&convId=${conv.id}` as any)
                      }
                      activeOpacity={0.8}
                    >
                      <Ionicons name="videocam-outline" size={18} color="#818cf8" />
                    </TouchableOpacity>
                  </View>
                );
              })}
            </View>
          </>
        )}

        {/* Recent calls header */}
        <Text style={styles.sectionHeader}>SON ARAMALAR</Text>

        {/* Empty state */}
        <View style={styles.empty}>
          <View style={styles.emptyIcon}>
            <Ionicons name="time-outline" size={26} color={colors.dim} />
          </View>
          <Text style={styles.emptyTitle}>Arama geçmişi yok</Text>
          <Text style={styles.emptyText}>
            Aramalar gizlilik koruması nedeniyle kaydedilmez
          </Text>
        </View>
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },
  header: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    paddingHorizontal: spacing.md, paddingVertical: spacing.sm,
  },
  headerTitle: {
    fontSize: typography.xl, fontWeight: "700",
    color: colors.head, letterSpacing: -0.5,
  },
  infoBanner: {
    flexDirection: "row", alignItems: "center", gap: 10,
    marginHorizontal: spacing.md, marginBottom: spacing.md,
    padding: spacing.md,
    backgroundColor: "rgba(74,222,128,0.06)",
    borderRadius: radius.lg, borderWidth: 1, borderColor: "rgba(74,222,128,0.18)",
  },
  infoText: { flex: 1, fontSize: typography.xs, color: colors.sub, lineHeight: 18 },
  callCards: {
    flexDirection: "row", gap: spacing.sm,
    marginHorizontal: spacing.md, marginBottom: spacing.lg,
  },
  callCard: {
    flex: 1, alignItems: "center", gap: 8,
    padding: spacing.md,
    borderRadius: radius.xl, borderWidth: 1,
  },
  callCardVoice: {
    backgroundColor: colors.surface,
    borderColor: "rgba(74,222,128,0.15)",
  },
  callCardVideo: {
    backgroundColor: colors.surface,
    borderColor: "rgba(99,102,241,0.15)",
  },
  callCardIcon: {
    width: 52, height: 52, borderRadius: 16,
    alignItems: "center", justifyContent: "center",
  },
  callCardTitle: {
    fontSize: typography.sm, fontWeight: "600", color: colors.head, textAlign: "center",
  },
  callCardSub: { fontSize: typography.xs, color: colors.dim, textAlign: "center" },
  sectionHeader: {
    fontSize: typography.xs, fontWeight: "600",
    color: colors.dim, letterSpacing: 0.8,
    marginHorizontal: spacing.md, marginBottom: spacing.sm,
  },
  quickList: {
    marginHorizontal: spacing.md, marginBottom: spacing.lg,
    backgroundColor: colors.surface,
    borderRadius: radius.xl, borderWidth: 1, borderColor: colors.border,
    overflow: "hidden",
  },
  quickItem: {
    flexDirection: "row", alignItems: "center", gap: 12,
    paddingHorizontal: spacing.md, paddingVertical: 12,
    borderBottomWidth: 1, borderBottomColor: colors.border + "60",
  },
  quickInfo: { flex: 1 },
  quickName: { fontSize: typography.sm, fontWeight: "500", color: colors.head },
  quickSub: { fontSize: typography.xs, color: colors.dim, marginTop: 1 },
  quickCallBtn: {
    width: 38, height: 38, borderRadius: 12,
    backgroundColor: "rgba(74,222,128,0.08)",
    borderWidth: 1, borderColor: "rgba(74,222,128,0.15)",
    alignItems: "center", justifyContent: "center",
  },
  empty: {
    alignItems: "center", gap: 8,
    paddingTop: 32, paddingHorizontal: spacing.lg,
  },
  emptyIcon: {
    width: 56, height: 56, borderRadius: 28,
    backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.border,
    alignItems: "center", justifyContent: "center", marginBottom: 4,
  },
  emptyTitle: { fontSize: typography.sm, fontWeight: "500", color: colors.sub },
  emptyText: {
    fontSize: typography.xs, color: colors.dim,
    textAlign: "center", maxWidth: 240, lineHeight: 17,
  },
});
