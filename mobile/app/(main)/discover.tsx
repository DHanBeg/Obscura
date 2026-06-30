import React, { useState, useEffect, useCallback, useRef } from "react";
import {
  View,
  Text,
  FlatList,
  TextInput,
  TouchableOpacity,
  StyleSheet,
  ActivityIndicator,
  Alert,
  StatusBar,
} from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";

interface DiscoverItem {
  id: string;
  name: string;
  conv_type: string;
  member_count: number;
}

const TYPE_LABELS: Record<string, string> = {
  group: "Grup",
  channel: "Kanal",
  community: "Topluluk",
  direct: "Sohbet",
};

const TYPE_ICONS: Record<string, keyof typeof Ionicons.glyphMap> = {
  group: "people-outline",
  channel: "megaphone-outline",
  community: "earth-outline",
  direct: "chatbubble-outline",
};

export default function DiscoverScreen() {
  const insets = useSafeAreaInsets();
  const [query, setQuery] = useState("");
  const [items, setItems] = useState<DiscoverItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [joiningId, setJoiningId] = useState<string | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const loadItems = useCallback(async (q: string) => {
    setLoading(true);
    try {
      const data = await api.discoverConversations(q);
      setItems(data ?? []);
    } catch {
      setItems([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadItems("");
  }, [loadItems]);

  const handleQueryChange = (text: string) => {
    setQuery(text);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      loadItems(text.trim());
    }, 400);
  };

  const handleJoin = async (item: DiscoverItem) => {
    setJoiningId(item.id);
    try {
      await api.joinConversation(item.id);
      Alert.alert("Katıldınız", `"${item.name}" grubuna katıldınız.`, [
        {
          text: "Tamam",
          onPress: () => router.back(),
        },
      ]);
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Katılım başarısız");
    } finally {
      setJoiningId(null);
    }
  };

  const renderItem = ({ item }: { item: DiscoverItem }) => {
    const typeLabel = TYPE_LABELS[item.conv_type] ?? item.conv_type;
    const iconName = TYPE_ICONS[item.conv_type] ?? "chatbubble-outline";
    const isJoining = joiningId === item.id;

    return (
      <View style={styles.item}>
        <View style={styles.itemIcon}>
          <Ionicons name={iconName} size={22} color={colors.accent} />
        </View>
        <View style={styles.itemContent}>
          <Text style={styles.itemName} numberOfLines={1}>
            {item.name}
          </Text>
          <Text style={styles.itemMeta}>
            {typeLabel} · {item.member_count} üye
          </Text>
        </View>
        {isJoining ? (
          <ActivityIndicator size="small" color={colors.accent} style={styles.joinBtn} />
        ) : (
          <TouchableOpacity
            style={styles.joinBtn}
            onPress={() => handleJoin(item)}
            activeOpacity={0.7}
          >
            <Text style={styles.joinBtnText}>Katıl</Text>
          </TouchableOpacity>
        )}
      </View>
    );
  };

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />

      {/* Header */}
      <View style={styles.header}>
        <TouchableOpacity
          onPress={() => router.back()}
          style={styles.backBtn}
          activeOpacity={0.7}
        >
          <Ionicons name="chevron-back" size={22} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Keşfet</Text>
        <View style={styles.backBtn} />
      </View>

      {/* Search */}
      <View style={styles.searchBar}>
        <Ionicons name="search-outline" size={16} color={colors.dim} />
        <TextInput
          style={styles.searchInput}
          value={query}
          onChangeText={handleQueryChange}
          placeholder="Grup veya kanal ara..."
          placeholderTextColor={colors.dim}
          returnKeyType="search"
        />
        {query.length > 0 && (
          <TouchableOpacity onPress={() => handleQueryChange("")} activeOpacity={0.7}>
            <Ionicons name="close-circle" size={16} color={colors.dim} />
          </TouchableOpacity>
        )}
      </View>

      {/* Content */}
      {loading ? (
        <View style={styles.center}>
          <ActivityIndicator color={colors.accent} />
        </View>
      ) : items.length === 0 ? (
        <View style={styles.center}>
          <Ionicons name="planet-outline" size={48} color={colors.muted} />
          <Text style={styles.emptyTitle}>
            {query ? "Sonuç bulunamadı" : "Henüz public grup yok"}
          </Text>
          <Text style={styles.emptySubtitle}>
            {query
              ? "Farklı bir arama terimi deneyin"
              : "Bir grup oluşturun ve herkese açık yapın"}
          </Text>
        </View>
      ) : (
        <FlatList
          data={items}
          keyExtractor={(item) => item.id}
          renderItem={renderItem}
          contentContainerStyle={{ paddingBottom: insets.bottom + 24 }}
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },
  header: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
  },
  backBtn: { width: 36, height: 36, alignItems: "center", justifyContent: "center" },
  headerTitle: { fontSize: typography.md, fontWeight: "700", color: colors.head },
  searchBar: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.sm,
    backgroundColor: colors.surface,
    borderRadius: radius.xl,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: spacing.md,
    paddingVertical: 10,
    margin: spacing.md,
  },
  searchInput: { flex: 1, fontSize: typography.sm, color: colors.head },
  center: { flex: 1, alignItems: "center", justifyContent: "center", gap: 12 },
  emptyTitle: { fontSize: typography.base, fontWeight: "600", color: colors.sub },
  emptySubtitle: {
    fontSize: typography.sm,
    color: colors.dim,
    textAlign: "center",
    paddingHorizontal: spacing.xl,
  },
  item: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.sm,
    paddingHorizontal: spacing.md,
    paddingVertical: 12,
    borderBottomWidth: 1,
    borderBottomColor: "rgba(255,255,255,0.04)",
  },
  itemIcon: {
    width: 42,
    height: 42,
    borderRadius: radius.lg,
    backgroundColor: colors.accentDeep,
    alignItems: "center",
    justifyContent: "center",
  },
  itemContent: { flex: 1 },
  itemName: { fontSize: typography.base, fontWeight: "600", color: colors.head },
  itemMeta: { fontSize: typography.xs, color: colors.sub, marginTop: 2 },
  joinBtn: {
    paddingHorizontal: 16,
    paddingVertical: 7,
    borderRadius: radius.full,
    backgroundColor: colors.accent,
    minWidth: 58,
    alignItems: "center",
    justifyContent: "center",
  },
  joinBtnText: { fontSize: 13, fontWeight: "700", color: colors.void },
});
