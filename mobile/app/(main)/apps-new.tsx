import React, { useState } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, StyleSheet,
  TextInput, ActivityIndicator, Alert,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";

type Category = "tools" | "finance" | "social" | "games";
type Permission = "location" | "messaging" | "wallet" | "zk_proof";
type Step = 1 | 2 | 3;

const CATEGORIES: { value: Category; label: string; icon: string }[] = [
  { value: "tools", label: "Araçlar", icon: "construct-outline" },
  { value: "finance", label: "Finans", icon: "cash-outline" },
  { value: "social", label: "Sosyal", icon: "people-outline" },
  { value: "games", label: "Oyunlar", icon: "game-controller-outline" },
];

const PERMISSIONS: { value: Permission; label: string; desc: string; icon: string }[] = [
  { value: "location", label: "Konum", desc: "GPS konumuna eriş", icon: "location-outline" },
  { value: "messaging", label: "Mesajlaşma", desc: "Mesaj gönder ve al", icon: "chatbubble-outline" },
  { value: "wallet", label: "Cüzdan", desc: "Bakiye sorgula ve transfer başlat", icon: "wallet-outline" },
  { value: "zk_proof", label: "ZK Kanıt", desc: "Sıfır bilgi kanıtı üret", icon: "shield-outline" },
];

