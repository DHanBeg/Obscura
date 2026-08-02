import React, { useState, useEffect } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, StyleSheet,
  ActivityIndicator, Alert,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router, useLocalSearchParams } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";
import { deriveOdiFromDid } from "@/lib/odi-display";

interface Attendee { did: string; joined_at: string; checked_in?: boolean; }
interface Event {
  id: string; title: string; description?: string;
  location_name?: string; starts_at: string; ends_at?: string;
  capacity?: number; attendee_count?: number; creator_did?: string;
}

function formatDateTime(iso: string) {
  try {
    return new Date(iso).toLocaleString("tr-TR", {
      day: "numeric", month: "long", year: "numeric",
      hour: "2-digit", minute: "2-digit",
    });
  } catch { return iso; }
}

export default function EventDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const [event, setEvent] = useState<Event | null>(null);
  const [attendees, setAttendees] = useState<Attendee[]>([]);
  const [loading, setLoading] = useState(true);
  const [joining, setJoining] = useState(false);
  const [joined, setJoined] = useState(false);
  const [qrCode, setQrCode] = useState<string | null>(null);
  const [qrLoading, setQrLoading] = useState(false);

  useEffect(() => { if (id) loadEvent(); }, [id]);

  async function loadEvent() {
    setLoading(true);
    try {
      const [ev, att] = await Promise.all([
        api.getEvent(id),
        api.listAttendees(id),
      ]);
      setEvent(ev);
      const list: Attendee[] = Array.isArray(att) ? att : att?.attendees ?? [];
      setAttendees(list);
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Etkinlik yüklenemedi");
    } finally {
      setLoading(false);
    }
  }

  async function toggleJoin() {
    setJoining(true);
    try {
      if (joined) { await api.leaveEvent(id); setJoined(false); }
      else { await api.joinEvent(id); setJoined(true); }
      await loadEvent();
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "İşlem başarısız");
    } finally {
      setJoining(false);
    }
  }

  async function loadQR() {
    setQrLoading(true);
    try {
      const res = await api.getCheckinQR(id);
      setQrCode(res?.qr_token ?? null);
    } catch {}
    finally { setQrLoading(false); }
  }

  if (loading) {
    return (
      <SafeAreaView style={styles.root} edges={["top"]}>
        <View style={styles.header}>
          <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
            <Ionicons name="chevron-back" size={24} color={colors.head} />
          </TouchableOpacity>
          <Text style={styles.headerTitle}>Etkinlik</Text>
          <View style={{ width: 40 }} />
        </View>
        <View style={styles.center}>
          <ActivityIndicator size="large" color={colors.accent} />
        </View>
      </SafeAreaView>
    );
  }

  if (!event) {
    return (
      <SafeAreaView style={styles.root} edges={["top"]}>
        <View style={styles.header}>
          <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
            <Ionicons name="chevron-back" size={24} color={colors.head} />
          </TouchableOpacity>
          <Text style={styles.headerTitle}>Etkinlik</Text>
          <View style={{ width: 40 }} />
        </View>
        <View style={styles.center}>
          <Text style={styles.emptyText}>Etkinlik bulunamadı</Text>
        </View>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle} numberOfLines={1}>{event.title}</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        {/* Info card */}
        <View style={styles.card}>
          <Text style={styles.eventTitle}>{event.title}</Text>
          {event.description && (
            <Text style={styles.eventDesc}>{event.description}</Text>
          )}

          <View style={styles.metaList}>
            <View style={styles.metaRow}>
              <Ionicons name="time-outline" size={14} color={colors.sub} />
              <Text style={styles.metaText}>
                {formatDateTime(event.starts_at)}
                {event.ends_at ? ` — ${formatDateTime(event.ends_at)}` : ""}
              </Text>
            </View>
            {event.location_name && (
              <View style={styles.metaRow}>
                <Ionicons name="location-outline" size={14} color={colors.sub} />
                <Text style={styles.metaText}>{event.location_name}</Text>
              </View>
            )}
            <View style={styles.metaRow}>
              <Ionicons name="people-outline" size={14} color={colors.sub} />
              <Text style={styles.metaText}>
                {attendees.length}{event.capacity ? `/${event.capacity}` : ""} katılımcı
              </Text>
            </View>
          </View>
        </View>

        {/* Actions */}
        <View style={styles.actionsRow}>
          <TouchableOpacity
            style={[styles.joinBtn, joined && styles.joinBtnActive]}
            onPress={toggleJoin}
            disabled={joining}
          >
            {joining
              ? <ActivityIndicator size="small" color={joined ? colors.red : colors.void} />
              : <Ionicons
                  name={joined ? "person-remove-outline" : "person-add-outline"}
                  size={16}
                  color={joined ? colors.red : colors.void}
                />}
            <Text style={[styles.joinBtnText, joined && styles.joinBtnTextActive]}>
              {joined ? "Ayrıl" : "Katıl"}
            </Text>
          </TouchableOpacity>

          <TouchableOpacity style={styles.qrBtn} onPress={loadQR} disabled={qrLoading}>
            {qrLoading
              ? <ActivityIndicator size="small" color={colors.head} />
              : <Ionicons name="qr-code-outline" size={16} color={colors.head} />}
            <Text style={styles.qrBtnText}>Check-in QR</Text>
          </TouchableOpacity>

          {joined && (
            <TouchableOpacity style={styles.qrBtn} onPress={() => router.push("/(main)/nfc" as any)}>
              <Ionicons name="radio-outline" size={16} color={colors.head} />
              <Text style={styles.qrBtnText}>NFC Check-in</Text>
            </TouchableOpacity>
          )}
        </View>

        {/* QR Code */}
        {qrCode && (
          <View style={styles.qrCard}>
            <Text style={styles.qrCardLabel}>CHECK-IN KODU</Text>
            <Text style={styles.qrCodeText}>{qrCode}</Text>
            <Text style={styles.qrCardHint}>Organizatöre gösterin</Text>
          </View>
        )}

        {/* Attendees */}
        {attendees.length > 0 && (
          <>
            <Text style={styles.sectionLabel}>KATILIMCILAR</Text>
            <View style={styles.card}>
              {attendees.map((a, i) => (
                <View
                  key={a.did}
                  style={[styles.attendeeRow, i < attendees.length - 1 && styles.attendeeRowBorder]}
                >
                  <View style={styles.attendeeAvatar}>
                    <Ionicons name="person-outline" size={14} color={colors.accent} />
                  </View>
                  <Text style={styles.attendeeDID} numberOfLines={1}>
                    {deriveOdiFromDid(a.did)}
                  </Text>
                  {a.checked_in && (
                    <View style={styles.checkedInBadge}>
                      <Text style={styles.checkedInText}>✓</Text>
                    </View>
                  )}
                </View>
              ))}
            </View>
          </>
        )}
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
  headerTitle: { fontSize: typography.base, fontWeight: "700", color: colors.head, flex: 1, textAlign: "center" },
  center: { flex: 1, alignItems: "center", justifyContent: "center" },
  emptyText: { fontSize: typography.base, color: colors.sub },
  content: { padding: spacing.lg, gap: spacing.md, paddingBottom: 48 },

  card: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, gap: spacing.sm, overflow: "hidden",
  },
  eventTitle: { fontSize: typography.lg, fontWeight: "700", color: colors.head },
  eventDesc: { fontSize: typography.sm, color: colors.body, lineHeight: 22 },
  metaList: { gap: spacing.sm, marginTop: spacing.xs },
  metaRow: { flexDirection: "row", alignItems: "center", gap: spacing.sm },
  metaText: { fontSize: typography.sm, color: colors.body, flex: 1 },

  actionsRow: { flexDirection: "row", gap: spacing.sm },
  joinBtn: {
    flex: 1, flexDirection: "row", alignItems: "center", justifyContent: "center", gap: spacing.sm,
    backgroundColor: colors.accent, borderRadius: radius.lg, padding: spacing.md,
  },
  joinBtnActive: { backgroundColor: "rgba(239,68,68,0.1)", borderWidth: 1, borderColor: "rgba(239,68,68,0.3)" },
  joinBtnText: { fontSize: typography.base, fontWeight: "700", color: colors.void },
  joinBtnTextActive: { color: colors.red },
  qrBtn: {
    flex: 1, flexDirection: "row", alignItems: "center", justifyContent: "center", gap: spacing.sm,
    backgroundColor: colors.surface, borderRadius: radius.lg, padding: spacing.md,
    borderWidth: 1, borderColor: colors.border,
  },
  qrBtnText: { fontSize: typography.base, fontWeight: "600", color: colors.head },

  qrCard: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: "rgba(74,222,128,0.2)", padding: spacing.md, alignItems: "center", gap: spacing.sm,
  },
  qrCardLabel: { fontSize: typography.xs, color: colors.sub, fontWeight: "600", textTransform: "uppercase", letterSpacing: 1 },
  qrCodeText: {
    fontSize: typography.xs, color: colors.accent, fontFamily: "monospace",
    backgroundColor: colors.raised, borderRadius: radius.md, padding: spacing.sm, width: "100%", textAlign: "center",
  },
  qrCardHint: { fontSize: typography.xs, color: colors.sub },

  sectionLabel: {
    fontSize: typography.xs, color: colors.sub, fontWeight: "600",
    textTransform: "uppercase", letterSpacing: 1,
  },
  attendeeRow: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm, padding: spacing.md,
  },
  attendeeRowBorder: { borderBottomWidth: 1, borderBottomColor: colors.border },
  attendeeAvatar: {
    width: 32, height: 32, borderRadius: radius.full, backgroundColor: "rgba(74,222,128,0.1)",
    alignItems: "center", justifyContent: "center",
  },
  attendeeDID: { flex: 1, fontSize: typography.xs, color: colors.body, fontFamily: "monospace" },
  checkedInBadge: {
    paddingHorizontal: spacing.sm, paddingVertical: 2,
    backgroundColor: "rgba(74,222,128,0.1)", borderRadius: radius.full,
  },
  checkedInText: { fontSize: typography.xs, color: colors.accent },
});
