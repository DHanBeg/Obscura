import React, { useCallback } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, StyleSheet,
  Switch, Alert, StatusBar,
} from "react-native";
import { router } from "expo-router";
import * as SecureStore from "expo-secure-store";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { useStore } from "@/lib/store";
import { Avatar } from "@/components/ui/Avatar";
import { EncryptionBadge } from "@/components/ui/EncryptionBadge";

interface RowProps {
  icon: string;
  iconBg?: string;
  iconColor?: string;
  label: string;
  sublabel?: string;
  value?: string;
  chevron?: boolean;
  danger?: boolean;
  badge?: string;
  toggle?: { value: boolean; onChange: (v: boolean) => void };
  onPress?: () => void;
  last?: boolean;
}

function Row({ icon, iconBg, iconColor, label, sublabel, value, chevron, danger, badge, toggle, onPress, last }: RowProps) {
  return (
    <TouchableOpacity
      style={[styles.row, last && styles.rowLast]}
      onPress={onPress}
      disabled={!onPress && !toggle}
      activeOpacity={0.7}
    >
      <View style={[styles.rowIcon, { backgroundColor: iconBg ?? colors.muted }]}>
        <Ionicons name={icon as any} size={18} color={iconColor ?? colors.sub} />
      </View>
      <View style={styles.rowContent}>
        <Text style={[styles.rowLabel, danger && { color: colors.red }]}>{label}</Text>
        {sublabel && <Text style={styles.rowSublabel} numberOfLines={1}>{sublabel}</Text>}
      </View>
      {toggle && (
        <Switch
          value={toggle.value}
          onValueChange={toggle.onChange}
          trackColor={{ false: colors.muted, true: colors.accent }}
          thumbColor={colors.white}
        />
      )}
      {value && !toggle && <Text style={styles.rowValue}>{value}</Text>}
      {badge && (
        <View style={{ backgroundColor: colors.amber, borderRadius: 6, paddingHorizontal: 7, paddingVertical: 2, marginRight: 4 }}>
          <Text style={{ fontSize: 10, color: colors.void, fontWeight: "700" }}>{badge}</Text>
        </View>
      )}
      {chevron && <Ionicons name="chevron-forward" size={16} color={colors.dim} style={{ marginLeft: 4 }} />}
    </TouchableOpacity>
  );
}

function Section({ title, children }: { title?: string; children: React.ReactNode }) {
  return (
    <View style={styles.section}>
      {title && <Text style={styles.sectionTitle}>{title}</Text>}
      <View style={styles.sectionContent}>{children}</View>
    </View>
  );
}

