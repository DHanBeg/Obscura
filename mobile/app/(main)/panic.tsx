import React, { useCallback, useEffect, useState } from "react";
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
  ActivityIndicator,
  Alert,
  Platform,
} from "react-native";
import * as Location from "expo-location";
import * as Linking from "expo-linking";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { Avatar } from "@/components/ui/Avatar";
import { api } from "@/lib/api";
import { Contact, getTrustedContacts, hasTrustedContact } from "@/lib/trusted-contacts";
import { sendPanicAlert, PANIC_PRIVACY_NOTE } from "@/lib/panic";
import { triggerPanicAlert, PanicSendDeps, PermissionResult } from "@/lib/panic-flow";

// Gerçek bağımlılıklar — panic-flow.ts'in saf orkestrasyonuna burada bağlanır.
// Tek seferlik konum: arka plan takip YOK, yalnızca butona basınca bir kez.
const realDeps: PanicSendDeps = {
  requestPermission: async (): Promise<PermissionResult> => {
    const { status, canAskAgain } = await Location.requestForegroundPermissionsAsync();
    if (status === "granted") return "granted";
    if (status === "denied" && canAskAgain === false) return "blocked";
    return "denied";
  },
  getCurrentPosition: async () => {
    const pos = await Location.getCurrentPositionAsync({ accuracy: Location.Accuracy.Balanced });
    return { latitude: pos.coords.latitude, longitude: pos.coords.longitude };
  },
  send: sendPanicAlert,
};

