import React, { useCallback, useEffect, useState } from "react";
import {
  View, Text, TouchableOpacity, StyleSheet, ScrollView, TextInput,
  ActivityIndicator, Alert, StatusBar, Switch, Share,
} from "react-native";
import { useLocalSearchParams, router, useFocusEffect } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import * as ImagePicker from "expo-image-picker";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { Avatar } from "@/components/ui/Avatar";
import { api } from "@/lib/api";
import { useStore } from "@/lib/store";
import { displayIdentifier } from "@/lib/odi-display";

interface ConvDetail {
  id: string; name: string; conv_type: string; description: string;
  avatar_url: string; is_public: boolean; member_count: number;
  my_role: "admin" | "member";
}

interface Member {
  did: string; odi?: string; display_name: string; avatar_url: string;
  tier: number; role: "admin" | "member"; joined_at: string;
}

const TYPE_LABELS: Record<string, string> = {
  group: "Grup", channel: "Kanal", community: "Topluluk",
};

export default function GroupProfileScreen() {
  const { convId } = useLocalSearchParams<{ convId: string }>();
  const insets = useSafeAreaInsets();
  const currentUser = useStore((s) => s.user);
  const setConversations = useStore((s) => s.setConversations);
  const conversations = useStore((s) => s.conversations);

  const [conv, setConv] = useState<ConvDetail | null>(null);
  const [members, setMembers] = useState<Member[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState("");
  const [editDesc, setEditDesc] = useState("");
  const [saving, setSaving] = useState(false);
  const [uploadingAvatar, setUploadingAvatar] = useState(false);

  const load = useCallback(async () => {
    if (!convId) return;
    try {
      const [detail, memberList] = await Promise.all([
        api.getConversation(convId),
        api.getConversationMembers(convId),
      ]);
      setConv(detail);
      setMembers(memberList);
      setEditName(detail.name);
      setEditDesc(detail.description || "");
      setError("");
    } catch {
      setError("Bilgiler yüklenemedi");
    } finally {
      setLoading(false);
    }
  }, [convId]);

  useEffect(() => { load(); }, [load]);
  useFocusEffect(useCallback(() => { load(); }, [load]));

  const typeLabel = conv ? (TYPE_LABELS[conv.conv_type] || "Sohbet") : "";
  const isAdmin = conv?.my_role === "admin";

  const handleSaveEdit = async () => {
    if (!convId || !editName.trim()) return;
    setSaving(true);
    try {
      await api.updateConversation(convId, { name: editName.trim(), description: editDesc.trim() });
      setEditing(false);
      await load();
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Güncellenemedi");
    } finally {
      setSaving(false);
    }
  };

  const handleTogglePublic = async (val: boolean) => {
    if (!convId) return;
    setConv((c) => (c ? { ...c, is_public: val } : c));
    try {
      await api.updateConversation(convId, { is_public: val });
    } catch (e: any) {
      setConv((c) => (c ? { ...c, is_public: !val } : c));
      Alert.alert("Hata", e?.message || "Güncellenemedi");
    }
  };

  const handlePickAvatar = async () => {
    if (!convId || !isAdmin) return;
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
    setUploadingAvatar(true);
    try {
      const uploaded = await api.uploadMedia(
        { uri: asset.uri, name: `conv_avatar_${Date.now()}.jpg`, type: "image/jpeg" },
        "avatar"
      );
      await api.updateConversation(convId, { avatar_url: uploaded.url });
      setConv((c) => (c ? { ...c, avatar_url: uploaded.url } : c));
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Fotoğraf yüklenemedi");
    } finally {
      setUploadingAvatar(false);
    }
  };

  const handleShare = async () => {
    if (!convId) return;
    try {
      const res = await api.createConvInvite(convId, {});
      await Share.share({ message: `Obscura'da bize katıl: ${res.invite_url}` });
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Davet linki oluşturulamadı");
    }
  };

  const handleLeave = () => {
    if (!convId) return;
    Alert.alert(
      `${typeLabel}dan Ayrıl`,
      "Bu konuşmadan ayrılmak istediğinize emin misiniz?",
      [
        { text: "Vazgeç", style: "cancel" },
        {
          text: "Ayrıl", style: "destructive",
          onPress: async () => {
            try {
              await api.leaveConversation(convId);
              setConversations(conversations.filter((c) => c.id !== convId));
              router.replace("/(main)/chats" as any);
            } catch (e: any) {
              Alert.alert("Hata", e?.message || "Ayrılınamadı");
            }
          },
        },
      ],
    );
  };

  const handleRemoveMember = (m: Member) => {
    if (!convId) return;
    Alert.alert(
      "Üyeyi At",
      `${m.display_name || displayIdentifier(m)} konuşmadan atılsın mı?`,
      [
        { text: "Vazgeç", style: "cancel" },
        {
          text: "At", style: "destructive",
          onPress: async () => {
            try {
              await api.removeConvMember(convId, m.did);
              setMembers((ms) => ms.filter((x) => x.did !== m.did));
            } catch (e: any) {
              Alert.alert("Hata", e?.message || "Atılamadı");
            }
          },
        },
      ],
    );
  };

  const handleToggleRole = async (m: Member) => {
    if (!convId) return;
    const newRole = m.role === "admin" ? "member" : "admin";
    try {
      await api.updateMemberRole(convId, m.did, newRole);
      setMembers((ms) => ms.map((x) => (x.did === m.did ? { ...x, role: newRole } : x)));
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Rol değiştirilemedi");
    }
  };

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />

      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>{typeLabel} Bilgisi</Text>
        {isAdmin ? (
          <TouchableOpacity onPress={() => setEditing((v) => !v)} style={styles.backBtn}>
            <Ionicons name={editing ? "close" : "create-outline"} size={22} color={colors.accent} />
          </TouchableOpacity>
        ) : (
          <View style={styles.backBtn} />
        )}
      </View>

      {loading ? (
        <View style={styles.center}><ActivityIndicator color={colors.accent} /></View>
      ) : error || !conv ? (
        <View style={styles.center}>
          <Ionicons name="alert-circle-outline" size={40} color={colors.red} />
          <Text style={styles.errorText}>{error || "Konuşma bulunamadı"}</Text>
        </View>
      ) : (
        <ScrollView contentContainerStyle={styles.content}>
          <View style={styles.heroSection}>
            <TouchableOpacity
              onPress={handlePickAvatar}
              disabled={!isAdmin || uploadingAvatar}
              activeOpacity={isAdmin ? 0.7 : 1}
              style={styles.avatarWrap}
            >
              <Avatar name={conv.name} size="xl" imageUrl={conv.avatar_url || undefined} />
              {isAdmin && (
                <View style={styles.avatarEditBadge}>
                  {uploadingAvatar
                    ? <ActivityIndicator size="small" color={colors.void} />
                    : <Ionicons name="camera" size={14} color={colors.void} />}
                </View>
              )}
            </TouchableOpacity>
            {editing ? (
              <View style={styles.editBlock}>
                <TextInput
                  style={styles.editInput}
                  value={editName}
                  onChangeText={setEditName}
                  placeholder="Ad"
                  placeholderTextColor={colors.dim}
                />
                <TextInput
                  style={[styles.editInput, styles.editInputMulti]}
                  value={editDesc}
                  onChangeText={setEditDesc}
                  placeholder="Açıklama"
                  placeholderTextColor={colors.dim}
                  multiline
                />
                <TouchableOpacity style={styles.saveBtn} onPress={handleSaveEdit} disabled={saving}>
                  {saving ? <ActivityIndicator color={colors.void} /> : <Text style={styles.saveBtnText}>Kaydet</Text>}
                </TouchableOpacity>
              </View>
            ) : (
              <>
                <Text style={styles.displayName}>{conv.name}</Text>
                {!!conv.description && <Text style={styles.description}>{conv.description}</Text>}
                <View style={styles.typeBadge}>
                  <Ionicons
                    name={conv.conv_type === "channel" ? "megaphone-outline" : conv.conv_type === "community" ? "planet-outline" : "people-outline"}
                    size={12} color={colors.accent}
                  />
                  <Text style={styles.typeText}>{typeLabel} · {conv.member_count} üye</Text>
                </View>
              </>
            )}
          </View>

          {isAdmin && (
            <View style={styles.settingRow}>
              <Text style={styles.settingLabel}>Herkese Açık (Keşfet'te görünür)</Text>
              <Switch
                value={conv.is_public}
                onValueChange={handleTogglePublic}
                trackColor={{ false: colors.muted, true: colors.accentDim }}
                thumbColor={conv.is_public ? colors.accent : colors.sub}
              />
            </View>
          )}

          <View style={styles.actions}>
            <TouchableOpacity style={styles.actionBtn} onPress={handleShare} activeOpacity={0.8}>
              <Ionicons name="share-outline" size={22} color={colors.void} />
              <Text style={styles.actionLabel}>Davet Et</Text>
            </TouchableOpacity>
            <TouchableOpacity style={[styles.actionBtn, styles.actionBtnDanger]} onPress={handleLeave} activeOpacity={0.8}>
              <Ionicons name="exit-outline" size={22} color={colors.red} />
              <Text style={[styles.actionLabel, { color: colors.red }]}>Ayrıl</Text>
            </TouchableOpacity>
          </View>

          <View style={styles.membersSection}>
            <Text style={styles.sectionTitle}>Üyeler ({members.length})</Text>
            {members.map((m) => {
              const isSelf = m.did === currentUser?.did;
              return (
                <TouchableOpacity
                  key={m.did}
                  style={styles.memberRow}
                  activeOpacity={0.7}
                  onPress={() => !isSelf && router.push(`/(main)/user-profile?did=${m.did}&name=${encodeURIComponent(m.display_name)}` as any)}
                >
                  <Avatar name={m.display_name || displayIdentifier(m)} size="sm" imageUrl={m.avatar_url || undefined} tier={m.tier} />
                  <View style={styles.memberInfo}>
                    <Text style={styles.memberName} numberOfLines={1}>
                      {m.display_name || displayIdentifier(m)}{isSelf ? " (Siz)" : ""}
                    </Text>
                    {m.role === "admin" && <Text style={styles.memberRole}>Yönetici</Text>}
                  </View>
                  {isAdmin && !isSelf && (
                    <View style={styles.memberActions}>
                      <TouchableOpacity onPress={() => handleToggleRole(m)} style={styles.memberActionBtn}>
                        <Ionicons name={m.role === "admin" ? "shield" : "shield-outline"} size={18} color={colors.sub} />
                      </TouchableOpacity>
                      <TouchableOpacity onPress={() => handleRemoveMember(m)} style={styles.memberActionBtn}>
                        <Ionicons name="person-remove-outline" size={18} color={colors.red} />
                      </TouchableOpacity>
                    </View>
                  )}
                </TouchableOpacity>
              );
            })}
          </View>
        </ScrollView>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },
  header: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    paddingHorizontal: spacing.lg, paddingVertical: spacing.md,
    borderBottomWidth: 1, borderBottomColor: colors.border,
  },
  backBtn: { width: 40, height: 40, alignItems: "center", justifyContent: "center" },
  headerTitle: { fontSize: typography.md, fontWeight: "700", color: colors.head },
  center: { flex: 1, alignItems: "center", justifyContent: "center", gap: 12 },
  errorText: { fontSize: typography.sm, color: colors.red },
  content: { padding: spacing.xl, gap: spacing.lg },
  heroSection: { alignItems: "center", gap: spacing.sm, paddingTop: spacing.lg },
  avatarWrap: { position: "relative" },
  avatarEditBadge: {
    position: "absolute", bottom: 0, right: 0,
    width: 26, height: 26, borderRadius: 13,
    backgroundColor: colors.accent, borderWidth: 2, borderColor: colors.void,
    alignItems: "center", justifyContent: "center",
  },
  displayName: { fontSize: typography.xxl, fontWeight: "700", color: colors.head, textAlign: "center" },
  description: { fontSize: typography.sm, color: colors.sub, textAlign: "center", paddingHorizontal: spacing.md },
  typeBadge: {
    flexDirection: "row", alignItems: "center", gap: 4,
    paddingHorizontal: 10, paddingVertical: 4,
    backgroundColor: "rgba(74,222,128,0.1)",
    borderWidth: 1, borderColor: "rgba(74,222,128,0.2)",
    borderRadius: radius.full,
  },
  typeText: { fontSize: 11, color: colors.accent, fontWeight: "600" },
  editBlock: { width: "100%", gap: spacing.sm },
  editInput: {
    backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.border,
    borderRadius: radius.md, padding: spacing.sm, color: colors.head, fontSize: typography.base,
  },
  editInputMulti: { minHeight: 70, textAlignVertical: "top" },
  saveBtn: {
    backgroundColor: colors.accent, borderRadius: radius.md,
    paddingVertical: 10, alignItems: "center",
  },
  saveBtnText: { color: colors.void, fontWeight: "700", fontSize: typography.sm },
  settingRow: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border, padding: spacing.md,
  },
  settingLabel: { fontSize: typography.sm, color: colors.head, flex: 1, marginRight: spacing.sm },
  actions: { flexDirection: "row", gap: spacing.sm },
  actionBtn: {
    flex: 1, flexDirection: "column", alignItems: "center", justifyContent: "center",
    gap: 6, paddingVertical: 14, borderRadius: radius.xl,
    backgroundColor: colors.accent,
  },
  actionBtnDanger: { backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.border },
  actionLabel: { fontSize: 11, fontWeight: "600", color: colors.void },
  membersSection: { gap: spacing.xs },
  sectionTitle: { fontSize: 10, color: colors.dim, textTransform: "uppercase", letterSpacing: 1, marginBottom: spacing.xs },
  memberRow: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm,
    backgroundColor: colors.surface, borderRadius: radius.lg,
    borderWidth: 1, borderColor: colors.border, padding: spacing.sm, marginBottom: spacing.xs,
  },
  memberInfo: { flex: 1 },
  memberName: { fontSize: typography.sm, color: colors.head, fontWeight: "600" },
  memberRole: { fontSize: 10, color: colors.accent, marginTop: 2 },
  memberActions: { flexDirection: "row", gap: spacing.xs },
  memberActionBtn: { width: 32, height: 32, alignItems: "center", justifyContent: "center" },
});
