import React from "react";
import { View, Text, StyleSheet, StatusBar } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, typography } from "@/lib/theme";

export default function CallsScreen() {
  const insets = useSafeAreaInsets();
  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />
      <View style={styles.header}>
        <Text style={styles.headerTitle}>Aramalar</Text>
        <Text style={styles.headerSub}>Uçtan uca şifreli ses ve görüntü</Text>
      </View>
      <View style={styles.empty}>
        <View style={styles.emptyIcon}>
          <Ionicons name="call-outline" size={36} color={colors.dim} />
        </View>
        <Text style={styles.emptyTitle}>Arama geçmişi yok</Text>
        <Text style={styles.emptyText}>Sohbet ekranından birini arayabilirsiniz</Text>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },
  header: { paddingHorizontal: spacing.md, paddingVertical: spacing.sm },
  headerTitle: { fontSize: typography.xxl, fontWeight: "700", color: colors.head },
  headerSub: { fontSize: typography.sm, color: colors.sub, marginTop: 2 },
  empty: { flex: 1, alignItems: "center", justifyContent: "center", gap: 10, paddingBottom: 80 },
  emptyIcon: {
    width: 80, height: 80, borderRadius: 40,
    backgroundColor: colors.raised, borderWidth: 1, borderColor: colors.border,
    alignItems: "center", justifyContent: "center", marginBottom: 4,
  },
  emptyTitle: { fontSize: typography.base, color: colors.body, fontWeight: "500" },
  emptyText: { fontSize: typography.sm, color: colors.dim, textAlign: "center", maxWidth: 220 },
});