export default function PanicScreen() {
  const insets = useSafeAreaInsets();
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedDid, setSelectedDid] = useState<string | null>(null);
  const [sending, setSending] = useState(false);

  const loadContacts = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.getContacts();
      const list: Contact[] = res?.contacts ?? [];
      setContacts(list);
      const trusted = getTrustedContacts(list);
      // Tek güven kişisi varsa otomatik seç — birden fazlaysa kullanıcı
      // bilinçli seçmeli (yanlış kişiye konum gitmesin).
      if (trusted.length === 1) setSelectedDid(trusted[0].did);
    } catch {
      setContacts([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadContacts();
  }, [loadContacts]);

  const trustedContacts = getTrustedContacts(contacts);

  const handlePress = () => {
    if (!selectedDid) return;
    const target = trustedContacts.find((c) => c.did === selectedDid);
    const name = target?.nickname || target?.display_name || target?.username || "seçili kişi";

    Alert.alert(
      "Panik Sinyali Gönderilsin mi?",
      `Kaba konumun (yaklaşık 1km) şifrelenip ${name}'e gönderilecek.`,
      [
        { text: "İptal", style: "cancel" },
        { text: "Gönder", style: "destructive", onPress: () => send(selectedDid) },
      ]
    );
  };

  const send = async (trustedContactDid: string) => {
    if (Platform.OS === "web") {
      Alert.alert("Desteklenmiyor", "Konum servisleri bu platformda kullanılamaz.");
      return;
    }
    setSending(true);
    try {
      const outcome = await triggerPanicAlert(trustedContactDid, realDeps);
      switch (outcome.status) {
        case "sent":
          Alert.alert("Gönderildi", "Panik sinyalin güven kişine ulaştı.");
          break;
        case "permission_denied":
          Alert.alert(
            "Konum İzni Gerekli",
            "Panik sinyali kaba konumunu taşır. İzin vermeden gönderilemez.",
            [
              { text: "İptal", style: "cancel" },
              { text: "Tekrar Dene", onPress: () => send(trustedContactDid) },
            ]
          );
          break;
        case "permission_blocked":
          Alert.alert(
            "Konum İzni Engellendi",
            "Cihaz ayarlarından Obscura uygulamasına konum izni vermelisin.",
            [
              { text: "İptal", style: "cancel" },
              { text: "Ayarlara Git", onPress: () => Linking.openSettings() },
            ]
          );
          break;
        case "error":
          Alert.alert("Hata", outcome.message || "Panik sinyali gönderilemedi.");
          break;
      }
    } finally {
      setSending(false);
    }
  };

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn} activeOpacity={0.7}>
          <Ionicons name="chevron-back" size={22} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Panik Butonu</Text>
        <View style={styles.backBtn} />
      </View>

      <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: insets.bottom + 32 }]}>
        {/* Dürüst gizlilik notu */}
        <View style={styles.infoCard}>
          <Ionicons name="lock-closed-outline" size={16} color={colors.sub} />
          <Text style={styles.infoText}>{PANIC_PRIVACY_NOTE}</Text>
        </View>

        {loading ? (
          <View style={styles.center}>
            <ActivityIndicator color={colors.accent} />
          </View>
        ) : !hasTrustedContact(contacts) ? (
          <View style={styles.emptyCard}>
            <Ionicons name="shield-outline" size={40} color={colors.sub} />
            <Text style={styles.emptyTitle}>Önce Bir Güven Kişisi Seç</Text>
            <Text style={styles.emptyDesc}>
              Panik butonu, konumunu yalnızca senin işaretlediğin güven
              kişisine gönderir. Henüz kimseyi güven kişisi olarak
              işaretlemedin.
            </Text>
            <TouchableOpacity
              style={styles.primaryBtn}
              onPress={() => router.push("/(main)/contacts" as any)}
              accessibilityRole="button"
              accessibilityLabel="Kişiler ekranına git"
            >
              <Ionicons name="people-outline" size={16} color={colors.void} />
              <Text style={styles.primaryBtnText}>Kişilere Git</Text>
            </TouchableOpacity>
          </View>
        ) : (
          <>
            <Text style={styles.sectionLabel}>Konumun kime gitsin?</Text>
            <View style={styles.contactList}>
              {trustedContacts.map((c) => {
                const name = c.nickname || c.display_name || c.username || c.did.slice(8, 20);
                const selected = c.did === selectedDid;
                return (
                  <TouchableOpacity
                    key={c.did}
                    style={[styles.contactRow, selected && styles.contactRowSelected]}
                    onPress={() => setSelectedDid(c.did)}
                    activeOpacity={0.7}
                    accessibilityRole="radio"
                    accessibilityState={{ selected }}
                  >
                    <Avatar name={name} size="sm" tier={c.tier} />
                    <Text style={styles.contactName} numberOfLines={1}>{name}</Text>
                    <Ionicons
                      name={selected ? "radio-button-on" : "radio-button-off"}
                      size={18}
                      color={selected ? colors.accent : colors.dim}
                    />
                  </TouchableOpacity>
                );
              })}
            </View>

            <TouchableOpacity
              style={[styles.panicBtn, (!selectedDid || sending) && styles.panicBtnDisabled]}
              onPress={handlePress}
              disabled={!selectedDid || sending}
              accessibilityRole="button"
              accessibilityLabel="Panik sinyali gönder"
            >
              {sending ? (
                <ActivityIndicator size="small" color={colors.void} />
              ) : (
                <>
                  <Ionicons name="warning" size={22} color={colors.void} />
                  <Text style={styles.panicBtnText}>PANİK</Text>
                </>
              )}
            </TouchableOpacity>
          </>
        )}
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
  backBtn: { width: 36, height: 36, alignItems: "center", justifyContent: "center" },
  headerTitle: { fontSize: typography.md, fontWeight: "700", color: colors.head },
  scroll: { padding: spacing.md, gap: spacing.md },
  infoCard: {
    flexDirection: "row", alignItems: "flex-start", gap: spacing.sm,
    backgroundColor: colors.surface, borderRadius: radius.md,
    borderWidth: 1, borderColor: colors.border,
    padding: spacing.md, marginBottom: spacing.md,
  },
  infoText: { fontSize: typography.sm, color: colors.sub, flex: 1, lineHeight: 18 },
  center: { alignItems: "center", justifyContent: "center", paddingVertical: 40 },
  emptyCard: {
    alignItems: "center", gap: spacing.sm,
    backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border,
    padding: spacing.lg,
  },
  emptyTitle: { fontSize: typography.md, fontWeight: "700", color: colors.head, textAlign: "center" },
  emptyDesc: { fontSize: typography.sm, color: colors.sub, textAlign: "center", lineHeight: 20 },
  primaryBtn: {
    flexDirection: "row", alignItems: "center", justifyContent: "center", gap: spacing.sm,
    height: 44, paddingHorizontal: spacing.lg, borderRadius: radius.lg,
    backgroundColor: colors.accent, marginTop: spacing.sm,
  },
  primaryBtnText: { fontSize: typography.base, fontWeight: "700", color: colors.void },
  sectionLabel: { fontSize: typography.sm, color: colors.sub, marginBottom: spacing.xs },
  contactList: {
    backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border, overflow: "hidden", marginBottom: spacing.lg,
  },
  contactRow: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm,
    padding: spacing.md, borderBottomWidth: 1, borderBottomColor: "rgba(255,255,255,0.04)",
  },
  contactRowSelected: { backgroundColor: colors.accent + "10" },
  contactName: { flex: 1, fontSize: typography.base, color: colors.head, fontWeight: "600" },
  panicBtn: {
    flexDirection: "row", alignItems: "center", justifyContent: "center", gap: spacing.sm,
    height: 56, borderRadius: radius.lg, backgroundColor: colors.red,
  },
  panicBtnDisabled: { opacity: 0.4 },
  panicBtnText: { fontSize: typography.lg, fontWeight: "800", color: colors.void, letterSpacing: 1 },
});
