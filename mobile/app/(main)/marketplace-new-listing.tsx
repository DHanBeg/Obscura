import React, { useCallback, useState } from "react";
import { View, Text, StyleSheet, TextInput, TouchableOpacity, ScrollView, ActivityIndicator, Alert } from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { mpColors, mpSpacing, mpRadius, mpTypography } from "@/lib/marketplace-theme";
import { createListing } from "@/lib/marketplace-api";

const CATEGORIES = ["goods", "services", "digital", "misc"] as const;
const CATEGORY_LABELS: Record<string, string> = {
  goods: "Ürün", services: "Hizmet", digital: "Dijital", misc: "Diğer",
};

export default function MarketplaceNewListingScreen() {
  const insets = useSafeAreaInsets();
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [price, setPrice] = useState("");
  const [category, setCategory] = useState<string>("goods");
  const [submitting, setSubmitting] = useState(false);

  const canSubmit = title.trim().length > 0 && description.trim().length > 0 && /^\d+(\.\d+)?$/.test(price.trim());

  const handleSubmit = useCallback(async () => {
    if (!canSubmit) return;
    setSubmitting(true);
    try {
      // price backend'e obs_token'ın en küçük birimi (18 ondalık) string
      // olarak gidiyor — kullanıcı tam OBS giriyor (wallet.tsx'in transfer
      // modalıyla aynı ölçek varsayımı).
      const priceSmallestUnit = (BigInt(Math.round(parseFloat(price) * 1e6)) * 1000000000000n).toString();
      const res = await createListing(title.trim(), description.trim(), priceSmallestUnit, category);
      Alert.alert("İlan oluşturuldu", "", [
        { text: "Tamam", onPress: () => router.replace(`/(main)/marketplace/${res.listing_id}` as any) },
      ]);
    } catch (e: any) {
      Alert.alert("Hata", e?.message || "İlan oluşturulamadı.");
    } finally {
      setSubmitting(false);
    }
  }, [canSubmit, title, description, price, category]);

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={mpColors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Yeni İlan</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.scroll}>
        <Text style={styles.label}>Başlık</Text>
        <TextInput
          style={styles.input}
          value={title}
          onChangeText={setTitle}
          placeholder="Ne satıyorsunuz?"
          placeholderTextColor={mpColors.dim}
        />

        <Text style={styles.label}>Açıklama</Text>
        <TextInput
          style={[styles.input, styles.inputMultiline]}
          value={description}
          onChangeText={setDescription}
          placeholder="Detayları yazın..."
          placeholderTextColor={mpColors.dim}
          multiline
        />

        <Text style={styles.label}>Fiyat (OBS)</Text>
        <TextInput
          style={styles.input}
          value={price}
          onChangeText={setPrice}
          placeholder="0"
          placeholderTextColor={mpColors.dim}
          keyboardType="decimal-pad"
        />

        <Text style={styles.label}>Kategori</Text>
        <View style={styles.categoryRow}>
          {CATEGORIES.map((c) => (
            <TouchableOpacity
              key={c}
              style={[styles.categoryChip, category === c && styles.categoryChipActive]}
              onPress={() => setCategory(c)}
            >
              <Text style={[styles.categoryChipText, category === c && styles.categoryChipTextActive]}>
                {CATEGORY_LABELS[c]}
              </Text>
            </TouchableOpacity>
          ))}
        </View>
      </ScrollView>

      <View style={[styles.footer, { paddingBottom: insets.bottom + mpSpacing.sm }]}>
        <TouchableOpacity
          style={[styles.submitBtn, !canSubmit && styles.submitBtnDisabled]}
          onPress={handleSubmit}
          disabled={!canSubmit || submitting}
        >
          {submitting ? <ActivityIndicator color={mpColors.void} /> : <Text style={styles.submitBtnText}>İlanı Yayınla</Text>}
        </TouchableOpacity>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: mpColors.void },
  header: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    paddingHorizontal: mpSpacing.lg, paddingVertical: mpSpacing.md,
    borderBottomWidth: 1, borderBottomColor: mpColors.border,
  },
  backBtn: { padding: 4, width: 40 },
  headerTitle: { fontSize: mpTypography.base, fontWeight: "700", color: mpColors.head },

  scroll: { padding: mpSpacing.lg, paddingBottom: mpSpacing.xxl },
  label: { fontSize: mpTypography.xs, color: mpColors.dim, fontWeight: "600", marginTop: mpSpacing.md, marginBottom: mpSpacing.xs },
  input: {
    backgroundColor: mpColors.surface, borderWidth: 1, borderColor: mpColors.border,
    borderRadius: mpRadius.lg, paddingHorizontal: mpSpacing.md, height: 48,
    color: mpColors.head, fontSize: mpTypography.base,
  },
  inputMultiline: { height: 100, paddingTop: mpSpacing.sm, textAlignVertical: "top" },

  categoryRow: { flexDirection: "row", gap: mpSpacing.xs, flexWrap: "wrap" },
  categoryChip: {
    paddingHorizontal: mpSpacing.md, paddingVertical: mpSpacing.sm,
    borderRadius: mpRadius.full, borderWidth: 1, borderColor: mpColors.border,
    backgroundColor: mpColors.surface,
  },
  categoryChipActive: { backgroundColor: mpColors.accentDeep, borderColor: mpColors.accentDim },
  categoryChipText: { fontSize: mpTypography.sm, color: mpColors.dim, fontWeight: "600" },
  categoryChipTextActive: { color: mpColors.accent },

  footer: { padding: mpSpacing.md, borderTopWidth: 1, borderTopColor: mpColors.border },
  submitBtn: { height: 52, borderRadius: mpRadius.full, backgroundColor: mpColors.accent, alignItems: "center", justifyContent: "center" },
  submitBtnDisabled: { backgroundColor: mpColors.muted },
  submitBtnText: { fontSize: mpTypography.base, fontWeight: "700", color: mpColors.void },
});
