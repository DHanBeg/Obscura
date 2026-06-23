import React, { useState } from "react";
import {
  View, Text, TextInput, TouchableOpacity, StyleSheet,
  Switch, ActivityIndicator, ScrollView,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";

export default function NewChannelScreen() {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [isPublic, setIsPublic] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  const submit = async () => {
    if (!name.trim()) { setError("Kanal adı zorunludur"); return; }
    setError("");
    setSubmitting(true);
    try {
      const res = await api.createConversation({
        type: "channel",
        name,
        description,
        is_public: isPublic,
        participants: [],
      });
      if (res?.conversation?.id) {
        router.replace(`/(main)/chat/${res.conversation.id}`);
      } else {
        router.back();
      }
    } catch {
      setError("Kanal oluşturulamadı");
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
        <Text style={styles.headerTitle}>Kanal Oluştur</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        <View style={styles.iconWrap}>
          <Ionicons name="megaphone-outline" size={32} color={colors.accent} />
        </View>

        <Text style={styles.label}>Kanal Adı *</Text>
        <TextInput
          style={styles.input}
          value={name}
          onChangeText={setName}
          placeholder="kanal-adı"
          placeholderTextColor={colors.dim}
          autoCapitalize="none"
          maxLength={64}
        />

        <Text style={styles.label}>Açıklama</Text>
        <TextInput
          style={[styles.input, styles.textArea]}
          value={description}
          onChangeText={setDescription}
          placeholder="Bu kanal ne hakkında?"
          placeholderTextColor={colors.dim}
          multiline
          numberOfLines={3}
          textAlignVertical="top"
        />

        <View style={styles.switchRow}>
          <View style={{ flex: 1 }}>
            <Text style={styles.switchLabel}>Herkese Açık</Text>
            <Text style={styles.switchSub}>
              {isPublic ? "Herkes kanalı keşfedebilir ve katılabilir" : "Yalnızca davet ile katılınabilir"}
            </Text>
          </View>
          <Switch
            value={isPublic}
            onValueChange={setIsPublic}
            trackColor={{ false: colors.raised, true: "rgba(74,222,128,0.4)" }}
            thumbColor={isPublic ? colors.accent : colors.sub}
          />
        </View>

        {error ? <Text style={styles.errorText}>{error}</Text> : null}

        <TouchableOpacity
          style={[styles.submitBtn, (submitting || !name.trim()) && styles.submitBtnDim]}
          onPress={submit}
          disabled={submitting || !name.trim()}
        >
          {submitting
            ? <ActivityIndicator size="small" color={colors.void} />
            : <Text style={styles.submitBtnText}>Kanalı Oluştur</Text>}
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
  switchRow: {
    flexDirection: "row", alignItems: "center", gap: spacing.md,
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md,
  },
  switchLabel: { fontSize: typography.sm, fontWeight: "600", color: colors.head, marginBottom: 2 },
  switchSub: { fontSize: typography.xs, color: colors.sub, lineHeight: 16 },
  errorText: { fontSize: typography.xs, color: colors.red },
  submitBtn: {
    height: 52, borderRadius: radius.xl, backgroundColor: colors.accent,
    alignItems: "center", justifyContent: "center", marginTop: spacing.sm,
  },
  submitBtnDim: { opacity: 0.4 },
  submitBtnText: { fontSize: typography.base, fontWeight: "700", color: colors.void },
});
