import React, { useState, useEffect } from "react";
import {
  View, Text, ScrollView, StyleSheet, StatusBar, TouchableOpacity, Modal,
} from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import * as SecureStore from "expo-secure-store";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { useStore } from "@/lib/store";

const FONT_SIZES = [
  { key: "small",  label: "Küçük",  value: 13 },
  { key: "medium", label: "Orta",   value: 15 },
  { key: "large",  label: "Büyük",  value: 18 },
];

const CHAT_BACKGROUNDS = [
  { key: "default",  label: "Varsayılan",  color: colors.void },
  { key: "midnight", label: "Gece Yarısı", color: "#0a0f1a" },
  { key: "forest",   label: "Orman",       color: "#0d1f0f" },
  { key: "ocean",    label: "Okyanus",     color: "#071525" },
  { key: "dusk",     label: "Alacakaranlık", color: "#1a0d1f" },
];

const PREF_FONT_KEY = "obscura_pref_font_size";
const PREF_BG_KEY   = "obscura_pref_chat_bg";

function Row({
  label, sublabel, value, onPress,
}: { label: string; sublabel?: string; value?: string; onPress?: () => void }) {
  return (
    <TouchableOpacity style={styles.row} onPress={onPress} activeOpacity={onPress ? 0.7 : 1}>
      <View style={styles.rowContent}>
        <Text style={styles.rowLabel}>{label}</Text>
        {sublabel && <Text style={styles.rowSublabel}>{sublabel}</Text>}
      </View>
      {value && (
        <View style={styles.rowValueWrap}>
          <Text style={styles.rowValue}>{value}</Text>
          {onPress && <Ionicons name="chevron-forward" size={14} color={colors.dim} />}
        </View>
      )}
    </TouchableOpacity>
  );
}

function SectionTitle({ title }: { title: string }) {
  return <Text style={styles.sectionTitle}>{title}</Text>;
}

