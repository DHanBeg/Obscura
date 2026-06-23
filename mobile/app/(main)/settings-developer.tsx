import React, { useState } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, StyleSheet, Switch, TextInput,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import Constants from "expo-constants";

const DEFAULT_API = Constants.expoConfig?.extra?.apiUrl ?? "https://obscura-backend-production-1827.up.railway.app";

export default function SettingsDeveloperScreen() {
  const [devMode, setDevMode] = useState(false);
  const [verboseLogs, setVerboseLogs] = useState(false);
  const [mockBackend, setMockBackend] = useState(false);
  const [apiOverride, setApiOverride] = useState("");
  const [zkSkip, setZkSkip] = useState(false);

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Geliştirici</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        <View style={styles.warningBox}>
          <Ionicons name="warning-outline" size={18} color={colors.amber} />
          <Text style={styles.warningText}>
            Bu ayarlar yalnızca geliştirici kullanımı içindir.
          </Text>
        </View>

        {/* Dev mode */}
        <Text style={styles.sectionLabel}>Mod</Text>
        <View style={styles.card}>
          <View style={[styles.row, styles.borderRow]}>
            <View style={{ flex: 1 }}>
              <Text style={styles.rowTitle}>Geliştirici Modu</Text>
              <Text style={styles.rowSub}>Ek bilgi ve hata ayıklama araçları</Text>
            </View>
            <Switch
              value={devMode}
              onValueChange={setDevMode}
              trackColor={{ false: colors.raised, true: "rgba(74,222,128,0.4)" }}
              thumbColor={devMode ? colors.accent : colors.sub}
            />
          </View>
          <View style={[styles.row, styles.borderRow]}>
            <View style={{ flex: 1 }}>
              <Text style={styles.rowTitle}>Ayrıntılı Loglar</Text>
              <Text style={styles.rowSub}>Konsola tüm API isteklerini yaz</Text>
            </View>
            <Switch
              value={verboseLogs}
              onValueChange={setVerboseLogs}
              trackColor={{ false: colors.raised, true: "rgba(74,222,128,0.4)" }}
              thumbColor={verboseLogs ? colors.accent : colors.sub}
            />
          </View>
          <View style={styles.row}>
            <View style={{ flex: 1 }}>
              <Text style={styles.rowTitle}>ZK Kanıtı Atla</Text>
              <Text style={styles.rowSub}>Test ortamında ZK kanıtı üretme</Text>
            </View>
            <Switch
              value={zkSkip}
              onValueChange={setZkSkip}
              trackColor={{ false: colors.raised, true: "rgba(239,68,68,0.4)" }}
              thumbColor={zkSkip ? colors.red : colors.sub}
            />
          </View>
        </View>

        {/* API */}
        <Text style={styles.sectionLabel}>Bağlantı</Text>
        <View style={styles.card}>
          <View style={styles.apiRow}>
            <Text style={styles.rowTitle}>API Endpoint</Text>
            <Text style={styles.rowSub}>{DEFAULT_API}</Text>
          </View>
          <View style={[styles.borderRow, { borderTopWidth: 1, borderTopColor: colors.border }]}>
            <Text style={[styles.rowSub, { padding: spacing.md, paddingBottom: 4 }]}>
              Override (boş = varsayılan)
            </Text>
            <TextInput
              style={styles.apiInput}
              value={apiOverride}
              onChangeText={setApiOverride}
              placeholder="https://..."
              placeholderTextColor={colors.dim}
              autoCapitalize="none"
              keyboardType="url"
            />
          </View>
          <View style={[styles.row, { borderTopWidth: 1, borderTopColor: colors.border }]}>
            <View style={{ flex: 1 }}>
              <Text style={styles.rowTitle}>Mock Backend</Text>
              <Text style={styles.rowSub}>Sahte yanıtlar kullan</Text>
            </View>
            <Switch
              value={mockBackend}
              onValueChange={setMockBackend}
              trackColor={{ false: colors.raised, true: "rgba(74,222,128,0.4)" }}
              thumbColor={mockBackend ? colors.accent : colors.sub}
            />
          </View>
        </View>

        {/* Dev tools shortcut */}
        <Text style={styles.sectionLabel}>Araçlar</Text>
        <View style={styles.card}>
          <TouchableOpacity style={styles.row} onPress={() => router.push("/(main)/dev-tools")}>
            <View style={{ flex: 1 }}>
              <Text style={styles.rowTitle}>Geliştirici Araçları</Text>
              <Text style={styles.rowSub}>Node logları, ZK metrikleri, WS debug</Text>
            </View>
            <Ionicons name="chevron-forward" size={18} color={colors.sub} />
          </TouchableOpacity>
        </View>
      </ScrollView>
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
  content: { padding: spacing.lg, gap: spacing.sm, paddingBottom: 48 },
  warningBox: {
    flexDirection: "row", gap: spacing.sm, alignItems: "center",
    backgroundColor: "rgba(245,158,11,0.08)", borderRadius: radius.xl,
    borderWidth: 1, borderColor: "rgba(245,158,11,0.2)", padding: spacing.md,
  },
  warningText: { flex: 1, fontSize: typography.sm, color: colors.amber, lineHeight: 20 },
  sectionLabel: {
    fontSize: typography.xs, color: colors.sub, fontWeight: "600",
    textTransform: "uppercase", letterSpacing: 1, marginTop: spacing.xs,
  },
  card: {
    backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border, overflow: "hidden",
  },
  row: {
    flexDirection: "row", alignItems: "center", gap: spacing.md,
    padding: spacing.md, minHeight: 56,
  },
  borderRow: { borderBottomWidth: 1, borderBottomColor: colors.border },
  rowTitle: { fontSize: typography.sm, fontWeight: "500", color: colors.head, marginBottom: 2 },
  rowSub: { fontSize: typography.xs, color: colors.sub },
  apiRow: { padding: spacing.md },
  apiInput: {
    marginHorizontal: spacing.md, marginBottom: spacing.md,
    backgroundColor: colors.raised, borderRadius: radius.lg, borderWidth: 1,
    borderColor: colors.border, padding: spacing.sm, color: colors.head,
    fontSize: typography.xs, fontFamily: "monospace",
  },
});
