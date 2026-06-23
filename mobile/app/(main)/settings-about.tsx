import React, { useState } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, StyleSheet, Clipboard, Alert,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";

const VERSION = "1.0.0";
const BUILD = "2026.06.22";

const CRYPTO_ITEMS = [
  "BN254 eliptik eğrisi (ZK kanıtları)",
  "X25519 ECDH (Diffie-Hellman)",
  "Ed25519 imzalama",
  "AES-256-GCM mesaj şifreleme",
  "SHA-256 / Poseidon hash",
  "Kyber-768 post-kuantum KEM",
  "Dilithium3 post-kuantum imza",
];

const LIBS = [
  { name: "snarkjs", desc: "Groth16 ZK kanıt üretimi" },
  { name: "Go 1.22", desc: "Backend node" },
  { name: "Rust + Circom 2.1.6", desc: "Kripto crate + ZK devre dili" },
  { name: "cloudflare/circl", desc: "Post-kuantum kriptografi" },
  { name: "libp2p", desc: "P2P ağ katmanı" },
  { name: "React Native + Expo", desc: "Mobil uygulama çerçevesi" },
];

export default function SettingsAboutScreen() {
  const [copied, setCopied] = useState(false);

  const copyVersion = () => {
    Clipboard.setString(`Obscura ${VERSION} (${BUILD})`);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Hakkında</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        {/* App info */}
        <View style={styles.appCard}>
          <Text style={styles.appName}>OBSCURA</Text>
          <Text style={styles.appTagline}>Sıfır Bilgi. Tam Gizlilik.</Text>
          <Text style={styles.appSpec}>Spec v3.0-FINAL</Text>
          <TouchableOpacity style={styles.versionRow} onPress={copyVersion}>
            <Text style={styles.versionText}>Sürüm {VERSION} ({BUILD})</Text>
            <Ionicons
              name={copied ? "checkmark-outline" : "copy-outline"}
              size={16}
              color={colors.sub}
            />
          </TouchableOpacity>
        </View>

        {/* Core crypto */}
        <Text style={styles.sectionLabel}>Çekirdek Kriptografi</Text>
        <View style={styles.card}>
          {CRYPTO_ITEMS.map((item, i) => (
            <View key={item} style={[styles.cryptoRow, i < CRYPTO_ITEMS.length - 1 && styles.cryptoRowBorder]}>
              <Ionicons name="lock-closed-outline" size={14} color={colors.accent} />
              <Text style={styles.cryptoText}>{item}</Text>
            </View>
          ))}
        </View>

        {/* Libraries */}
        <Text style={styles.sectionLabel}>Kullanılan Kütüphaneler</Text>
        <View style={styles.card}>
          {LIBS.map((lib, i) => (
            <View key={lib.name} style={[styles.libRow, i < LIBS.length - 1 && styles.libRowBorder]}>
              <Text style={styles.libName}>{lib.name}</Text>
              <Text style={styles.libDesc}>{lib.desc}</Text>
            </View>
          ))}
        </View>

        {/* Legal */}
        <Text style={styles.sectionLabel}>Hukuki</Text>
        <View style={styles.card}>
          <TouchableOpacity style={[styles.linkRow, styles.libRowBorder]} onPress={() => router.push("/(main)/settings")}>
            <Text style={styles.linkText}>Gizlilik Politikası</Text>
            <Ionicons name="chevron-forward" size={16} color={colors.sub} />
          </TouchableOpacity>
          <TouchableOpacity style={styles.linkRow} onPress={() => router.push("/(main)/settings")}>
            <Text style={styles.linkText}>Kullanım Koşulları</Text>
            <Ionicons name="chevron-forward" size={16} color={colors.sub} />
          </TouchableOpacity>
        </View>

        <Text style={styles.madeWith}>❤️ ile yapıldı</Text>
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
  content: { padding: spacing.lg, gap: spacing.md, paddingBottom: 48 },
  appCard: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: colors.border, padding: spacing.xl, alignItems: "center", gap: spacing.xs,
  },
  appName: { fontSize: typography.xxl, fontWeight: "800", color: colors.head, letterSpacing: -1 },
  appTagline: { fontSize: typography.sm, color: colors.sub },
  appSpec: { fontSize: typography.xs, color: colors.dim },
  versionRow: {
    flexDirection: "row", alignItems: "center", gap: spacing.xs,
    marginTop: spacing.sm,
  },
  versionText: { fontSize: typography.xs, color: colors.sub },
  sectionLabel: {
    fontSize: typography.xs, color: colors.sub, fontWeight: "600",
    textTransform: "uppercase", letterSpacing: 1, marginTop: spacing.xs,
  },
  card: {
    backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border, overflow: "hidden",
  },
  cryptoRow: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm,
    padding: spacing.md,
  },
  cryptoRowBorder: { borderBottomWidth: 1, borderBottomColor: colors.border },
  cryptoText: { fontSize: typography.sm, color: colors.body, flex: 1 },
  libRow: { padding: spacing.md },
  libRowBorder: { borderBottomWidth: 1, borderBottomColor: colors.border },
  libName: { fontSize: typography.sm, fontWeight: "600", color: colors.head, marginBottom: 2 },
  libDesc: { fontSize: typography.xs, color: colors.sub },
  linkRow: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    padding: spacing.md,
  },
  linkText: { fontSize: typography.sm, color: colors.body },
  madeWith: { textAlign: "center", fontSize: typography.xs, color: colors.dim, marginTop: spacing.sm },
});
