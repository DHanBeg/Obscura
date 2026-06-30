import React, { useEffect, useState, useCallback } from "react";
import {
  View, Text, FlatList, TouchableOpacity, StyleSheet,
  TextInput, StatusBar, ActivityIndicator, Alert,
} from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { Avatar } from "@/components/ui/Avatar";
import { api } from "@/lib/api";

interface Contact {
  did: string;
  display_name: string;
  username: string;
  avatar_url: string;
  tier: number;
  nickname: string;
  added_at: string;
}

interface SearchUser {
  did: string;
  display_name?: string;
  username?: string;
  tier?: number;
}

type Tab = "contacts" | "search";

export default function ContactsScreen() {
  const insets = useSafeAreaInsets();
  const [tab, setTab] = useState<Tab>("contacts");

  // Contacts tab
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState("");
  const [refreshing, setRefreshing] = useState(false);

  // Search tab
  const [query, setQuery] = useState("");
  const [searching, setSearching] = useState(false);
  const [searchResults, setSearchResults] = useState<SearchUser[]>([]);
  const [addingDID, setAddingDID] = useState<string | null>(null);

  const loadContacts = useCallback(async (silent = false) => {
    if (!silent) setLoading(true);
    try {
      const res = await api.getContacts();
      setContacts(res?.contacts ?? []);
    } catch { /* ignore */ } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => { loadContacts(); }, [loadContacts]);

  const handleSearch = async () => {
    const q = query.trim();
    if (!q) return;
    setSearching(true);
    setSearchResults([]);
    try {
      const res = await api.searchUsers(q);
      setSearchResults(res?.users ?? []);
    } catch { setSearchResults([]); } finally {
      setSearching(false);
    }
  };

  const handleAdd = async (user: SearchUser) => {
    setAddingDID(user.did);
    try {
      await api.addContact(user.did);
      Alert.alert("Eklendi", `${user.display_name || user.username || "Kullanıcı"} rehbere eklendi.`);
      loadContacts(true);
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Eklenemedi");
    } finally {
      setAddingDID(null);
    }
  };

  const handleRemove = (contact: Contact) => {
    const name = contact.nickname || contact.display_name || contact.username || "Bu kişi";
    Alert.alert(
      "Kişiyi Sil",
      `${name} rehberden silinsin mi?`,
      [
        { text: "İptal", style: "cancel" },
        {
          text: "Sil", style: "destructive",
          onPress: async () => {
            await api.removeContact(contact.did);
            setContacts((prev) => prev.filter((c) => c.did !== contact.did));
          },
        },
      ]
    );
  };

  const handleMessage = async (did: string) => {
    try {
      const res = await api.createConversation({ peer_did: did });
      if (res?.conv_id) router.push(`/(main)/chat/${res.conv_id}` as any);
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Sohbet açılamadı");
    }
  };

  const filtered = contacts.filter((c) => {
    if (!filter) return true;
    const q = filter.toLowerCase();
    return (
      (c.nickname || "").toLowerCase().includes(q) ||
      (c.display_name || "").toLowerCase().includes(q) ||
      (c.username || "").toLowerCase().includes(q)
    );
  });

  const isInContacts = (did: string) => contacts.some((c) => c.did === did);

  const renderContact = ({ item }: { item: Contact }) => {
    const name = item.nickname || item.display_name || item.username || item.did.slice(8, 20);
    const sub = item.nickname && item.display_name ? item.display_name : item.username ? `@${item.username}` : "";
    return (
      <TouchableOpacity
        style={styles.item}
        activeOpacity={0.7}
        onPress={() => router.push(`/(main)/user-profile?did=${item.did}&name=${encodeURIComponent(name)}` as any)}
      >
        <Avatar name={name} size="md" tier={item.tier} />
        <View style={styles.itemContent}>
          <Text style={styles.itemName} numberOfLines={1}>{name}</Text>
          {sub ? <Text style={styles.itemSub} numberOfLines={1}>{sub}</Text> : null}
        </View>
        <TouchableOpacity style={styles.msgBtn} onPress={() => handleMessage(item.did)} activeOpacity={0.7}>
          <Ionicons name="chatbubble-outline" size={18} color={colors.accent} />
        </TouchableOpacity>
        <TouchableOpacity style={styles.removeBtn} onPress={() => handleRemove(item)} activeOpacity={0.7}>
          <Ionicons name="person-remove-outline" size={18} color={colors.dim} />
        </TouchableOpacity>
      </TouchableOpacity>
    );
  };

  const renderSearchResult = ({ item }: { item: SearchUser }) => {
    const name = item.display_name || item.username || item.did.slice(8, 20);
    const inList = isInContacts(item.did);
    const adding = addingDID === item.did;
    return (
      <TouchableOpacity
        style={styles.item}
        activeOpacity={0.7}
        onPress={() => router.push(`/(main)/user-profile?did=${item.did}&name=${encodeURIComponent(name)}` as any)}
      >
        <Avatar name={name} size="md" tier={item.tier} />
        <View style={styles.itemContent}>
          <Text style={styles.itemName} numberOfLines={1}>{name}</Text>
          {item.username ? <Text style={styles.itemSub}>@{item.username}</Text> : null}
        </View>
        {adding ? (
          <ActivityIndicator size="small" color={colors.accent} />
        ) : inList ? (
          <View style={styles.inListBadge}>
            <Ionicons name="checkmark" size={14} color={colors.accent} />
            <Text style={styles.inListText}>Rehberde</Text>
          </View>
        ) : (
          <TouchableOpacity style={styles.addBtn} onPress={() => handleAdd(item)} activeOpacity={0.7}>
            <Ionicons name="person-add-outline" size={15} color={colors.void} />
            <Text style={styles.addBtnText}>Ekle</Text>
          </TouchableOpacity>
        )}
      </TouchableOpacity>
    );
  };

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />

      {/* Header */}
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn} activeOpacity={0.7}>
          <Ionicons name="chevron-back" size={22} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Kişiler</Text>
        <View style={styles.backBtn} />
      </View>

      {/* Tabs */}
      <View style={styles.tabBar}>
        {(["contacts", "search"] as Tab[]).map((t) => (
          <TouchableOpacity
            key={t}
            style={[styles.tabItem, tab === t && styles.tabItemActive]}
            onPress={() => setTab(t)}
            activeOpacity={0.7}
          >
            <Text style={[styles.tabText, tab === t && styles.tabTextActive]}>
              {t === "contacts" ? `Rehber${contacts.length ? ` (${contacts.length})` : ""}` : "Kullanıcı Ara"}
            </Text>
          </TouchableOpacity>
        ))}
      </View>

      {tab === "contacts" ? (
        <>
          {/* Filter input */}
          <View style={styles.searchBar}>
            <Ionicons name="search-outline" size={16} color={colors.dim} />
            <TextInput
              style={styles.searchInput}
              value={filter}
              onChangeText={setFilter}
              placeholder="Rehberde ara..."
              placeholderTextColor={colors.dim}
            />
            {filter ? (
              <TouchableOpacity onPress={() => setFilter("")}>
                <Ionicons name="close-circle" size={16} color={colors.dim} />
              </TouchableOpacity>
            ) : null}
          </View>

          {loading ? (
            <View style={styles.center}>
              <ActivityIndicator color={colors.accent} />
            </View>
          ) : filtered.length === 0 ? (
            <View style={styles.center}>
              <Ionicons name="people-outline" size={44} color={colors.muted} />
              <Text style={styles.emptyText}>
                {filter ? "Sonuç bulunamadı" : "Rehberiniz boş"}
              </Text>
              {!filter && (
                <TouchableOpacity style={styles.emptyAction} onPress={() => setTab("search")} activeOpacity={0.7}>
                  <Text style={styles.emptyActionText}>Kullanıcı ara ve ekle</Text>
                </TouchableOpacity>
              )}
            </View>
          ) : (
            <FlatList
              data={filtered}
              keyExtractor={(c) => c.did}
              renderItem={renderContact}
              contentContainerStyle={{ paddingBottom: insets.bottom + 24 }}
              onRefresh={() => { setRefreshing(true); loadContacts(); }}
              refreshing={refreshing}
            />
          )}
        </>
      ) : (
        <>
          {/* Search input */}
          <View style={styles.searchBarRow}>
            <View style={[styles.searchBar, { flex: 1 }]}>
              <Ionicons name="search-outline" size={16} color={colors.dim} />
              <TextInput
                style={styles.searchInput}
                value={query}
                onChangeText={setQuery}
                placeholder="İsim veya kullanıcı adı..."
                placeholderTextColor={colors.dim}
                returnKeyType="search"
                onSubmitEditing={handleSearch}
                autoFocus
              />
            </View>
            <TouchableOpacity style={styles.searchActionBtn} onPress={handleSearch} activeOpacity={0.7}>
              {searching
                ? <ActivityIndicator size="small" color={colors.void} />
                : <Text style={styles.searchActionText}>Ara</Text>}
            </TouchableOpacity>
          </View>

          {searchResults.length > 0 ? (
            <FlatList
              data={searchResults}
              keyExtractor={(u) => u.did}
              renderItem={renderSearchResult}
              contentContainerStyle={{ paddingBottom: insets.bottom + 24 }}
            />
          ) : !searching && query ? (
            <View style={styles.center}>
              <Ionicons name="search-outline" size={44} color={colors.muted} />
              <Text style={styles.emptyText}>Sonuç bulunamadı</Text>
            </View>
          ) : (
            <View style={styles.center}>
              <Ionicons name="person-search-outline" size={44} color={colors.muted} />
              <Text style={styles.emptyText}>Kullanıcı ara</Text>
            </View>
          )}
        </>
      )}
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
  backBtn: { width: 36, height: 36, alignItems: "center", justifyContent: "center" },
  headerTitle: { fontSize: typography.md, fontWeight: "700", color: colors.head },
  tabBar: {
    flexDirection: "row", borderBottomWidth: 1, borderBottomColor: colors.border,
  },
  tabItem: {
    flex: 1, paddingVertical: 12, alignItems: "center",
    borderBottomWidth: 2, borderBottomColor: "transparent",
  },
  tabItemActive: { borderBottomColor: colors.accent },
  tabText: { fontSize: typography.sm, color: colors.sub },
  tabTextActive: { color: colors.accent, fontWeight: "600" },
  searchBar: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm,
    backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border,
    paddingHorizontal: spacing.md, paddingVertical: 10,
    margin: spacing.md,
  },
  searchBarRow: {
    flexDirection: "row", gap: spacing.sm, margin: spacing.md, alignItems: "center",
  },
  searchInput: { flex: 1, fontSize: typography.sm, color: colors.head },
  searchActionBtn: {
    backgroundColor: colors.accent, paddingHorizontal: 16, paddingVertical: 11,
    borderRadius: radius.xl, minWidth: 48, alignItems: "center",
  },
  searchActionText: { color: colors.void, fontWeight: "700", fontSize: typography.sm },
  item: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm,
    paddingHorizontal: spacing.md, paddingVertical: 12,
    borderBottomWidth: 1, borderBottomColor: "rgba(255,255,255,0.04)",
  },
  itemContent: { flex: 1 },
  itemName: { fontSize: typography.base, fontWeight: "600", color: colors.head },
  itemSub: { fontSize: typography.xs, color: colors.sub, marginTop: 1 },
  msgBtn: { padding: 8 },
  removeBtn: { padding: 8 },
  addBtn: {
    flexDirection: "row", alignItems: "center", gap: 4,
    backgroundColor: colors.accent, paddingHorizontal: 12, paddingVertical: 6,
    borderRadius: radius.full,
  },
  addBtnText: { fontSize: 12, fontWeight: "700", color: colors.void },
  inListBadge: {
    flexDirection: "row", alignItems: "center", gap: 4,
    paddingHorizontal: 10, paddingVertical: 6,
    borderRadius: radius.full, borderWidth: 1, borderColor: "rgba(74,222,128,0.3)",
  },
  inListText: { fontSize: 12, color: colors.accent },
  center: { flex: 1, alignItems: "center", justifyContent: "center", gap: 12 },
  emptyText: { fontSize: typography.sm, color: colors.sub },
  emptyAction: {
    paddingHorizontal: 20, paddingVertical: 10, borderRadius: radius.full,
    backgroundColor: colors.accent, marginTop: 4,
  },
  emptyActionText: { fontSize: typography.sm, fontWeight: "700", color: colors.void },
});
