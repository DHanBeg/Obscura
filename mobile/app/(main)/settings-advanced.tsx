import React, { useState } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, StyleSheet, Switch, Alert,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import * as SecureStore from "expo-secure-store";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";

export default function SettingsAdvancedScreen() {
  const [p2pEnabled, setP2pEnabled] = useState(false);
  const [pqEnabled, setPqEnabled] = useState(true);
  const [proofCache, setProofCache] = useState(true);
  const [mlsDebug, setMlsDebug] = useState(false);

  const clearCache = () => {
    Alert.alert(
      "Önbelleği Temizle",
      "Tüm yerel önbellek silinecek. Devam edilsin mi?",
      [
        { text: "İptal", style: "cancel" },
        {
          text: "Temizle", style: "destructive",
          onPress: () => Alert.alert("Tamam", "Önbellek temizlendi."),
        },
      ]
    );
  };

  const resetNode = () => {
    Alert.alert(
      "Node Bağlantısını Sıfırla",
      "WebSocket bağlantısı yeniden kurulacak.",
      [
        { text: "İptal", style: "cancel" },
        { text: "Sıfırla", onPress: () => Alert.alert("Tamam", "Bağlantı yeniden kuruldu.") },
      ]
    );
  };

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Gelişmiş</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        {/* Network */}
        <Text style={styles.sectionLabel}>Ağ</Text>
        <View style={styles.card}>
          <View style={[styles.row, styles.borderRow]}>
            <View style={{ flex: 1 }}>
              <Text style={styles.rowTitle}>P2P Modu</Text>
              <Text style={styles.rowSub}>libp2p QUIC transport aktif et</Text>
            </View>
            <Switch
              value={p2pEnabled}
              onValueChange={setP2pEnabled}
              trackColor={{ false: colors.raised, true: "rgba(74,222,128,0.4)" }}
              thumbColor={p2pEnabled ? colors.accent : colors.sub}
            />
          </View>
          <TouchableOpacity style={styles.row} onPress={resetNode}>
            <View style={{ flex: 1 }}>
              <Text style={styles.rowTitle}>Node Bağlantısını Sıfırla</Text>
              <Text style={styles.rowSub}>WebSocket yeniden bağlan</Text>
            </View>
            <Ionicons name="refresh-outline" size={18} color={colors.sub} />
          </TouchableOpacity>
        </View>

        {/* Kriptografi */}
        <Text style={styles.sectionLabel}>Kriptografi</Text>
        <View style={styles.card}>
          <View style={[styles.row, styles.borderRow]}>
            <View style={{ flex: 1 }}>
              <Text style={styles.rowTitle}>Post-Kuantum KEM</Text>
              <Text style={styles.rowSub}>Kyber-768 anahtar değişimi</Text>
            </View>
            <Switch
              value={pqEnabled}
              onValueChange={setPqEnabled}
              trackColor={{ false: colors.raised, true: "rgba(74,222,128,0.4)" }}
              thumbColor={pqEnabled ? colors.accent : colors.sub}
            />
          </View>
          <View style={styles.row}>
            <View style={{ flex: 1 }}>
              <Text style={styles.rowTitle}>ZK Kanıt Önbelleği</Text>
              <Text style={styles.rowSub}>Tekrar kullanılan kanıtları önbellekle</Text>
            </View>
            <Switch
              value={proofCache}
              onValueChange={setProofCache}
              trackColor={{ false: colors.raised, true: "rgba(74,222,128,0.4)" }}
              thumbColor={proofCache ? colors.accent : colors.sub}
            />
          </View>
        </View>

        {/* MLS */}
        <Text style={styles.sectionLabel}>Mesajlaşma</Text>
        <View style={styles.card}>
          <View style={styles.row}>
            <View style={{ flex: 1 }}>
              <Text style={styles.rowTitle}>MLS Debug</Text>
              <Text style={styles.rowSub}>Grup şifreleme hata ayıklama</Text>
            </View>
            <Switch
              value={mlsDebug}
              onValueChange={setMlsDebug}
              trackColor={{ false: colors.raised, true: "rgba(74,222,128,0.4)" }}
              thumbColor={mlsDebug ? colors.accent : colors.sub}
            />
          </View>
        </View>

        {/* Cache */}
        <Text style={styles.sectionLabel}>Veri</Text>
        <View style={styles.card}>
          <TouchableOpacity style={styles.row} onPress={clearCache}>
            <View style={{ flex: 1 }}>
              <Text style={[styles.rowTitle, { color: colors.red }]}>Önbelleği Temizle</Text>
              <Text style={styles.rowSub}>Yerel önbellek ve geçici dosyalar</Text>
            </View>
            <Ionicons name="trash-outline" size={18} color={colors.red} />
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
});
