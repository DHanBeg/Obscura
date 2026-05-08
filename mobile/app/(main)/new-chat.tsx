import React, { useState, useCallback } from "react";
import {
  View, Text, TextInput, FlatList, TouchableOpacity,
  StyleSheet, ActivityIndicator, StatusBar,
} from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";
import { Avatar } from "@/components/ui/Avatar";

export default function NewChatScreen() {
  const insets = useSafeAreaInsets();
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);

  const search = useCallback(async (q: string) => {
    setQuery(q);
    if (q.length < 2) { setResults([]); return; }
    setLoading(true);
    try {
      const data = await api.searchUsers(q);
      setResults(data || []);
    } catch { setResults([]); }
    finally { setLoading(false); }
  }, []);

  const startChat = useCallback(async (user: any) => {
    try {
      await api.sendMessage({ to_id: user.did, ciphertext: "__init__", type: "system" });
      const convs = await api.getConversations();
      const conv = convs?.find((c: any) => c.peer_did === user.did);
      if (conv) router.replace(`/(main)/chat/${conv.id}`);
      else router.back();
    } catch { router.back(); }
  }, []);

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.closeBtn}>
          <Ionicons name="close" size={22} color={colors.body} />
        </TouchableOpacity>
        <Text style={styles.title}>Yeni sohbet</Text>
      </View>
      <View style={styles.searchBar}>
        <Ionicons name="search" size={16} color={colors.dim} />
        <TextInput
          style={styles.searchInput}
          value={query}
          onChangeText={search}
          placeholder="Kullanıcı ara..."
          placeholderTextColor={colors.dim}
          autoFocus
          autoCapitalize="none"
        />
        {loading && <ActivityIndicator size="small" color={colors.accent} />}
      </View>
      <FlatList
        data={results}
        keyExtractor={(u) => u.did}
        renderItem={({ item: user }) => (
          <TouchableOpacity style={styles.userRow} onPress={() => startChat(user)} activeOpacity={0.7}>
            <Avatar name={user.display_name || user.username} size="md" tier={user.tier} />
            <View style={styles.userInfo}>
              <Text style={styles.userName}>{user.display_name || user.username}</Text>
              <Text style={styles.userHandle}>@{user.username}</Text>
            </View>
            <Text style={styles.startBtn}>Başlat</Text>
          </TouchableOpacity>
        )}
        ListEmptyComponent={
          query.length >= 2 && !loading ? (
            <View style={styles.empty}>
              <Ionicons name="person-add-outline" size={36} color={colors.dim} />
              <Text style={styles.emptyText}>Kullanıcı bulunamadı</Text>
            </View>
          ) : query.length < 2 ? (
            <View style={styles.empty}>
              <Ionicons name="search-outline" size={36} color={colors.dim} />
              <Text style={styles.emptyText}>En az 2 karakter girin</Text>
            </View>
          ) : null
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },
  header: { flexDirection: "row", alignItems: "center", gap: 12, paddingHorizontal: spacing.md, paddingVertical: spacing.sm },
  closeBtn: { width: 36, height: 36, borderRadius: radius.lg, backgroundColor: colors.raised, alignItems: "center", justifyContent: "center" },
  title: { fontSize: typography.lg, fontWeight: "600", color: colors.head },
  searchBar: {
    flexDirection: "row", alignItems: "center", gap: 10,
    marginHorizontal: spacing.md, marginBottom: spacing.sm,
    backgroundColor: colors.raised, borderRadius: radius.xl,
    paddingHorizontal: spacing.md, height: 48,
    borderWidth: 1, borderColor: colors.border,
  },
  searchInput: { flex: 1, color: colors.body, fontSize: typography.sm },
  userRow: {
    flexDirection: "row", alignItems: "center", gap: 12,
    paddingHorizontal: spacing.md, paddingVertical: 14,
    borderBottomWidth: 1, borderBottomColor: colors.border + "40",
  },
  userInfo: { flex: 1 },
  userName: { fontSize: typography.sm, color: colors.head, fontWeight: "500" },
  userHandle: { fontSize: 12, color: colors.dim, marginTop: 1 },
  startBtn: { fontSize: 12, color: colors.accent, fontWeight: "600" },
  empty: { alignItems: "center", justifyContent: "center", paddingTop: 60, gap: 10 },
  emptyText: { fontSize: typography.sm, color: colors.dim },
});
