import React from "react";
import {
  View, Text, ScrollView, StyleSheet, StatusBar, TouchableOpacity,
} from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";

// Bu ekran eskiden 4 toggle içeriyordu ama hiçbiri hiçbir zaman gerçek bir
// şeyi tetiklemedi (AsyncStorage'a yazıp bırakıyordu). Backend'de bu
// özelliklerin bir kısmı zaten var ve aktif — kullanıcı bunları açıp
// kapatamaz, bu yüzden artık salt-okunur durum satırları olarak gösteriliyor.
const FEATURES: { label: string; sublabel: string; active: boolean }[] = [
  { label: "ZK Kimlik Doğrulama", sublabel: "Groth16 tabanlı anonim giriş", active: true },
  { label: "Post-Quantum Şifreleme", sublabel: "Dilithium3 + Kyber1024", active: true },
  { label: "Dağıtık Depolama", sublabel: "Reed-Solomon parçalama (4-of-6)", active: true },
  { label: "Federe Mesajlaşma", sublabel: "Node-to-node federasyon", active: true },
];

function StatusRow({
  label, sublabel, active, last,
}: { label: string; sublabel: string; active: boolean; last?: boolean }) {
  return (
    <View style={[styles.row, !last && styles.rowBorder]}>
      <View style={styles.rowContent}>
        <Text style={styles.rowLabel}>{label}</Text>
        <Text style={styles.rowSublabel}>{sublabel}</Text>
      </View>
      <View style={[styles.statusPill, active ? styles.statusPillActive : styles.statusPillInactive]}>
        <View style={[styles.statusDot, { backgroundColor: active ? colors.accent : colors.dim }]} />
        <Text style={[styles.statusText, { color: active ? colors.accent : colors.dim }]}>
          {active ? "Aktif" : "Pasif"}
        </Text>
      </View>
    </View>
  );
}

export default function SettingsLabsScreen() {
  const insets = useSafeAreaInsets();

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />

      {/* Header */}
      <View style={styles.header}>
        <TouchableOpacity style={styles.backBtn} onPress={() => router.back()} activeOpacity={0.7}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Atış Alanı</Text>
        <View style={styles.backBtn} />
      </View>

      <ScrollView contentContainerStyle={{ paddingBottom: insets.bottom + 40 }}>
        {/* Info banner */}
        <View style={styles.infoBanner}>
          <Ionicons name="information-circle-outline" size={16} color={colors.accent} style={{ marginTop: 1 }} />
          <Text style={styles.infoText}>
            Bu özellikler arka planda çalışır, açma/kapama anahtarı yoktur
          </Text>
        </View>

        <View style={styles.sectionContent}>
          {FEATURES.map((f, i) => (
            <StatusRow key={f.label} {...f} last={i === FEATURES.length - 1} />
          ))}
        </View>
      </ScrollView>
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
    marginBottom: spacing.sm,
  },
  backBtn: {
    width: 40,
    height: 40,
    alignItems: "center",
    justifyContent: "center",
  },
  headerTitle: {
    flex: 1,
    textAlign: "center",
    fontSize: typography.md,
    fontWeight: "700",
    color: colors.head,
  },

  infoBanner: {
    flexDirection: "row",
    alignItems: "flex-start",
    gap: 10,
    marginHorizontal: spacing.md,
    marginBottom: spacing.md,
    backgroundColor: colors.accentDeep,
    borderRadius: radius.xl,
    borderWidth: 1,
    borderColor: colors.accentDim,
    padding: spacing.md,
  },
  infoText: {
    flex: 1,
    fontSize: typography.sm,
    color: colors.body,
    lineHeight: 20,
  },

  sectionContent: {
    marginHorizontal: spacing.md,
    backgroundColor: colors.surface,
    borderRadius: radius.xl,
    borderWidth: 1,
    borderColor: colors.border,
    overflow: "hidden",
  },

  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: spacing.md,
    paddingVertical: 14,
  },
  rowBorder: {
    borderBottomWidth: 1,
    borderBottomColor: "rgba(255,255,255,0.05)",
  },
  rowContent: { flex: 1, marginRight: spacing.sm },
  rowLabel: { fontSize: typography.sm, color: colors.body, fontWeight: "500" },
  rowSublabel: { fontSize: 11, color: colors.dim, marginTop: 2 },

  statusPill: {
    flexDirection: "row", alignItems: "center", gap: 5,
    paddingHorizontal: spacing.sm, paddingVertical: 4,
    borderRadius: radius.full, borderWidth: 1,
  },
  statusPillActive: { backgroundColor: colors.accentDeep, borderColor: colors.accentDim },
  statusPillInactive: { backgroundColor: colors.raised, borderColor: colors.border },
  statusDot: { width: 6, height: 6, borderRadius: 3 },
  statusText: { fontSize: 11, fontWeight: "700" },
});
