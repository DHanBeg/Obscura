import React, { useState } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, StyleSheet,
  TextInput, ActivityIndicator, Alert,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import * as Clipboard from "expo-clipboard";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";

type Phase = "form" | "success";

export default function BotsNewScreen() {
  const [phase, setPhase] = useState<Phase>("form");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [webhookURL, setWebhookURL] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [createdToken, setCreatedToken] = useState("");
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);

  const handleSubmit = async () => {
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
    } catch (e: any) {
      setError(e?.message || "Bot oluşturulamadı");
    } finally {
      setSubmitting(false);
    }
  };

  const handleCopyToken = async () => {
    if (!createdToken) return;
    await Clipboard.setStringAsync(createdToken);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  if (phase === "success") {
    return (
      <SafeAreaView style={styles.root} edges={["top"]}>
        <View style={styles.header}>
          <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
            <Ionicons name="chevron-back" size={24} color={colors.head} />
          </TouchableOpacity>
          <Text style={styles.headerTitle}>Bot Oluşturuldu</Text>
          <View style={{ width: 40 }} />
        </View>

        <View style={styles.successContent}>
          <View style={styles.successIcon}>
            <Ionicons name="checkmark-circle" size={48} color={colors.accent} />
          </View>
          <Text style={styles.successTitle}>{name}</Text>
          <Text style={styles.successSub}>Bot başarıyla oluşturuldu</Text>

          <View style={styles.tokenCard}>
            <View style={styles.tokenCardHeader}>
              <Text style={styles.tokenLabel}>BOT TOKEN</Text>
              <TouchableOpacity onPress={handleCopyToken} style={styles.copyBtn}>
                <Ionicons
                  name={copied ? "checkmark" : "copy-outline"}
                  size={16}
                  color={copied ? colors.accent : colors.sub}
                />
                <Text style={[styles.copyBtnText, copied && { color: colors.accent }]}>
                  {copied ? "Kopyalandı" : "Kopyala"}
                </Text>
              </TouchableOpacity>
            </View>
            <Text style={styles.tokenValue} numberOfLines={3}>
              {createdToken || "—"}
            </Text>
            <View style={styles.tokenWarning}>
              <Ionicons name="warning-outline" size={14} color={colors.amber} />
              <Text style={styles.tokenWarningText}>
                Bu token bir daha gösterilmeyecek. Güvenli bir yerde saklayın.
              </Text>
            </View>
          </View>

          <TouchableOpacity style={styles.doneBtn} onPress={() => router.back()}>
            <Text style={styles.doneBtnText}>Tamam</Text>
          </TouchableOpacity>
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
        <Text style={styles.headerTitle}>Yeni Bot</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        <View style={styles.infoBanner}>
          <Ionicons name="hardware-chip-outline" size={18} color={colors.tier3} />
          <Text style={styles.infoText}>
            Botlar, webhook üzerinden mesajlaşma olaylarını alır ve otomatik yanıtlar gönderir.
          </Text>
        </View>

        <View style={styles.field}>
          <Text style={styles.label}>Bot Adı</Text>
          <TextInput
            style={styles.input}
            placeholder="Botunuza bir ad verin"
            placeholderTextColor={colors.sub}
            value={name}
            onChangeText={setName}
            maxLength={50}
          />
        </View>

        <View style={styles.field}>
          <Text style={styles.label}>Açıklama (isteğe bağlı)</Text>
          <TextInput
            style={[styles.input, styles.inputMulti]}
            placeholder="Botunuzun ne yaptığını açıklayın"
            placeholderTextColor={colors.sub}
            value={description}
            onChangeText={setDescription}
            multiline
            numberOfLines={3}
            maxLength={200}
          />
        </View>

        <View style={styles.field}>
          <Text style={styles.label}>Webhook URL</Text>
          <TextInput
            style={styles.input}
            placeholder="https://yourserver.com/webhook"
            placeholderTextColor={colors.sub}
            value={webhookURL}
            onChangeText={setWebhookURL}
            autoCapitalize="none"
            keyboardType="url"
          />
          <Text style={styles.hint}>Mesajlaşma olayları bu URL'ye POST edilir</Text>
        </View>

        {error ? (
          <View style={styles.errorBox}>
            <Ionicons name="alert-circle-outline" size={16} color={colors.red} />
            <Text style={styles.errorText}>{error}</Text>
          </View>
        ) : null}

        <TouchableOpacity
          style={[styles.submitBtn, (submitting || !name.trim() || !webhookURL.trim()) && styles.submitBtnDisabled]}
          onPress={handleSubmit}
          disabled={submitting || !name.trim() || !webhookURL.trim()}
        >
          {submitting
            ? <ActivityIndicator size="small" color={colors.void} />
            : <Ionicons name="add-circle-outline" size={18} color={colors.void} />}
          <Text style={styles.submitBtnText}>Bot Oluştur</Text>
        </TouchableOpacity>

        {/* Info */}
        <View style={styles.docsCard}>
          <Text style={styles.docsTitle}>Webhook Payload Formatı</Text>
          <Text style={styles.docsCode}>{`POST /your-webhook\n{\n  "event": "message",\n  "from_did": "did:obs:...",\n  "text": "Merhaba bot!",\n  "conv_id": "uuid"\n}`}</Text>
        </View>
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
  content: { padding: spacing.lg, gap: spacing.md, paddingBottom: 48 },

  infoBanner: {
    flexDirection: "row", gap: spacing.sm, alignItems: "flex-start",
    backgroundColor: "rgba(167,139,250,0.08)", borderRadius: radius.lg,
    borderWidth: 1, borderColor: "rgba(167,139,250,0.2)", padding: spacing.md,
  },
  infoText: { flex: 1, fontSize: typography.sm, color: colors.body, lineHeight: 20 },

  field: { gap: spacing.xs },
  label: { fontSize: typography.xs, color: colors.sub, fontWeight: "600", textTransform: "uppercase", letterSpacing: 0.5 },
  input: {
    backgroundColor: colors.surface, borderRadius: radius.lg, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, color: colors.head, fontSize: typography.sm,
  },
  inputMulti: { minHeight: 80, textAlignVertical: "top" },
  hint: { fontSize: typography.xs, color: colors.dim },

  errorBox: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm,
    backgroundColor: "rgba(239,68,68,0.08)", borderRadius: radius.lg,
    borderWidth: 1, borderColor: "rgba(239,68,68,0.2)", padding: spacing.md,
  },
  errorText: { fontSize: typography.sm, color: colors.red, flex: 1 },

  submitBtn: {
    flexDirection: "row", alignItems: "center", justifyContent: "center", gap: spacing.sm,
    backgroundColor: colors.accent, borderRadius: radius.lg, padding: spacing.md,
  },
  submitBtnDisabled: { opacity: 0.4 },
  submitBtnText: { fontSize: typography.base, fontWeight: "700", color: colors.void },

  docsCard: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, gap: spacing.sm,
  },
  docsTitle: { fontSize: typography.sm, fontWeight: "600", color: colors.head },
  docsCode: {
    fontSize: typography.xs, color: colors.body, fontFamily: "monospace",
    backgroundColor: colors.raised, borderRadius: radius.md, padding: spacing.sm, lineHeight: 18,
  },

  // Success
  successContent: { flex: 1, padding: spacing.lg, gap: spacing.md, alignItems: "center", justifyContent: "center" },
  successIcon: {
    width: 80, height: 80, borderRadius: radius.full, backgroundColor: "rgba(74,222,128,0.08)",
    borderWidth: 2, borderColor: "rgba(74,222,128,0.3)", alignItems: "center", justifyContent: "center",
  },
  successTitle: { fontSize: typography.xl, fontWeight: "700", color: colors.head },
  successSub: { fontSize: typography.sm, color: colors.sub },

  tokenCard: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, gap: spacing.sm, width: "100%",
  },
  tokenCardHeader: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  tokenLabel: { fontSize: typography.xs, color: colors.sub, fontWeight: "600", textTransform: "uppercase", letterSpacing: 1 },
  copyBtn: { flexDirection: "row", alignItems: "center", gap: spacing.xs },
  copyBtnText: { fontSize: typography.xs, color: colors.sub },
  tokenValue: {
    fontSize: typography.xs, color: colors.head, fontFamily: "monospace",
    backgroundColor: colors.raised, borderRadius: radius.md, padding: spacing.sm, lineHeight: 18,
  },
  tokenWarning: {
    flexDirection: "row", alignItems: "flex-start", gap: spacing.xs,
    backgroundColor: "rgba(245,158,11,0.06)", borderRadius: radius.md,
    borderWidth: 1, borderColor: "rgba(245,158,11,0.15)", padding: spacing.sm,
  },
  tokenWarningText: { fontSize: typography.xs, color: colors.body, flex: 1, lineHeight: 16 },

  doneBtn: {
    backgroundColor: colors.accent, borderRadius: radius.lg, paddingHorizontal: spacing.xxl,
    paddingVertical: spacing.md, alignSelf: "stretch",
  },
  doneBtnText: { fontSize: typography.base, fontWeight: "700", color: colors.void, textAlign: "center" },
});
