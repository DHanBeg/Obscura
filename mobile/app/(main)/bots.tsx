import React, { useState, useEffect, useCallback } from "react";
import {
  View, Text, FlatList, TouchableOpacity, StyleSheet,
  ActivityIndicator, Modal, TextInput, Alert, Clipboard,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";

interface Bot {
  id: string;
  name: string;
  description: string;
  webhook_url: string;
  token?: string;
  created_at: string;
}

type Phase = "list" | "new" | "success";

export default function BotsScreen() {
  const [bots, setBots] = useState<Bot[]>([]);
  const [loading, setLoading] = useState(true);
  const [phase, setPhase] = useState<Phase>("list");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [webhookURL, setWebhookURL] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [createdToken, setCreatedToken] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.listMyBots();
      setBots(res?.bots || []);
    } catch {}
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const submit = async () => {
    if (!name.trim() || !webhookURL.trim()) {
      setError("Bot adı ve webhook URL zorunludur");
      return;
    }
    setError("");
    setSubmitting(true);
    try {
      const result = await api.createBot({ name, description, webhook_url: webhookURL });
      setCreatedToken(result?.token ?? "");
      setPhase("success");
      await load();
    } catch {
      setError("Bot oluşturulamadı");
    } finally {
      setSubmitting(false);
    }
  };

  const deleteBot = (bot: Bot) => {
    Alert.alert("Botu sil", `"${bot.name}" silinecek. Emin misiniz?`, [
      { text: "İptal", style: "cancel" },
      {
        text: "Sil", style: "destructive",
        onPress: async () => {
          try {
            await api.deleteBot(bot.id);
            await load();
          } catch {
            Alert.alert("Hata", "Bot silinemedi");
          }
        },
      },
    ]);
  };

  const copyToken = (token: string) => {
    Clipboard.setString(token);
    Alert.alert("Kopyalandı", "Token panoya kopyalandı.");
  };

  const resetForm = () => {
    setName(""); setDescription(""); setWebhookURL("");
    setError(""); setCreatedToken("");
    setPhase("list");
  };

  const renderBot = ({ item }: { item: Bot }) => (
    <View style={styles.card}>
      <View style={styles.cardHeader}>
        <View style={styles.botIcon}>
          <Ionicons name="hardware-chip-outline" size={20} color={colors.accent} />
        </View>
        <View style={{ flex: 1 }}>
          <Text style={styles.cardTitle}>{item.name}</Text>
          {item.description ? (
            <Text style={styles.cardSub} numberOfLines={1}>{item.description}</Text>
          ) : null}
        </View>
        <TouchableOpacity onPress={() => deleteBot(item)} style={styles.deleteBtn}>
          <Ionicons name="trash-outline" size={18} color={colors.red} />
        </TouchableOpacity>
      </View>
      <Text style={styles.webhookText} numberOfLines={1}>{item.webhook_url}</Text>
    </View>
  );

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      {/* Header */}
      <View style={styles.header}>
        <Text style={styles.headerTitle}>Botlarım</Text>
        <TouchableOpacity style={styles.addBtn} onPress={() => setPhase("new")}>
          <Ionicons name="add" size={22} color={colors.accent} />
        </TouchableOpacity>
      </View>

      {/* List */}
      {loading ? (
        <View style={styles.center}>
          <ActivityIndicator color={colors.accent} />
        </View>
      ) : (
        <FlatList
          data={bots}
          keyExtractor={(i) => i.id}
          renderItem={renderBot}
          contentContainerStyle={styles.list}
          ListEmptyComponent={
            <View style={styles.center}>
              <Ionicons name="hardware-chip-outline" size={40} color={colors.muted} />
              <Text style={styles.emptyText}>Henüz bot yok</Text>
              <TouchableOpacity style={styles.createFirstBtn} onPress={() => setPhase("new")}>
                <Text style={styles.createFirstText}>İlk botu oluştur</Text>
              </TouchableOpacity>
            </View>
          }
        />
      )}

      {/* New Bot Modal */}
      <Modal visible={phase === "new"} transparent animationType="slide">
        <View style={styles.overlay}>
          <View style={styles.sheet}>
            <View style={styles.sheetHeader}>
              <Text style={styles.sheetTitle}>Yeni Bot</Text>
              <TouchableOpacity onPress={resetForm}>
                <Ionicons name="close" size={22} color={colors.sub} />
              </TouchableOpacity>
            </View>

            <Text style={styles.label}>Bot Adı *</Text>
            <TextInput
              style={styles.input}
              value={name}
              onChangeText={setName}
              placeholder="Bildirim Botu"
              placeholderTextColor={colors.dim}
              autoCapitalize="words"
            />

            <Text style={styles.label}>Açıklama</Text>
            <TextInput
              style={styles.input}
              value={description}
              onChangeText={setDescription}
              placeholder="Ne yapar?"
              placeholderTextColor={colors.dim}
            />

            <Text style={styles.label}>Webhook URL *</Text>
            <TextInput
              style={styles.input}
              value={webhookURL}
              onChangeText={setWebhookURL}
              placeholder="https://..."
              placeholderTextColor={colors.dim}
              autoCapitalize="none"
              keyboardType="url"
            />

            {error ? <Text style={styles.errorText}>{error}</Text> : null}

            <TouchableOpacity
              style={[styles.submitBtn, submitting && styles.submitBtnDim]}
              onPress={submit}
              disabled={submitting}
            >
              {submitting
                ? <ActivityIndicator size="small" color={colors.void} />
                : <Text style={styles.submitBtnText}>Oluştur</Text>}
            </TouchableOpacity>
          </View>
        </View>
      </Modal>

      {/* Success Modal */}
      <Modal visible={phase === "success"} transparent animationType="fade">
        <View style={styles.overlay}>
          <View style={styles.sheet}>
            <View style={styles.successIcon}>
              <Ionicons name="hardware-chip-outline" size={32} color={colors.accent} />
            </View>
            <Text style={styles.sheetTitle}>Bot Hazır!</Text>
            <Text style={styles.sheetSub}>Token'ı güvenli bir yerde sakla.</Text>

            <View style={styles.tokenBox}>
              <Text style={styles.tokenText} numberOfLines={2} selectable>
                {createdToken}
              </Text>
              <TouchableOpacity onPress={() => copyToken(createdToken)} style={styles.copyBtn}>
                <Ionicons name="copy-outline" size={18} color={colors.accent} />
              </TouchableOpacity>
            </View>

            <TouchableOpacity style={styles.submitBtn} onPress={resetForm}>
              <Text style={styles.submitBtnText}>Tamam</Text>
            </TouchableOpacity>
          </View>
        </View>
      </Modal>
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
  headerTitle: { fontSize: typography.lg, fontWeight: "700", color: colors.head },
  addBtn: { padding: 4 },
  list: { padding: spacing.md, gap: spacing.sm },
  center: { flex: 1, alignItems: "center", justifyContent: "center", padding: 40, gap: 12 },
  emptyText: { fontSize: typography.sm, color: colors.sub },
  createFirstBtn: {
    paddingHorizontal: spacing.lg, paddingVertical: spacing.sm,
    backgroundColor: "rgba(74,222,128,0.1)", borderRadius: radius.lg,
    borderWidth: 1, borderColor: "rgba(74,222,128,0.2)",
  },
  createFirstText: { fontSize: typography.sm, color: colors.accent, fontWeight: "600" },
  card: {
    backgroundColor: colors.surface, borderRadius: radius.xl, padding: spacing.md,
    borderWidth: 1, borderColor: colors.border, gap: spacing.xs,
  },
  cardHeader: { flexDirection: "row", alignItems: "center", gap: spacing.sm },
  botIcon: {
    width: 40, height: 40, borderRadius: radius.lg,
    backgroundColor: "rgba(74,222,128,0.08)", alignItems: "center", justifyContent: "center",
  },
  cardTitle: { fontSize: typography.base, fontWeight: "600", color: colors.head },
  cardSub: { fontSize: typography.xs, color: colors.sub, marginTop: 2 },
  deleteBtn: { padding: 4 },
  webhookText: { fontSize: typography.xs, color: colors.dim, fontFamily: "monospace" },
  overlay: { flex: 1, backgroundColor: "rgba(0,0,0,0.7)", justifyContent: "flex-end" },
  sheet: {
    backgroundColor: colors.ground, borderTopLeftRadius: radius.xxl,
    borderTopRightRadius: radius.xxl, padding: spacing.xl, gap: spacing.md,
  },
  sheetHeader: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  sheetTitle: { fontSize: typography.md, fontWeight: "700", color: colors.head },
  sheetSub: { fontSize: typography.sm, color: colors.sub, textAlign: "center" },
  label: { fontSize: typography.xs, color: colors.sub, fontWeight: "600", textTransform: "uppercase", letterSpacing: 1 },
  input: {
    backgroundColor: colors.surface, borderRadius: radius.lg, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, color: colors.head,
    fontSize: typography.sm,
  },
  errorText: { fontSize: typography.xs, color: colors.red },
  submitBtn: {
    height: 50, borderRadius: radius.lg, backgroundColor: colors.accent,
    alignItems: "center", justifyContent: "center",
  },
  submitBtnDim: { opacity: 0.6 },
  submitBtnText: { fontSize: typography.base, fontWeight: "700", color: colors.void },
  successIcon: {
    width: 64, height: 64, borderRadius: 20, backgroundColor: "rgba(74,222,128,0.12)",
    alignItems: "center", justifyContent: "center", alignSelf: "center",
  },
  tokenBox: {
    flexDirection: "row", alignItems: "center", backgroundColor: colors.surface,
    borderRadius: radius.lg, borderWidth: 1, borderColor: colors.border,
    padding: spacing.md, gap: spacing.sm,
  },
  tokenText: { flex: 1, fontSize: typography.xs, color: colors.body, fontFamily: "monospace" },
  copyBtn: { padding: 4 },
});
