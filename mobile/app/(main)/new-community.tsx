import React, { useState } from "react";
import {
  View, Text, TextInput, TouchableOpacity, StyleSheet,
  ScrollView, ActivityIndicator,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";

const TOPICS = [
  "Teknoloji", "Kripto", "Oyun", "Müzik", "Spor",
  "Sanat", "Bilim", "Eğitim", "Haber", "Eğlence",
];

export default function NewCommunityScreen() {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [selectedTopics, setSelectedTopics] = useState<string[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  const toggleTopic = (t: string) =>
    setSelectedTopics((prev) =>
      prev.includes(t) ? prev.filter((x) => x !== t) : [...prev, t]
    );

  const submit = async () => {
    if (!name.trim()) { setError("Topluluk adı zorunludur"); return; }
    setError("");
    setSubmitting(true);
    try {
      const res = await api.createConversation({
        type: "community",
        name,
        description,
        topics: selectedTopics,
        participants: [],
      });
      if (res?.conversation?.id) {
        router.replace(`/(main)/chat/${res.conversation.id}`);
      } else {
        router.back();
      }
    } catch {
      setError("Topluluk oluşturulamadı");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Topluluk Oluştur</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        <View style={styles.iconWrap}>
          <Ionicons name="earth-outline" size={32} color={colors.accent} />
        </View>

        <Text style={styles.label}>Topluluk Adı *</Text>
        <TextInput
          style={styles.input}
          value={name}
          onChangeText={setName}
          placeholder="Topluluğun adı"
          placeholderTextColor={colors.dim}
          maxLength={64}
        />

        <Text style={styles.label}>Açıklama</Text>
        <TextInput
          style={[styles.input, styles.textArea]}
          value={description}
          onChangeText={setDescription}
          placeholder="Bu topluluk ne hakkında?"
          placeholderTextColor={colors.dim}
          multiline
          numberOfLines={3}
          textAlignVertical="top"
        />

        <Text style={styles.label}>Konular</Text>
        <View style={styles.topicsWrap}>
          {TOPICS.map((t) => {
            const selected = selectedTopics.includes(t);
            return (
              <TouchableOpacity
                key={t}
                style={[styles.topicChip, selected && styles.topicChipSelected]}
                onPress={() => toggleTopic(t)}
              >
                <Text style={[styles.topicText, selected && styles.topicTextSelected]}>{t}</Text>
              </TouchableOpacity>
            );
          })}
        </View>

        {error ? <Text style={styles.errorText}>{error}</Text> : null}

        <TouchableOpacity
          style={[styles.submitBtn, (submitting || !name.trim()) && styles.submitBtnDim]}
          onPress={submit}
          disabled={submitting || !name.trim()}
        >
          {submitting
            ? <ActivityIndicator size="small" color={colors.void} />
            : <Text style={styles.submitBtnText}>Topluluğu Oluştur</Text>}
        </TouchableOpacity>
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
  content: { padding: spacing.xl, gap: spacing.md },
  iconWrap: {
    width: 80, height: 80, borderRadius: 24,
    backgroundColor: "rgba(74,222,128,0.1)", alignItems: "center", justifyContent: "center",
    alignSelf: "center", marginBottom: spacing.md,
  },
  label: { fontSize: typography.xs, color: colors.sub, fontWeight: "600", textTransform: "uppercase", letterSpacing: 1 },
  input: {
    backgroundColor: colors.surface, borderRadius: radius.lg, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, color: colors.head, fontSize: typography.sm,
  },
  textArea: { minHeight: 80 },
  topicsWrap: { flexDirection: "row", flexWrap: "wrap", gap: spacing.xs },
  topicChip: {
    paddingHorizontal: spacing.md, paddingVertical: spacing.xs,
    borderRadius: radius.full, borderWidth: 1, borderColor: colors.border,
    backgroundColor: colors.surface,
  },
  topicChipSelected: {
    backgroundColor: "rgba(74,222,128,0.1)", borderColor: "rgba(74,222,128,0.3)",
  },
  topicText: { fontSize: typography.xs, color: colors.sub },
  topicTextSelected: { color: colors.accent, fontWeight: "600" },
  errorText: { fontSize: typography.xs, color: colors.red },
  submitBtn: {
    height: 52, borderRadius: radius.xl, backgroundColor: colors.accent,
    alignItems: "center", justifyContent: "center", marginTop: spacing.sm,
  },
  submitBtnDim: { opacity: 0.4 },
  submitBtnText: { fontSize: typography.base, fontWeight: "700", color: colors.void },
});
