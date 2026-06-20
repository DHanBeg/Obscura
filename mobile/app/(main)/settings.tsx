import React, { useState, useCallback } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, StyleSheet,
  Switch, Alert, StatusBar, TextInput, Modal, ActivityIndicator,
} from "react-native";
import { router } from "expo-router";
import * as SecureStore from "expo-secure-store";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { useStore } from "@/lib/store";
import { Avatar } from "@/components/ui/Avatar";
import { tierLabel, tierColor } from "@/lib/format";
import { api } from "@/lib/api";

interface RowProps {
  icon: string;
  iconBg?: string;
  iconColor?: string;
  label: string;
  sublabel?: string;
  value?: string;
  chevron?: boolean;
  danger?: boolean;
  toggle?: { value: boolean; onChange: (v: boolean) => void };
  onPress?: () => void;
  last?: boolean;
}

function Row({ icon, iconBg, iconColor, label, sublabel, value, chevron, danger, toggle, onPress, last }: RowProps) {
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
  const { user, setUser, reset } = useStore();
  const [readReceipts, setReadReceipts] = useState(true);
  const [onlineStatus, setOnlineStatus] = useState(true);
  const [notifications, setNotifications] = useState(true);
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [editName, setEditName] = useState(user?.display_name || "");
  const [editUsername, setEditUsername] = useState(user?.username || "");
  const [saving, setSaving] = useState(false);

  const saveProfile = useCallback(async () => {
    setSaving(true);
    try {
      const updated = await api.updateMe({
        display_name: editName.trim() || undefined,
        username: editUsername.trim() || undefined,
      });
      setUser(updated);
      setEditModalVisible(false);
    } catch (e: any) {
      Alert.alert("Hata", e.message);
    } finally {
      setSaving(false);
    }
  }, [editName, editUsername, setUser]);

  const logout = useCallback(() => {
    Alert.alert("Çıkış Yap", "Hesabınızdan çıkmak istediğinizden emin misiniz?", [
      { text: "İptal", style: "cancel" },
      {
        text: "Çıkış Yap", style: "destructive",
        onPress: async () => {
          await SecureStore.deleteItemAsync("obscura_token");
          reset();
          router.replace("/(auth)/login");
        },
      },
    ]);
  }, [reset]);

  const name = user?.display_name || user?.username || "Kullanıcı";
  const tierName = user ? tierLabel(user.tier) : "";
  const tierClr = user ? tierColor(user.tier) : colors.sub;

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />
      <View style={styles.header}>
        <Text style={styles.headerTitle}>Ayarlar</Text>
      </View>

      <ScrollView contentContainerStyle={{ paddingBottom: insets.bottom + 100 }}>
        {/* Profile card */}
        <View style={styles.profileCard}>
          <Avatar name={name} size="lg" tier={user?.tier} />
          <View style={styles.profileInfo}>
            <Text style={styles.profileName}>{name}</Text>
            {user?.username && <Text style={styles.profileUsername}>@{user.username}</Text>}
            <Text style={[styles.profileTier, { color: tierClr }]}>{tierName}</Text>
          </View>
          <TouchableOpacity
            style={styles.editBtn}
            onPress={() => {
              setEditName(user?.display_name || "");
              setEditUsername(user?.username || "");
              setEditModalVisible(true);
            }}
          >
            <Text style={styles.editBtnText}>Düzenle</Text>
          </TouchableOpacity>
        </View>

        {/* E2EE fingerprint */}
        {user?.did && (
          <View style={styles.fingerprintCard}>
            <Ionicons name="finger-print" size={16} color={colors.dim} />
            <Text style={styles.fingerprintText} numberOfLines={1}>{user.did}</Text>
          </View>
        )}

        <Section title="Güvenlik">
          <Row icon="shield-checkmark" iconBg={colors.accentDeep} iconColor={colors.accent}
            label="Uçtan Uca Şifreleme" sublabel="X3DH + Double Ratchet aktif" value="Aktif" last={false} />
          <Row icon="key-outline" iconBg="#1a1a2e" iconColor="#4a9eff"
            label="Şifreleme Anahtarları" sublabel="X3DH + Double Ratchet" chevron last={false} />
          <Row icon="lock-closed-outline" iconBg="#1a1a3e" iconColor="#a78bfa"
            label="Uygulama Kilidi" chevron last />
        </Section>

        <Section title="Gizlilik">
          <Row icon="checkmark-done-outline" iconBg={colors.muted} label="Okundu Bildirimleri"
            sublabel="Mesajları okuduğunuzda bildirilir"
            toggle={{ value: readReceipts, onChange: setReadReceipts }} last={false} />
          <Row icon="eye-outline" iconBg={colors.muted} label="Çevrimiçi Durumu"
            sublabel="Bağlandığınızda görünür"
            toggle={{ value: onlineStatus, onChange: setOnlineStatus }} last />
        </Section>

        <Section title="Bildirimler">
          <Row icon="notifications-outline" iconBg="#2a1a00" iconColor={colors.amber}
            label="Push Bildirimleri"
            toggle={{ value: notifications, onChange: setNotifications }} last />
        </Section>

        <Section title="Hakkında">
          <Row icon="information-circle-outline" iconBg={colors.muted}
            label="Sürüm" value="1.0.0" last={false} />
          <Row icon="document-text-outline" iconBg={colors.muted}
            label="Gizlilik Politikası" chevron last />
        </Section>

        <Section>
          <Row icon="log-out-outline" iconBg="rgba(239,68,68,0.15)" iconColor={colors.red}
            label="Çıkış Yap" danger onPress={logout} last />
        </Section>
      </ScrollView>

      {/* Edit Profile Modal */}
      <Modal visible={editModalVisible} animationType="slide" transparent onRequestClose={() => setEditModalVisible(false)}>
        <View style={styles.modalOverlay}>
          <View style={[styles.modalContent, { paddingBottom: insets.bottom + 16 }]}>
            <View style={styles.modalHeader}>
              <Text style={styles.modalTitle}>Profili Düzenle</Text>
              <TouchableOpacity onPress={() => setEditModalVisible(false)} style={styles.modalClose}>
                <Ionicons name="close" size={20} color={colors.sub} />
              </TouchableOpacity>
            </View>
            <Text style={styles.inputLabel}>Görünen Ad</Text>
            <TextInput
              style={styles.input}
              value={editName}
              onChangeText={setEditName}
              placeholder="Adınız"
              placeholderTextColor={colors.dim}
              maxLength={50}
            />
            <Text style={styles.inputLabel}>Kullanıcı Adı</Text>
            <TextInput
              style={styles.input}
              value={editUsername}
              onChangeText={(t) => setEditUsername(t.toLowerCase().replace(/[^a-z0-9_.]/g, ""))}
              placeholder="kullaniciadi"
              placeholderTextColor={colors.dim}
              maxLength={32}
              autoCapitalize="none"
            />
            <TouchableOpacity style={styles.saveBtn} onPress={saveProfile} disabled={saving}>
              {saving ? (
                <ActivityIndicator size="small" color={colors.white} />
              ) : (
                <Text style={styles.saveBtnText}>Kaydet</Text>
              )}
            </TouchableOpacity>
          </View>
        </View>
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },
  header: { paddingHorizontal: spacing.md, paddingVertical: spacing.sm },
  headerTitle: { fontSize: typography.xxl, fontWeight: "700", color: colors.head },
  profileCard: {
    flexDirection: "row", alignItems: "center", gap: 14,
    marginHorizontal: spacing.md, marginVertical: spacing.sm,
    backgroundColor: colors.surface, borderRadius: radius.xxl,
    borderWidth: 1, borderColor: colors.border,
    padding: spacing.md,
  },
  profileInfo: { flex: 1 },
  profileName: { fontSize: typography.md, fontWeight: "600", color: colors.head },
  profileUsername: { fontSize: typography.sm, color: colors.dim, marginTop: 1 },
  profileTier: { fontSize: 11, fontWeight: "500", marginTop: 3 },
  editBtn: {
    paddingHorizontal: 14, paddingVertical: 6,
    borderRadius: radius.xl, borderWidth: 1, borderColor: colors.border,
  },
  editBtnText: { fontSize: 12, color: colors.sub, fontWeight: "500" },
  fingerprintCard: {
    flexDirection: "row", alignItems: "center", gap: 8,
    marginHorizontal: spacing.md, marginBottom: spacing.sm,
    backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border,
    paddingHorizontal: spacing.md, paddingVertical: 10,
  },
  fingerprintText: { flex: 1, fontSize: 11, color: colors.dim, fontFamily: "monospace" },
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
  // Modal
  modalOverlay: {
    flex: 1, justifyContent: "flex-end",
    backgroundColor: "rgba(0,0,0,0.6)",
  },
  modalContent: {
    backgroundColor: colors.surface,
    borderTopLeftRadius: radius.xxl, borderTopRightRadius: radius.xxl,
    borderWidth: 1, borderColor: colors.border,
    paddingHorizontal: spacing.lg, paddingTop: spacing.lg,
    gap: spacing.sm,
  },
  modalHeader: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    marginBottom: spacing.sm,
  },
  modalTitle: { fontSize: typography.lg, fontWeight: "600", color: colors.head },
  modalClose: {
    width: 32, height: 32, borderRadius: radius.full,
    backgroundColor: colors.muted, alignItems: "center", justifyContent: "center",
  },
  inputLabel: { fontSize: 12, color: colors.dim, marginBottom: 4 },
  input: {
    backgroundColor: colors.raised,
    borderWidth: 1, borderColor: colors.border,
    borderRadius: radius.xl,
    paddingHorizontal: spacing.md, paddingVertical: 12,
    fontSize: typography.sm, color: colors.body,
    marginBottom: spacing.sm,
  },
  saveBtn: {
    backgroundColor: colors.accent, borderRadius: radius.xl,
    paddingVertical: 13, alignItems: "center", justifyContent: "center",
    marginTop: spacing.xs,
  },
  saveBtnText: { fontSize: typography.sm, color: colors.white, fontWeight: "600" },
});
