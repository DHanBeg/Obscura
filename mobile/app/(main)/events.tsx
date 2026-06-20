import { useState, useEffect, useCallback } from "react";
import {
  View, Text, FlatList, TouchableOpacity, StyleSheet,
  RefreshControl, ActivityIndicator, Modal, TextInput, ScrollView, Alert,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useRouter } from "expo-router";
import { api } from "../../lib/api";
import { colors, radius, spacing, typography } from "../../lib/theme";

interface Event {
  id: string;
  title: string;
  description?: string;
  location_name?: string;
  starts_at: string;
  ends_at?: string;
  capacity?: number;
  attendee_count?: number;
}

export default function EventsScreen() {
  const router = useRouter();
  const [events, setEvents] = useState<Event[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [creating, setCreating] = useState(false);
  const [form, setForm] = useState({ title: "", description: "", location: "", starts_at: "", capacity: "" });

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true);
    try {
      const res = await api.listEvents();
      setEvents(Array.isArray(res) ? res : res?.events ?? []);
    } catch { /* ignore */ }
    finally { setLoading(false); setRefreshing(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  async function handleCreate() {
    if (!form.title.trim() || !form.starts_at.trim()) {
      Alert.alert("Hata", "Başlık ve başlangıç tarihi zorunlu");
      return;
    }
    setCreating(true);
    try {
      await api.createEvent({
        title: form.title,
        description: form.description || undefined,
        location: form.location || undefined,
        starts_at: new Date(form.starts_at).toISOString(),
        ends_at: new Date(form.starts_at).toISOString(),
        capacity: form.capacity ? Number(form.capacity) : undefined,
      });
      setShowCreate(false);
      setForm({ title: "", description: "", location: "", starts_at: "", capacity: "" });
      load();
    } catch { Alert.alert("Hata", "Etkinlik oluşturulamadı"); }
    finally { setCreating(false); }
  }

  const renderItem = ({ item }: { item: Event }) => (
    <TouchableOpacity style={s.card} onPress={() => router.push(`/events/${item.id}` as never)}>
      <Text style={s.cardTitle} numberOfLines={1}>{item.title}</Text>
      {item.location_name ? <Text style={s.cardMeta} numberOfLines={1}>📍 {item.location_name}</Text> : null}
      <View style={s.cardRow}>
        <Text style={s.cardMeta}>
          🕐 {new Date(item.starts_at).toLocaleDateString("tr-TR")}
        </Text>
        {item.attendee_count !== undefined && (
          <Text style={s.cardMeta}>
            👥 {item.attendee_count}{item.capacity ? `/${item.capacity}` : ""}
          </Text>
        )}
      </View>
    </TouchableOpacity>
  );

  return (
    <SafeAreaView style={s.container}>
      <View style={s.header}>
        <Text style={s.title}>Etkinlikler</Text>
        <TouchableOpacity style={s.addBtn} onPress={() => setShowCreate(true)}>
          <Text style={s.addBtnText}>+ Oluştur</Text>
        </TouchableOpacity>
      </View>

      {loading ? (
        <View style={s.center}>
          <ActivityIndicator color={colors.accent} />
        </View>
      ) : (
        <FlatList
          data={events}
          keyExtractor={(i) => i.id}
          renderItem={renderItem}
          contentContainerStyle={s.list}
          refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => load(true)} tintColor={colors.accent} />}
          ListEmptyComponent={
            <View style={s.center}>
              <Text style={s.empty}>Henüz etkinlik yok</Text>
            </View>
          }
        />
      )}

      <Modal visible={showCreate} animationType="slide" presentationStyle="pageSheet"
        onRequestClose={() => setShowCreate(false)}>
        <SafeAreaView style={s.modal}>
          <View style={s.modalHeader}>
            <TouchableOpacity onPress={() => setShowCreate(false)}>
              <Text style={s.cancel}>İptal</Text>
            </TouchableOpacity>
            <Text style={s.modalTitle}>Etkinlik Oluştur</Text>
            <TouchableOpacity onPress={handleCreate} disabled={creating}>
              {creating ? <ActivityIndicator color={colors.accent} /> : <Text style={s.save}>Kaydet</Text>}
            </TouchableOpacity>
          </View>
          <ScrollView style={s.modalBody}>
            <Text style={s.label}>Başlık *</Text>
            <TextInput style={s.input} placeholder="Etkinlik adı" placeholderTextColor={colors.text3}
              value={form.title} onChangeText={(v) => setForm({ ...form, title: v })} />
            <Text style={s.label}>Açıklama</Text>
            <TextInput style={[s.input, s.textarea]} placeholder="Açıklama" placeholderTextColor={colors.text3}
              multiline value={form.description} onChangeText={(v) => setForm({ ...form, description: v })} />
            <Text style={s.label}>Konum</Text>
            <TextInput style={s.input} placeholder="Yer" placeholderTextColor={colors.text3}
              value={form.location} onChangeText={(v) => setForm({ ...form, location: v })} />
            <Text style={s.label}>Başlangıç Tarihi *</Text>
            <TextInput style={s.input} placeholder="YYYY-MM-DD HH:MM" placeholderTextColor={colors.text3}
              value={form.starts_at} onChangeText={(v) => setForm({ ...form, starts_at: v })} />
            <Text style={s.label}>Kapasite</Text>
            <TextInput style={s.input} placeholder="Maksimum katılımcı" placeholderTextColor={colors.text3}
              keyboardType="number-pad" value={form.capacity} onChangeText={(v) => setForm({ ...form, capacity: v })} />
          </ScrollView>
        </SafeAreaView>
      </Modal>
    </SafeAreaView>
  );
}

const s = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.bg },
  header: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingHorizontal: spacing.md, paddingVertical: spacing.sm },
  title: { ...typography.title, color: colors.text1 },
  addBtn: { backgroundColor: colors.accent, borderRadius: radius.md, paddingHorizontal: 12, paddingVertical: 6 },
  addBtnText: { color: "#000", fontSize: 13, fontWeight: "700" },
  center: { flex: 1, alignItems: "center", justifyContent: "center", paddingTop: 80 },
  list: { padding: spacing.md, gap: 10 },
  card: { backgroundColor: colors.surface2, borderRadius: radius.lg, padding: spacing.md, gap: 4 },
  cardTitle: { fontSize: 15, fontWeight: "600", color: colors.text1 },
  cardMeta: { fontSize: 12, color: colors.text3 },
  cardRow: { flexDirection: "row", gap: 12, marginTop: 2 },
  empty: { color: colors.text3, fontSize: 14 },
  modal: { flex: 1, backgroundColor: colors.bg },
  modalHeader: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingHorizontal: spacing.md, paddingVertical: spacing.sm, borderBottomWidth: 1, borderBottomColor: colors.border1 },
  modalTitle: { fontSize: 16, fontWeight: "700", color: colors.text1 },
  cancel: { fontSize: 15, color: colors.text3 },
  save: { fontSize: 15, color: colors.accent, fontWeight: "700" },
  modalBody: { padding: spacing.md },
  label: { fontSize: 11, color: colors.text3, fontWeight: "600", textTransform: "uppercase", marginBottom: 4, marginTop: 12 },
  input: { backgroundColor: colors.surface3, borderRadius: radius.md, padding: 12, color: colors.text1, fontSize: 14 },
  textarea: { height: 80, textAlignVertical: "top" },
});