export default function AppsNewScreen() {
  const [step, setStep] = useState<Step>(1);
  const [submitting, setSubmitting] = useState(false);

  // Step 1
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [category, setCategory] = useState<Category>("tools");
  const [iconEmoji, setIconEmoji] = useState("");

  // Step 2
  const [codeUrl, setCodeUrl] = useState("");
  const [permissions, setPermissions] = useState<Permission[]>([]);

  // Step 3
  const [webhookUrl, setWebhookUrl] = useState("");

  function togglePermission(p: Permission) {
    setPermissions((prev) =>
      prev.includes(p) ? prev.filter((x) => x !== p) : [...prev, p]
    );
  }

  function canProceed1() { return name.trim().length >= 2 && description.trim().length >= 5; }
  function canProceed2() { return codeUrl.trim().length > 0; }

  async function handleSubmit() {
    setSubmitting(true);
    try {
      await api.publishApp({
        manifest: {
          name,
          description,
          category,
          icon: iconEmoji || undefined,
          permissions,
          webhook_url: webhookUrl || undefined,
        },
        code_url: codeUrl,
      });
      Alert.alert("Başarılı", "Uygulamanız yayınlandı!", [
        { text: "Tamam", onPress: () => router.back() },
      ]);
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "Yayınlama başarısız");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <TouchableOpacity
          onPress={() => { if (step > 1) setStep((s) => (s - 1) as Step); else router.back(); }}
          style={styles.backBtn}
        >
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Yeni Uygulama</Text>
        <View style={{ width: 40 }} />
      </View>

      {/* Step indicator */}
      <View style={styles.stepBar}>
        {[1, 2, 3].map((s) => (
          <View key={s} style={[styles.stepDot, s <= step && styles.stepDotActive]} />
        ))}
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        {step === 1 && (
          <>
            <Text style={styles.stepTitle}>Uygulama Bilgileri</Text>
            <Text style={styles.stepSub}>Mini uygulamanızın temel bilgilerini girin</Text>

            <View style={styles.field}>
              <Text style={styles.label}>Uygulama Adı</Text>
              <TextInput
                style={styles.input}
                placeholder="Uygulamanıza bir ad verin"
                placeholderTextColor={colors.sub}
                value={name}
                onChangeText={setName}
                maxLength={40}
              />
            </View>

            <View style={styles.field}>
              <Text style={styles.label}>Açıklama</Text>
              <TextInput
                style={[styles.input, styles.inputMulti]}
                placeholder="Uygulamanızın ne yaptığını açıklayın"
                placeholderTextColor={colors.sub}
                value={description}
                onChangeText={setDescription}
                multiline
                numberOfLines={3}
                maxLength={300}
              />
            </View>

            <View style={styles.field}>
              <Text style={styles.label}>İkon (Emoji)</Text>
              <TextInput
                style={styles.input}
                placeholder="🚀"
                placeholderTextColor={colors.sub}
                value={iconEmoji}
                onChangeText={setIconEmoji}
                maxLength={2}
              />
            </View>

            <View style={styles.field}>
              <Text style={styles.label}>Kategori</Text>
              <View style={styles.categoryGrid}>
                {CATEGORIES.map((c) => (
                  <TouchableOpacity
                    key={c.value}
                    style={[styles.categoryBtn, category === c.value && styles.categoryBtnActive]}
                    onPress={() => setCategory(c.value)}
                  >
                    <Ionicons
                      name={c.icon as any}
                      size={20}
                      color={category === c.value ? colors.accent : colors.sub}
                    />
                    <Text style={[styles.categoryText, category === c.value && styles.categoryTextActive]}>
                      {c.label}
                    </Text>
                  </TouchableOpacity>
                ))}
              </View>
            </View>
          </>
        )}

        {step === 2 && (
          <>
            <Text style={styles.stepTitle}>Kod & İzinler</Text>
            <Text style={styles.stepSub}>Uygulamanızın kaynak kodunu ve izinleri belirtin</Text>

            <View style={styles.field}>
              <Text style={styles.label}>Kod URL'si</Text>
              <TextInput
                style={styles.input}
                placeholder="https://example.com/app.js"
                placeholderTextColor={colors.sub}
                value={codeUrl}
                onChangeText={setCodeUrl}
                autoCapitalize="none"
                keyboardType="url"
              />
              <Text style={styles.hint}>Uygulamanızın JavaScript dosyasının URL'si</Text>
            </View>

            <View style={styles.field}>
              <Text style={styles.label}>İzinler</Text>
              {PERMISSIONS.map((p) => {
                const active = permissions.includes(p.value);
                return (
                  <TouchableOpacity
                    key={p.value}
                    style={[styles.permRow, active && styles.permRowActive]}
                    onPress={() => togglePermission(p.value)}
                  >
                    <View style={[styles.permIcon, active && styles.permIconActive]}>
                      <Ionicons name={p.icon as any} size={16} color={active ? colors.accent : colors.sub} />
                    </View>
                    <View style={{ flex: 1 }}>
                      <Text style={[styles.permLabel, active && styles.permLabelActive]}>{p.label}</Text>
                      <Text style={styles.permDesc}>{p.desc}</Text>
                    </View>
                    <Ionicons
                      name={active ? "checkmark-circle" : "ellipse-outline"}
                      size={20}
                      color={active ? colors.accent : colors.sub}
                    />
                  </TouchableOpacity>
                );
              })}
            </View>
          </>
        )}

        {step === 3 && (
          <>
            <Text style={styles.stepTitle}>Entegrasyonlar</Text>
            <Text style={styles.stepSub}>Harici webhook ve API bağlantılarını yapılandırın</Text>

            <View style={styles.field}>
              <Text style={styles.label}>Webhook URL (isteğe bağlı)</Text>
              <TextInput
                style={styles.input}
                placeholder="https://yourapi.com/webhook"
                placeholderTextColor={colors.sub}
                value={webhookUrl}
                onChangeText={setWebhookUrl}
                autoCapitalize="none"
                keyboardType="url"
              />
              <Text style={styles.hint}>Kullanıcı etkileşimlerini bu URL'ye bildiririz</Text>
            </View>

            <View style={styles.summaryCard}>
              <Text style={styles.summaryTitle}>Özet</Text>
              <View style={styles.summaryRow}>
                <Text style={styles.summaryKey}>Ad:</Text>
                <Text style={styles.summaryVal}>{name}</Text>
              </View>
              <View style={styles.summaryRow}>
                <Text style={styles.summaryKey}>Kategori:</Text>
                <Text style={styles.summaryVal}>{CATEGORIES.find((c) => c.value === category)?.label}</Text>
              </View>
              <View style={styles.summaryRow}>
                <Text style={styles.summaryKey}>İzinler:</Text>
                <Text style={styles.summaryVal}>
                  {permissions.length === 0 ? "Yok" : permissions.join(", ")}
                </Text>
              </View>
            </View>
          </>
        )}
      </ScrollView>

      {/* Bottom actions */}
      <View style={styles.bottomBar}>
        {step < 3 ? (
          <TouchableOpacity
            style={[
              styles.nextBtn,
              ((step === 1 && !canProceed1()) || (step === 2 && !canProceed2())) && styles.nextBtnDisabled,
            ]}
            onPress={() => setStep((s) => (s + 1) as Step)}
            disabled={(step === 1 && !canProceed1()) || (step === 2 && !canProceed2())}
          >
            <Text style={styles.nextBtnText}>Devam</Text>
            <Ionicons name="chevron-forward" size={18} color={colors.void} />
          </TouchableOpacity>
        ) : (
          <TouchableOpacity
            style={[styles.nextBtn, submitting && styles.nextBtnDisabled]}
            onPress={handleSubmit}
            disabled={submitting}
          >
            {submitting
              ? <ActivityIndicator size="small" color={colors.void} />
              : <Ionicons name="cloud-upload-outline" size={18} color={colors.void} />}
            <Text style={styles.nextBtnText}>Yayınla</Text>
          </TouchableOpacity>
        )}
      </View>
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

  stepBar: {
    flexDirection: "row", gap: spacing.sm, justifyContent: "center",
    paddingVertical: spacing.md,
  },
  stepDot: { width: 32, height: 4, borderRadius: 2, backgroundColor: colors.raised },
  stepDotActive: { backgroundColor: colors.accent },

  content: { padding: spacing.lg, gap: spacing.md, paddingBottom: 100 },
  stepTitle: { fontSize: typography.lg, fontWeight: "700", color: colors.head },
  stepSub: { fontSize: typography.sm, color: colors.sub, marginTop: -spacing.sm },

  field: { gap: spacing.sm },
  label: { fontSize: typography.xs, color: colors.sub, fontWeight: "600", textTransform: "uppercase", letterSpacing: 0.5 },
  input: {
    backgroundColor: colors.surface, borderRadius: radius.lg, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, color: colors.head, fontSize: typography.sm,
  },
  inputMulti: { minHeight: 80, textAlignVertical: "top" },
  hint: { fontSize: typography.xs, color: colors.dim },

  categoryGrid: { flexDirection: "row", flexWrap: "wrap", gap: spacing.sm },
  categoryBtn: {
    flex: 1, minWidth: "44%", backgroundColor: colors.surface, borderRadius: radius.lg,
    borderWidth: 1, borderColor: colors.border, padding: spacing.md,
    alignItems: "center", gap: spacing.xs,
  },
  categoryBtnActive: { borderColor: "rgba(74,222,128,0.4)", backgroundColor: "rgba(74,222,128,0.06)" },
  categoryText: { fontSize: typography.sm, fontWeight: "600", color: colors.sub },
  categoryTextActive: { color: colors.accent },

  permRow: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm,
    backgroundColor: colors.surface, borderRadius: radius.lg, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md,
  },
  permRowActive: { borderColor: "rgba(74,222,128,0.3)", backgroundColor: "rgba(74,222,128,0.04)" },
  permIcon: {
    width: 36, height: 36, borderRadius: radius.md, backgroundColor: colors.raised,
    alignItems: "center", justifyContent: "center",
  },
  permIconActive: { backgroundColor: "rgba(74,222,128,0.1)" },
  permLabel: { fontSize: typography.sm, fontWeight: "600", color: colors.sub },
  permLabelActive: { color: colors.head },
  permDesc: { fontSize: typography.xs, color: colors.dim },

  summaryCard: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, gap: spacing.sm,
  },
  summaryTitle: { fontSize: typography.sm, fontWeight: "700", color: colors.head, marginBottom: spacing.xs },
  summaryRow: { flexDirection: "row", gap: spacing.sm },
  summaryKey: { fontSize: typography.sm, color: colors.sub, width: 80 },
  summaryVal: { fontSize: typography.sm, color: colors.head, flex: 1 },

  bottomBar: {
    position: "absolute", bottom: 0, left: 0, right: 0,
    padding: spacing.lg, paddingBottom: spacing.xl,
    backgroundColor: colors.void, borderTopWidth: 1, borderTopColor: colors.border,
  },
  nextBtn: {
    flexDirection: "row", alignItems: "center", justifyContent: "center", gap: spacing.sm,
    backgroundColor: colors.accent, borderRadius: radius.lg, padding: spacing.md,
  },
  nextBtnDisabled: { opacity: 0.4 },
  nextBtnText: { fontSize: typography.base, fontWeight: "700", color: colors.void },
});
