import React, { useState, useCallback } from "react";
import * as ImagePicker from "expo-image-picker";
import {
  View, Text, ScrollView, TouchableOpacity, StyleSheet,
  TextInput, Alert, ActivityIndicator, StatusBar, Switch,
} from "react-native";
import * as Clipboard from "expo-clipboard";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { useStore } from "@/lib/store";
import { Avatar } from "@/components/ui/Avatar";
import { api } from "@/lib/api";

function SectionLabel({ children }: { children: React.ReactNode }) {
  return <Text style={styles.sectionLabel}>{children}</Text>;
}

function Card({ children }: { children: React.ReactNode }) {
  return <View style={styles.card}>{children}</View>;
}

function EditRow({
  icon, label, children, last,
}: { icon: string; label: string; children: React.ReactNode; last?: boolean }) {
  return (
    <View style={[styles.editRow, !last && styles.editRowBorder]}>
      <Ionicons name={icon as any} size={18} color={colors.accent} style={styles.editRowIcon} />
      <View style={styles.editRowContent}>
        <Text style={styles.editRowLabel}>{label}</Text>
        {children}
      </View>
    </View>
  );
}

export default function ProfileScreen() {
  const insets = useSafeAreaInsets();
  const { user, setUser } = useStore();
  const wsConnected = useStore((s: any) => s.wsConnected);

  const [displayName, setDisplayName] = useState(user?.display_name || "");
  const [username, setUsername] = useState(user?.username || "");
  const [bio, setBio] = useState((user as any)?.bio || "");
  const [hideOnline, setHideOnline] = useState<boolean>(!!(user as any)?.hide_online);
  const [saving, setSaving] = useState(false);
  const [uploading, setUploading] = useState(false);

  const name = user?.display_name || user?.username || "Kullanıcı";

  const save = useCallback(async () => {
    setSaving(true);
    try {
      const updated = await api.updateMe({
        display_name: displayName.trim() || undefined,
        username: username.trim() || undefined,
        bio: bio.trim() || undefined,
        hide_online: hideOnline ? 1 : 0,
      });
      setUser(updated);
      Alert.alert("Başarılı", "Profil güncellendi");
    } catch (e: any) {
      Alert.alert("Hata", e.message);
    } finally {
      setSaving(false);
    }
  }, [displayName, username, bio, setUser]);

  const pickAndUploadPhoto = useCallback(async () => {
    const perm = await ImagePicker.requestMediaLibraryPermissionsAsync();
    if (!perm.granted) {
      Alert.alert("İzin Gerekli", "Fotoğraf galerisine erişim izni verin");
      return;
    }
    const result = await ImagePicker.launchImageLibraryAsync({
      mediaTypes: ImagePicker.MediaTypeOptions.Images,
      allowsEditing: true,
      aspect: [1, 1],
      quality: 0.8,
    });
    if (result.canceled) return;
    const asset = result.assets[0];
    setUploading(true);
    try {
      const uploaded = await api.uploadMedia(
        { uri: asset.uri, name: `avatar_${Date.now()}.jpg`, type: "image/jpeg" },
        "avatar"
      );
      const updated = await api.updateMe({ avatar_url: uploaded.url });
      setUser(updated);
    } catch (e: any) {
      Alert.alert("Hata", "Fotoğraf yüklenemedi");
    } finally {
      setUploading(false);
    }
  }, [setUser]);

  const copyDID = async () => {
    if (user?.did) {
      await Clipboard.setStringAsync(user.did);
      Alert.alert("Kopyalandı", "DID panoya kopyalandı");
    }
  };

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />

      {/* Header */}
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.headerBackBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Profil</Text>
        <View style={styles.headerBackBtn} />
      </View>

      <ScrollView contentContainerStyle={{ paddingBottom: insets.bottom + 40 }} showsVerticalScrollIndicator={false}>

        {/* Avatar */}
        <View style={styles.avatarSection}>
          <View style={styles.avatarWrap}>
            <Avatar name={name} size="xl" tier={user?.tier} imageUrl={(user as any)?.avatar_url} />
            <TouchableOpacity style={styles.cameraBtn} onPress={pickAndUploadPhoto} disabled={uploading}>
              {uploading
                ? <ActivityIndicator size={10} color={colors.void} />
                : <Ionicons name="camera" size={14} color={colors.void} />}
            </TouchableOpacity>
          </View>
          <Text style={styles.profileName}>{name}</Text>
          {user?.username ? <Text style={styles.profileUser}>@{user.username}</Text> : null}
          <View style={[styles.onlinePill, (!wsConnected || hideOnline) && styles.offlinePill]}>
            <View style={[styles.onlineDot, (!wsConnected || hideOnline) && styles.offlineDot]} />
            <Text style={[styles.onlineText, (!wsConnected || hideOnline) && styles.offlineText]}>
              {!wsConnected ? "Çevrimdışı" : hideOnline ? "Gizli" : "Çevrimiçi"}
            </Text>
          </View>
        </View>

        {/* Gizlilik */}
        <SectionLabel>DURUM</SectionLabel>
        <Card>
          <View style={[styles.editRow, { alignItems: "center" }]}>
            <Ionicons name="eye-off-outline" size={18} color={colors.accent} style={styles.editRowIcon} />
            <View style={styles.editRowContent}>
              <Text style={styles.editRowLabel}>Çevrimiçi Durumu Gizle</Text>
              <Text style={{ fontSize: 11, color: colors.dim, marginTop: 2 }}>
                Diğerleri sizi çevrimdışı görür
              </Text>
            </View>
            <Switch
              value={hideOnline}
              onValueChange={setHideOnline}
              trackColor={{ false: colors.raised, true: "rgba(74,222,128,0.4)" }}
              thumbColor={hideOnline ? colors.accent : colors.sub}
            />
          </View>
        </Card>

        {/* Profil Bilgileri */}
        <SectionLabel>PROFİL BİLGİLERİ</SectionLabel>
        <Card>
          <EditRow icon="person-outline" label="Görünen Ad">
            <TextInput
              style={styles.input}
              value={displayName}
              onChangeText={(t) => setDisplayName(t.replace(/[‪-‮⁦-⁩​-‏]/g, ""))}
              placeholder="Adınız"
              placeholderTextColor={colors.sub}
              maxLength={50}
            />
          </EditRow>
          <EditRow icon="at-outline" label="Kullanıcı Adı">
            <TextInput
              style={styles.input}
              value={username}
              onChangeText={(t) => setUsername(t.toLowerCase().replace(/[^a-z0-9_.]/g, ""))}
              placeholder="kullaniciadi"
              placeholderTextColor={colors.sub}
              maxLength={32}
              autoCapitalize="none"
            />
          </EditRow>
          <EditRow icon="document-text-outline" label="Bio" last>
            <TextInput
              style={[styles.input, { height: 72, textAlignVertical: "top" }]}
              value={bio}
              onChangeText={(t) => setBio(t.replace(/[‪-‮⁦-⁩​-‏]/g, ""))}
              placeholder="Kendinizden bahsedin..."
              placeholderTextColor={colors.sub}
              maxLength={160}
              multiline
            />
          </EditRow>
        </Card>

        {/* Hesap */}
        <SectionLabel>HESAP</SectionLabel>
        <Card>
          <TouchableOpacity style={styles.infoRow} onPress={copyDID}>
            <Text style={styles.infoLabel}>DID</Text>
            <Text style={styles.infoSubLabel}>Merkeziyetsiz Kimlik</Text>
            <View style={styles.infoRowRight}>
              <Text style={styles.infoValue} numberOfLines={1}>
                {user?.did || "—"}
              </Text>
              <Ionicons name="copy-outline" size={16} color={colors.sub} style={{ marginLeft: 8 }} />
            </View>
          </TouchableOpacity>
        </Card>

        {/* ZK-ID */}
        <SectionLabel>KRİPTOGRAFİK KİMLİK</SectionLabel>
        <Card>
          <View style={styles.zkRow}>
            <Ionicons name="shield-checkmark" size={20} color={colors.accent} />
            <View style={styles.zkInfo}>
              <Text style={styles.zkTitle}>ZK-ID Doğrulandı</Text>
              <Text style={styles.zkSub}>Groth16 · BN254 eliptik eğrisi</Text>
            </View>
            <View style={styles.zkBadge}>
              <Ionicons name="checkmark" size={12} color={colors.accent} />
              <Text style={styles.zkBadgeText}>Aktif</Text>
            </View>
          </View>
        </Card>

        {/* Save */}
        <TouchableOpacity style={styles.saveBtn} onPress={save} disabled={saving}>
          {saving ? (
            <ActivityIndicator size="small" color={colors.void} />
          ) : (
            <Text style={styles.saveBtnText}>Değişiklikleri Kaydet</Text>
          )}
        </TouchableOpacity>
      </ScrollView>
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
  headerBackBtn: { width: 40, height: 40, alignItems: "center", justifyContent: "center" },
  headerTitle: { flex: 1, textAlign: "center", fontSize: typography.md, fontWeight: "700", color: colors.head },
  avatarSection: { alignItems: "center", paddingVertical: spacing.xl },
  avatarWrap: { position: "relative", marginBottom: spacing.md },
  cameraBtn: {
    position: "absolute", bottom: 2, right: 2,
    width: 26, height: 26, borderRadius: 13,
    backgroundColor: colors.accent,
    alignItems: "center", justifyContent: "center",
    borderWidth: 2, borderColor: colors.void,
  },
  profileName: { fontSize: typography.xl, fontWeight: "700", color: colors.head, marginBottom: 2 },
  profileUser: { fontSize: typography.sm, color: colors.sub, marginBottom: 8 },
  onlinePill: {
    flexDirection: "row", alignItems: "center", gap: 5,
    paddingHorizontal: 10, paddingVertical: 4,
    borderRadius: radius.full,
    backgroundColor: "rgba(74,222,128,0.1)",
    borderWidth: 1, borderColor: "rgba(74,222,128,0.2)",
  },
  onlineDot: { width: 7, height: 7, borderRadius: 4, backgroundColor: colors.accent },
  onlineText: { fontSize: 12, color: colors.accent, fontWeight: "500" },
  offlinePill: { backgroundColor: "rgba(107,114,128,0.1)", borderColor: "rgba(107,114,128,0.2)" },
  offlineDot: { backgroundColor: "#6b7280" },
  offlineText: { color: "#6b7280" },
  sectionLabel: {
    fontSize: 11, fontWeight: "700", color: colors.sub,
    letterSpacing: 1.5, textTransform: "uppercase",
    paddingHorizontal: spacing.md, paddingBottom: spacing.xs, paddingTop: spacing.sm,
  },
  card: {
    marginHorizontal: spacing.md, marginBottom: spacing.sm,
    backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border, overflow: "hidden",
  },
  editRow: { flexDirection: "row", alignItems: "flex-start", paddingHorizontal: spacing.md, paddingVertical: 12 },
  editRowBorder: { borderBottomWidth: 1, borderBottomColor: colors.border },
  editRowIcon: { marginTop: 14, marginRight: spacing.sm, flexShrink: 0 },
  editRowContent: { flex: 1 },
  editRowLabel: { fontSize: 11, color: colors.accent, fontWeight: "600", letterSpacing: 0.5, marginBottom: 4 },
  input: {
    fontSize: typography.sm, color: colors.head,
    paddingVertical: 4,
  },
  infoRow: {
    paddingHorizontal: spacing.md, paddingVertical: 14,
  },
  infoLabel: { fontSize: 11, color: colors.sub, marginBottom: 2 },
  infoSubLabel: { fontSize: typography.sm, color: colors.body, marginBottom: 4 },
  infoRowRight: { flexDirection: "row", alignItems: "center" },
  infoValue: { flex: 1, fontSize: 11, color: colors.dim, fontFamily: "monospace" },
  zkRow: {
    flexDirection: "row", alignItems: "center", gap: 12,
    paddingHorizontal: spacing.md, paddingVertical: 14,
  },
  zkInfo: { flex: 1 },
  zkTitle: { fontSize: typography.sm, fontWeight: "600", color: colors.head },
  zkSub: { fontSize: 11, color: colors.sub, marginTop: 2 },
  zkBadge: {
    flexDirection: "row", alignItems: "center", gap: 4,
    paddingHorizontal: 8, paddingVertical: 4,
    backgroundColor: "rgba(74,222,128,0.1)", borderRadius: radius.lg,
    borderWidth: 1, borderColor: "rgba(74,222,128,0.2)",
  },
  zkBadgeText: { fontSize: 11, color: colors.accent, fontWeight: "600" },
  saveBtn: {
    marginHorizontal: spacing.md, marginTop: spacing.md,
    backgroundColor: colors.accent, borderRadius: radius.xl,
    paddingVertical: 15, alignItems: "center",
  },
  saveBtnText: { fontSize: typography.base, fontWeight: "700", color: colors.void },
});