export default function SettingsScreen() {
  const insets = useSafeAreaInsets();
  const { user, reset } = useStore();

  const logout = useCallback(() => {
    Alert.alert("Çıkış Yap", "Hesabınızdan çıkmak istediğinizden emin misiniz?", [
      { text: "İptal", style: "cancel" },
      {
        text: "Çıkış Yap", style: "destructive",
        onPress: async () => {
          reset();
          await SecureStore.deleteItemAsync("obscura_token");
          router.replace("/(auth)/login");
        },
      },
    ]);
  }, [reset]);

  const name = user?.display_name || user?.username || "Kullanıcı";

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />

      <View style={styles.header}>
        <Text style={styles.headerTitle}>Ayarlar</Text>
        <EncryptionBadge showLabel />
      </View>

      <ScrollView contentContainerStyle={{ paddingBottom: insets.bottom + 100 }}>
        {/* Profile card — compact horizontal */}
        <TouchableOpacity
          style={styles.profileCard}
          onPress={() => router.push("/(main)/profile" as any)}
          activeOpacity={0.8}
        >
          <Avatar name={name} size="md" tier={user?.tier} />
          <View style={styles.profileInfo}>
            <Text style={styles.profileName}>{name}</Text>
            {user?.username ? <Text style={styles.profileUsername}>@{user.username}</Text> : null}
          </View>
          <Text style={styles.profileDuzenle}>Düzenle</Text>
        </TouchableOpacity>

        {/* AYARLAR */}
        <Section title="AYARLAR">
          <Row
            icon="shield-checkmark"
            iconBg="rgba(74,222,128,0.1)"
            iconColor={colors.accent}
            label="Gizlilik ve Güvenlik"
            sublabel="Kim sizi görebilir, aramalar, mesaj koruması"
            chevron
            onPress={() => router.push("/(main)/settings-privacy" as any)}
            last={false}
          />
          <Row
            icon="notifications"
            iconBg="rgba(245,158,11,0.1)"
            iconColor={colors.amber}
            label="Bildirimler"
            sublabel="Ses, önizleme ve uyarı ayarları"
            chevron
            onPress={() => router.push("/(main)/settings-notifications" as any)}
            last={false}
          />
          <Row
            icon="color-palette"
            iconBg="rgba(99,102,241,0.1)"
            iconColor="#6366f1"
            label="Görünüm"
            sublabel="Tema ve sohbet düzeni"
            chevron
            onPress={() => router.push("/(main)/settings-appearance" as any)}
            last
          />
        </Section>

        {/* GELİŞMİŞ */}
        <Section title="GELİŞMİŞ">
          <Row
            icon="flask-outline"
            iconBg={colors.muted}
            label="Atış Alanı"
            sublabel="Deneysel özellikler"
            badge="BETA"
            chevron
            onPress={() => router.push("/(main)/settings-labs" as any)}
            last
          />
        </Section>

        {/* UYGULAMA */}
        <Section title="UYGULAMA">
          <Row
            icon="information-circle-outline"
            iconBg={colors.muted}
            label="Hakkında"
            sublabel="Sürüm, kriptografi, kütüphaneler"
            chevron
            onPress={() => router.push("/(main)/settings-about" as any)}
          />
          <Row
            icon="options-outline"
            iconBg={colors.muted}
            label="Gelişmiş"
            sublabel="P2P, post-kuantum, önbellek"
            chevron
            onPress={() => router.push("/(main)/settings-advanced" as any)}
          />
          <Row
            icon="phone-portrait-outline"
            iconBg={colors.muted}
            label="Çoklu Cihaz"
            sublabel="QR ile güvenli cihaz eşleştirme"
            chevron
            onPress={() => router.push("/(main)/settings-cross-signing" as any)}
          />
          <Row
            icon="code-slash-outline"
            iconBg={colors.muted}
            label="Geliştirici"
            sublabel="Debug modu, API override"
            chevron
            onPress={() => router.push("/(main)/settings-developer" as any)}
            last
          />
        </Section>

        {/* GELİŞTİRİCİ BİLGİLERİ */}
        <View style={styles.section}>
          <View style={[styles.sectionContent, styles.devSection]}>
            <View style={styles.devHeader}>
              <Ionicons name="code-slash" size={13} color={colors.accent} />
              <Text style={styles.devHeaderText}>GELİŞTİRİCİ BİLGİLERİ</Text>
            </View>
            <Row icon="globe-outline" iconBg={colors.muted} label="Node Seçimi" value="node-1 · 12ms" last={false} />
            <Row icon="hardware-chip-outline" iconBg={colors.muted} label="ZK Metrikleri" value="7 circuit" last />
          </View>
        </View>

        {/* TEHLİKELİ BÖLGE */}
        <Section title="TEHLİKELİ BÖLGE">
          <Row
            icon="log-out-outline"
            iconBg="rgba(239,68,68,0.15)"
            iconColor={colors.red}
            label="Oturumu Kapat"
            sublabel="Bu cihazdan çıkış yap"
            danger
            onPress={logout}
            last
          />
        </Section>
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
  },
  headerTitle: { fontSize: typography.xxl, fontWeight: "700", color: colors.head },

  // Profile card
  profileCard: {
    flexDirection: "row", alignItems: "center", gap: 14,
    marginHorizontal: spacing.md, marginBottom: spacing.md,
    backgroundColor: colors.surface, borderRadius: radius.xxl,
    borderWidth: 1, borderColor: colors.border,
    paddingHorizontal: spacing.md, paddingVertical: 12,
  },
  profileInfo: { flex: 1 },
  profileName: { fontSize: typography.base, fontWeight: "600", color: colors.head },
  profileUsername: { fontSize: typography.xs, color: colors.sub, marginTop: 1 },
  profileDuzenle: { fontSize: 13, color: colors.accent, fontWeight: "500" },

  // Sections
  section: { marginBottom: spacing.xs },
  sectionTitle: {
    fontSize: 11, fontWeight: "600", color: colors.dim,
    textTransform: "uppercase", letterSpacing: 1.2,
    paddingHorizontal: spacing.lg, paddingVertical: spacing.sm,
  },
  sectionContent: {
    marginHorizontal: spacing.md,
    backgroundColor: colors.surface,
    borderRadius: radius.xxl, borderWidth: 1, borderColor: colors.border,
    overflow: "hidden",
  },

  // Dev section overrides
  devSection: { backgroundColor: colors.raised, borderColor: "rgba(74,222,128,0.1)" },
  devHeader: {
    flexDirection: "row", alignItems: "center", gap: 6,
    paddingHorizontal: spacing.md, paddingTop: 12, paddingBottom: 8,
  },
  devHeaderText: { fontSize: 11, fontWeight: "700", color: colors.accent, letterSpacing: 1.2 },

  // Row
  row: {
    flexDirection: "row", alignItems: "center", gap: 12,
    paddingHorizontal: spacing.md, paddingVertical: 13,
    borderBottomWidth: 1, borderBottomColor: colors.border + "60",
  },
  rowLast: { borderBottomWidth: 0 },
  rowIcon: { width: 36, height: 36, borderRadius: radius.lg, alignItems: "center", justifyContent: "center" },
  rowContent: { flex: 1 },
  rowLabel: { fontSize: typography.sm, color: colors.body, fontWeight: "500" },
  rowSublabel: { fontSize: 11, color: colors.dim, marginTop: 1 },
  rowValue: { fontSize: 12, color: colors.dim },
});