export default function SettingsAppearanceScreen() {
  const insets = useSafeAreaInsets();
  const { setFontSize } = useStore();
  const [fontSizeKey, setFontSizeKey] = useState("medium");
  const [chatBgKey, setChatBgKey] = useState("default");
  const [showFontModal, setShowFontModal] = useState(false);
  const [showBgModal, setShowBgModal] = useState(false);

  useEffect(() => {
    SecureStore.getItemAsync(PREF_FONT_KEY).then((v) => {
      if (v) {
        setFontSizeKey(v);
        const found = FONT_SIZES.find((f) => f.key === v);
        if (found) setFontSize(found.value);
      }
    });
    SecureStore.getItemAsync(PREF_BG_KEY).then((v) => { if (v) setChatBgKey(v); });
  }, []);

  const saveFontSize = async (key: string) => {
    setFontSizeKey(key);
    await SecureStore.setItemAsync(PREF_FONT_KEY, key);
    const found = FONT_SIZES.find((f) => f.key === key);
    if (found) setFontSize(found.value);
    setShowFontModal(false);
  };

  const saveChatBg = async (key: string) => {
    setChatBgKey(key);
    await SecureStore.setItemAsync(PREF_BG_KEY, key);
    setShowBgModal(false);
  };

  const currentFont = FONT_SIZES.find((f) => f.key === fontSizeKey) ?? FONT_SIZES[1];
  const currentBg   = CHAT_BACKGROUNDS.find((b) => b.key === chatBgKey) ?? CHAT_BACKGROUNDS[0];

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />

      <View style={styles.header}>
        <TouchableOpacity style={styles.backBtn} onPress={() => router.back()} activeOpacity={0.7}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Görünüm</Text>
        <View style={styles.backBtn} />
      </View>

      <ScrollView contentContainerStyle={{ paddingBottom: insets.bottom + 40 }}>

        <SectionTitle title="TEMA" />
        <View style={styles.sectionContent}>
          <View style={styles.row}>
            <View style={styles.activeIconWrap}>
              <Ionicons name="checkmark" size={16} color={colors.accent} />
            </View>
            <View style={styles.rowContent}>
              <Text style={styles.rowLabel}>Koyu</Text>
            </View>
            <Text style={[styles.rowValue, { color: colors.accent }]}>Aktif</Text>
          </View>
          <Row label="Açık" value="Yakında" />
        </View>

        <SectionTitle title="METİN" />
        <View style={styles.sectionContent}>
          <Row
            label="Yazı Boyutu"
            sublabel={`Şu an: ${currentFont.value}px`}
            value={currentFont.label}
            onPress={() => setShowFontModal(true)}
          />
        </View>

        <SectionTitle title="SOHBET" />
        <View style={styles.sectionContent}>
          <Row
            label="Sohbet Arka Planı"
            value={currentBg.label}
            onPress={() => setShowBgModal(true)}
          />
        </View>

      </ScrollView>

      {/* Font Size Modal */}
      <Modal visible={showFontModal} transparent animationType="slide" onRequestClose={() => setShowFontModal(false)}>
        <TouchableOpacity style={styles.modalOverlay} activeOpacity={1} onPress={() => setShowFontModal(false)}>
          <View style={styles.modalSheet}>
            <View style={styles.modalHandle} />
            <Text style={styles.modalTitle}>Yazı Boyutu</Text>
            {FONT_SIZES.map((f) => (
              <TouchableOpacity
                key={f.key}
                style={[styles.modalOption, fontSizeKey === f.key && styles.modalOptionActive]}
                onPress={() => saveFontSize(f.key)}
              >
                <Text style={[styles.modalOptionText, { fontSize: f.value }, fontSizeKey === f.key && styles.modalOptionTextActive]}>
                  {f.label}
                </Text>
                {fontSizeKey === f.key && <Ionicons name="checkmark" size={18} color={colors.accent} />}
              </TouchableOpacity>
            ))}
          </View>
        </TouchableOpacity>
      </Modal>

      {/* Chat Background Modal */}
      <Modal visible={showBgModal} transparent animationType="slide" onRequestClose={() => setShowBgModal(false)}>
        <TouchableOpacity style={styles.modalOverlay} activeOpacity={1} onPress={() => setShowBgModal(false)}>
          <View style={styles.modalSheet}>
            <View style={styles.modalHandle} />
            <Text style={styles.modalTitle}>Sohbet Arka Planı</Text>
            {CHAT_BACKGROUNDS.map((bg) => (
              <TouchableOpacity
                key={bg.key}
                style={[styles.modalOption, chatBgKey === bg.key && styles.modalOptionActive]}
                onPress={() => saveChatBg(bg.key)}
              >
                <View style={[styles.bgSwatch, { backgroundColor: bg.color }]} />
                <Text style={[styles.modalOptionText, chatBgKey === bg.key && styles.modalOptionTextActive]}>
                  {bg.label}
                </Text>
                {chatBgKey === bg.key && <Ionicons name="checkmark" size={18} color={colors.accent} />}
              </TouchableOpacity>
            ))}
          </View>
        </TouchableOpacity>
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },
  header: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    paddingHorizontal: spacing.md, paddingVertical: spacing.sm, marginBottom: spacing.sm,
  },
  backBtn: { width: 40, height: 40, alignItems: "center", justifyContent: "center" },
  headerTitle: { fontSize: typography.md, fontWeight: "700", color: colors.head, flex: 1, textAlign: "center" },
  sectionTitle: {
    fontSize: 11, fontWeight: "600", color: colors.dim, textTransform: "uppercase",
    letterSpacing: 1.2, paddingHorizontal: spacing.lg, paddingTop: spacing.md, paddingBottom: spacing.sm,
  },
  sectionContent: {
    marginHorizontal: spacing.md, backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border, overflow: "hidden", marginBottom: spacing.sm,
  },
  row: {
    flexDirection: "row", alignItems: "center", paddingHorizontal: spacing.md, paddingVertical: 14,
    borderBottomWidth: 1, borderBottomColor: "rgba(255,255,255,0.05)",
  },
  activeIconWrap: {
    width: 28, height: 28, borderRadius: radius.sm,
    backgroundColor: "rgba(74,222,128,0.1)", alignItems: "center", justifyContent: "center", marginRight: 10,
  },
  rowContent: { flex: 1 },
  rowLabel: { fontSize: typography.sm, color: colors.body, fontWeight: "500" },
  rowSublabel: { fontSize: 11, color: colors.dim, marginTop: 2 },
  rowValueWrap: { flexDirection: "row", alignItems: "center", gap: 4 },
  rowValue: { fontSize: 12, color: colors.dim },
  modalOverlay: { flex: 1, backgroundColor: "rgba(0,0,0,0.6)", justifyContent: "flex-end" },
  modalSheet: {
    backgroundColor: colors.surface, borderTopLeftRadius: 24, borderTopRightRadius: 24,
    padding: spacing.lg, paddingBottom: 32, gap: spacing.xs,
  },
  modalHandle: {
    width: 36, height: 4, borderRadius: 2,
    backgroundColor: colors.border, alignSelf: "center", marginBottom: spacing.md,
  },
  modalTitle: { fontSize: typography.base, fontWeight: "700", color: colors.head, marginBottom: spacing.sm },
  modalOption: {
    flexDirection: "row", alignItems: "center", gap: spacing.md,
    padding: spacing.md, borderRadius: radius.lg,
  },
  modalOptionActive: { backgroundColor: "rgba(74,222,128,0.08)" },
  modalOptionText: { flex: 1, fontSize: typography.base, color: colors.body },
  modalOptionTextActive: { color: colors.accent, fontWeight: "600" },
  bgSwatch: { width: 32, height: 32, borderRadius: radius.sm, borderWidth: 1, borderColor: colors.border },
});
