import React, { useEffect, useCallback, useState } from "react";
import {
  View, Text, FlatList, TouchableOpacity, StyleSheet,
  TextInput, RefreshControl, StatusBar,
} from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { useStore } from "@/lib/store";
import { api } from "@/lib/api";
import { Avatar } from "@/components/ui/Avatar";
import { EncryptionBadge } from "@/components/ui/EncryptionBadge";
import { formatTime, truncate } from "@/lib/format";
import type { Conversation } from "@/lib/store";

export default function ChatsScreen() {
  const insets = useSafeAreaInsets();
  const { conversations, setConversations } = useStore();
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [search, setSearch] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);

  const load = useCallback(async (silent = false) => {
    if (!silent) setLoading(true);
    try {
      const data = await api.getConversations();
      setConversations(data || []);
    } catch {} finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [setConversations]);

  useEffect(() => { load(); }, [load]);

  const filtered = conversations.filter((c) => {
    if (!search) return true;
    return (c.name + (c.peer_name || "")).toLowerCase().includes(search.toLowerCase());
  });

  const totalUnread = conversations.reduce((s, c) => s + c.unread_count, 0);

  const renderItem = ({ item: conv }: { item: Conversation }) => {
    const name = conv.name || conv.peer_name || "Bilinmiyor";
    const hasUnread = conv.unread_count > 0;
    return (
      <TouchableOpacity
        style={styles.convCard}
        onPress={() => router.push(`/(main)/chat/${conv.id}`)}
        activeOpacity={0.7}
      >
        <Avatar name={name} size="md" tier={conv.peer_tier} />
        <View style={styles.convContent}>
          <View style={styles.convRow}>
            <Text style={[styles.convName, hasUnread && styles.convNameUnread]} numberOfLines={1}>
              {name}
            </Text>
            <Text style={styles.convTime}>{formatTime(conv.last_msg_at)}</Text>
          </View>
          <View style={styles.convRow}>
            <Text style={[styles.convLastMsg, hasUnread && styles.convLastMsgUnread]} numberOfLines={1}>
              {conv.last_msg_text
                ? truncate(conv.last_msg_text.replace("__init__", ""), 50) || "Sohbet başladı"
                : "Mesaj yok"}
            </Text>
            {hasUnread && (
              <View style={styles.badge}>
                <Text style={styles.badgeText}>
                  {conv.unread_count > 99 ? "99+" : conv.unread_count}
                </Text>
              </View>
            )}
          </View>
        </View>
      </TouchableOpacity>
    );
  };

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />

      {/* Header */}
      <View style={styles.header}>
        <View style={styles.headerLeft}>
          <Text style={styles.headerTitle}>obscura</Text>
          {totalUnread > 0 && (
            <View style={styles.unreadBadge}>
              <Text style={styles.unreadBadgeText}>{totalUnread}</Text>
            </View>
          )}
        </View>
        <View style={styles.headerRight}>
          <EncryptionBadge showLabel />
          <TouchableOpacity onPress={() => setSearchOpen((v) => !v)} style={styles.iconBtn}>
            <Ionicons name={searchOpen ? "close" : "search"} size={20} color={colors.sub} />
          </TouchableOpacity>
          <TouchableOpacity
            onPress={() => router.push("/(main)/new-chat")}
            style={styles.iconBtn}
          >
            <Ionicons name="create-outline" size={20} color={colors.sub} />
          </TouchableOpacity>
        </View>
      </View>

      {/* Search */}
      {searchOpen && (
        <View style={styles.searchBar}>
          <Ionicons name="search" size={16} color={colors.dim} />
          <TextInput
            style={styles.searchInput}
            value={search}
            onChangeText={setSearch}
            placeholder="Sohbet ara..."
            placeholderTextColor={colors.dim}
            autoFocus
          />
        </View>
      )}

      {/* List */}
      <FlatList
        data={filtered}
        keyExtractor={(c) => c.id}
        renderItem={renderItem}
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={() => { setRefreshing(true); load(true); }}
            tintColor={colors.accent}
          />
        }
        ListEmptyComponent={
          !loading ? (
            <View style={styles.empty}>
              <Ionicons name="chatbubbles-outline" size={48} color={colors.dim} />
              <Text style={styles.emptyTitle}>
                {search ? "Sonuç bulunamadı" : "Henüz sohbet yok"}
              </Text>
              <Text style={styles.emptyText}>
                {search ? "Farklı bir arama deneyin" : "Sağ üstten yeni sohbet başlatın"}
              </Text>
            </View>
          ) : null
        }
        contentContainerStyle={filtered.length === 0 ? { flex: 1 } : { paddingBottom: insets.bottom + 80 }}
      />

      {/* FAB */}
      <TouchableOpacity
        style={[styles.fab, { bottom: insets.bottom + 24 }]}
        onPress={() => router.push("/(main)/new-chat")}
        activeOpacity={0.85}
      >
        <Ionicons name="create" size={24} color={colors.void} />
      </TouchableOpacity>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },
  header: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    paddingHorizontal: spacing.md, paddingVertical: spacing.sm,
    borderBottomWidth: 1, borderBottomColor: colors.border,
  },
  headerLeft: { flexDirection: "row", alignItems: "center", gap: 8 },
  headerTitle: { fontSize: typography.xl, fontWeight: "700", color: colors.head, letterSpacing: -0.5 },
  unreadBadge: {
    minWidth: 20, height: 20, borderRadius: 10,
    backgroundColor: colors.accent, paddingHorizontal: 5,
    alignItems: "center", justifyContent: "center",
  },
  unreadBadgeText: { fontSize: 11, fontWeight: "700", color: colors.void },
  headerRight: { flexDirection: "row", alignItems: "center", gap: 4 },
  iconBtn: { width: 38, height: 38, alignItems: "center", justifyContent: "center", borderRadius: radius.lg },
  searchBar: {
    flexDirection: "row", alignItems: "center", gap: 10,
    marginHorizontal: spacing.md, marginBottom: spacing.sm,
    backgroundColor: colors.raised, borderRadius: radius.xl,
    paddingHorizontal: spacing.md, height: 44,
    borderWidth: 1, borderColor: colors.border,
  },
  searchInput: { flex: 1, color: colors.body, fontSize: typography.sm },
  convCard: {
    flexDirection: "row", alignItems: "center", gap: 12,
    paddingHorizontal: spacing.md, paddingVertical: 14,
    borderBottomWidth: 1, borderBottomColor: colors.border + "40",
  },
  convContent: { flex: 1, minWidth: 0 },
  convRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", gap: 8 },
  convName: { fontSize: typography.sm, color: colors.body, fontWeight: "500", flex: 1 },
  convNameUnread: { color: colors.head, fontWeight: "600" },
  convTime: { fontSize: 11, color: colors.dim, flexShrink: 0 },
  convLastMsg: { fontSize: 12, color: colors.dim, flex: 1, marginTop: 2 },
  convLastMsgUnread: { color: colors.sub },
  badge: {
    minWidth: 20, height: 20, borderRadius: 10, backgroundColor: colors.accent,
    paddingHorizontal: 5, alignItems: "center", justifyContent: "center",
  },
  badgeText: { fontSize: 11, fontWeight: "700", color: colors.void },
  empty: { flex: 1, alignItems: "center", justifyContent: "center", gap: 8, paddingBottom: 100 },
  emptyTitle: { fontSize: typography.base, color: colors.body, fontWeight: "500" },
  emptyText: { fontSize: typography.sm, color: colors.dim, textAlign: "center", maxWidth: 240 },
  fab: {
    position: "absolute", right: spacing.lg,
    width: 56, height: 56, borderRadius: 28,
    backgroundColor: colors.accent,
    alignItems: "center", justifyContent: "center",
    shadowColor: colors.accent, shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.4, shadowRadius: 12, elevation: 8,
  },
});
