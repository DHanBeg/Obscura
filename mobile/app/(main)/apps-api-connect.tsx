import React, { useState, useEffect, useCallback } from "react";
import {
  View, Text, ScrollView, TouchableOpacity, StyleSheet,
  TextInput, Alert, ActivityIndicator,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import * as SecureStore from "expo-secure-store";
import * as Clipboard from "expo-clipboard";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";

type AuthType = "none" | "api_key" | "bearer";

interface StoredApiKey {
  id: string;
  service: string;
  key: string;
  authType: AuthType;
  addedAt: string;
}

const AUTH_LABELS: Record<AuthType, string> = {
  none: "Yok",
  api_key: "API Anahtarı",
  bearer: "Bearer Token",
};

const SS_KEY = "obscura_api_keys_v1";

async function loadKeys(): Promise<StoredApiKey[]> {
  try {
    const raw = await SecureStore.getItemAsync(SS_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch { return []; }
}

async function saveKeys(keys: StoredApiKey[]) {
  await SecureStore.setItemAsync(SS_KEY, JSON.stringify(keys));
}

export default function AppsApiConnectScreen() {
  const [keys, setKeys] = useState<StoredApiKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [adding, setAdding] = useState(false);

  // Form state
  const [service, setService] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [authType, setAuthType] = useState<AuthType>("api_key");
  const [keyVisible, setKeyVisible] = useState(false);
  const [saving, setSaving] = useState(false);
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setKeys(await loadKeys());
    setLoading(false);
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleSave = async () => {
    if (!service.trim() || !apiKey.trim()) {
      Alert.alert("Hata", "Servis adı ve anahtar zorunludur");
      return;
    }
    setSaving(true);
    try {
      const newKey: StoredApiKey = {
        id: `key_${Date.now()}`,
        service: service.trim(),
        key: apiKey.trim(),
        authType,
        addedAt: new Date().toISOString(),
      };
      const updated = [...keys, newKey];
      await saveKeys(updated);
      setKeys(updated);
      setService("");
      setApiKey("");
      setAuthType("api_key");
      setAdding(false);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = (id: string) => {
    Alert.alert("Anahtarı Sil", "Bu API anahtarını silmek istediğinizden emin misiniz?", [
      { text: "İptal", style: "cancel" },
      {
        text: "Sil", style: "destructive",
        onPress: async () => {
          const updated = keys.filter((k) => k.id !== id);
          await saveKeys(updated);
          setKeys(updated);
        },
      },
    ]);
  };

  const handleCopy = async (key: StoredApiKey) => {
    await Clipboard.setStringAsync(key.key);
    setCopiedId(key.id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>API Bağlantıları</Text>
        <TouchableOpacity
          onPress={() => setAdding((v) => !v)}
          style={styles.addBtn}
        >
          <Ionicons name={adding ? "close" : "add"} size={22} color={colors.head} />
        </TouchableOpacity>
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        {/* Info banner */}
        <View style={styles.infoBanner}>
          <Ionicons name="key-outline" size={18} color={colors.accent} />
          <Text style={styles.infoText}>
            API anahtarlarınız cihazınızda güvenli şekilde saklanır. Hiçbir zaman sunucularımıza gönderilmez.
          </Text>
        </View>

        {/* Add form */}
        {adding && (
          <View style={styles.card}>
            <Text style={styles.cardTitle}>Yeni Bağlantı</Text>

            <View style={styles.field}>
              <Text style={styles.fieldLabel}>Servis Adı</Text>
              <TextInput
                style={styles.input}
                placeholder="örn. OpenAI, GitHub, Stripe"
                placeholderTextColor={colors.sub}
                value={service}
                onChangeText={setService}
              />
            </View>

            <View style={styles.field}>
              <Text style={styles.fieldLabel}>Kimlik Doğrulama Türü</Text>
              <View style={styles.authTypes}>
                {(["none", "api_key", "bearer"] as AuthType[]).map((t) => (
                  <TouchableOpacity
                    key={t}
                    style={[styles.authTypeBtn, authType === t && styles.authTypeBtnActive]}
                    onPress={() => setAuthType(t)}
                  >
                    <Text style={[styles.authTypeBtnText, authType === t && styles.authTypeBtnTextActive]}>
                      {AUTH_LABELS[t]}
                    </Text>
                  </TouchableOpacity>
                ))}
              </View>
            </View>

            {authType !== "none" && (
              <View style={styles.field}>
                <Text style={styles.fieldLabel}>Anahtar / Token</Text>
                <View style={styles.keyInputRow}>
                  <TextInput
                    style={[styles.input, { flex: 1 }]}
                    placeholder="sk-..."
                    placeholderTextColor={colors.sub}
                    value={apiKey}
                    onChangeText={setApiKey}
                    secureTextEntry={!keyVisible}
                    autoCapitalize="none"
                  />
                  <TouchableOpacity
                    onPress={() => setKeyVisible((v) => !v)}
                    style={styles.eyeBtn}
                  >
                    <Ionicons
                      name={keyVisible ? "eye-off-outline" : "eye-outline"}
                      size={18}
                      color={colors.sub}
                    />
                  </TouchableOpacity>
                </View>
              </View>
            )}

            <View style={styles.formBtns}>
              <TouchableOpacity
                style={[styles.btnOutline, { flex: 1 }]}
                onPress={() => { setAdding(false); setService(""); setApiKey(""); }}
              >
                <Text style={styles.btnOutlineText}>İptal</Text>
              </TouchableOpacity>
              <TouchableOpacity
                style={[styles.btn, { flex: 1 }, saving && styles.btnDisabled]}
                onPress={handleSave}
                disabled={saving}
              >
                {saving
                  ? <ActivityIndicator size="small" color={colors.void} />
                  : <Ionicons name="save-outline" size={16} color={colors.void} />}
                <Text style={styles.btnText}>Kaydet</Text>
              </TouchableOpacity>
            </View>
          </View>
        )}

        {/* Keys list */}
        <Text style={styles.sectionLabel}>KAYITLI ANAHTARLAR</Text>

        {loading ? (
          <View style={styles.center}>
            <ActivityIndicator size="small" color={colors.accent} />
          </View>
        ) : keys.length === 0 ? (
          <View style={styles.emptyCard}>
            <Ionicons name="key-outline" size={28} color={colors.sub} />
            <Text style={styles.emptyTitle}>API anahtarı eklenmedi</Text>
            <Text style={styles.emptySub}>Mini uygulamaların harici servislere bağlanması için anahtar ekleyin.</Text>
          </View>
        ) : (
          <View style={styles.card}>
            {keys.map((k, i) => (
              <View key={k.id} style={[styles.keyRow, i < keys.length - 1 && styles.keyRowBorder]}>
                <View style={styles.keyIconBox}>
                  <Ionicons name="key-outline" size={16} color={colors.tier2} />
                </View>
                <View style={{ flex: 1 }}>
                  <Text style={styles.keyService}>{k.service}</Text>
                  <View style={styles.keyMeta}>
                    <Text style={styles.keyAuthType}>{AUTH_LABELS[k.authType]}</Text>
                    <Text style={styles.keyDot}>·</Text>
                    <Text style={styles.keyDate}>
                      {new Date(k.addedAt).toLocaleDateString("tr-TR")}
                    </Text>
                  </View>
                </View>
                <TouchableOpacity onPress={() => handleCopy(k)} style={styles.keyAction}>
                  <Ionicons
                    name={copiedId === k.id ? "checkmark" : "copy-outline"}
                    size={16}
                    color={copiedId === k.id ? colors.accent : colors.sub}
                  />
                </TouchableOpacity>
                <TouchableOpacity onPress={() => handleDelete(k.id)} style={styles.keyAction}>
                  <Ionicons name="trash-outline" size={16} color={colors.red} />
                </TouchableOpacity>
              </View>
            ))}
          </View>
        )}

        {/* Security note */}
        <View style={styles.secNote}>
          <Ionicons name="lock-closed-outline" size={14} color={colors.sub} />
          <Text style={styles.secNoteText}>
            Anahtarlar Expo SecureStore ile AES şifrelemeli olarak saklanır
          </Text>
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
  addBtn: { padding: 4, width: 40, alignItems: "flex-end" },
  content: { padding: spacing.lg, gap: spacing.md, paddingBottom: 48 },

  infoBanner: {
    flexDirection: "row", gap: spacing.sm, alignItems: "flex-start",
    backgroundColor: "rgba(74,222,128,0.06)", borderRadius: radius.lg,
    borderWidth: 1, borderColor: "rgba(74,222,128,0.15)", padding: spacing.md,
  },
  infoText: { flex: 1, fontSize: typography.sm, color: colors.body, lineHeight: 20 },

  card: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, gap: spacing.md, overflow: "hidden",
  },
  cardTitle: { fontSize: typography.base, fontWeight: "700", color: colors.head },

  field: { gap: spacing.xs },
  fieldLabel: { fontSize: typography.xs, color: colors.sub, fontWeight: "600", textTransform: "uppercase", letterSpacing: 0.5 },
  input: {
    backgroundColor: colors.raised, borderRadius: radius.lg, borderWidth: 1,
    borderColor: colors.border, padding: spacing.md, color: colors.head, fontSize: typography.sm,
  },
  keyInputRow: { flexDirection: "row", alignItems: "center", gap: spacing.sm },
  eyeBtn: { padding: spacing.sm },

  authTypes: { flexDirection: "row", gap: spacing.sm },
  authTypeBtn: {
    paddingHorizontal: spacing.md, paddingVertical: spacing.sm,
    borderRadius: radius.md, backgroundColor: colors.raised, borderWidth: 1, borderColor: "transparent",
  },
  authTypeBtnActive: {
    backgroundColor: "rgba(74,222,128,0.1)", borderColor: "rgba(74,222,128,0.3)",
  },
  authTypeBtnText: { fontSize: typography.xs, fontWeight: "600", color: colors.sub },
  authTypeBtnTextActive: { color: colors.accent },

  formBtns: { flexDirection: "row", gap: spacing.sm },
  btn: {
    flexDirection: "row", alignItems: "center", justifyContent: "center", gap: spacing.sm,
    backgroundColor: colors.accent, borderRadius: radius.lg, padding: spacing.md,
  },
  btnDisabled: { opacity: 0.5 },
  btnText: { fontSize: typography.base, fontWeight: "700", color: colors.void },
  btnOutline: {
    flexDirection: "row", alignItems: "center", justifyContent: "center",
    borderRadius: radius.lg, padding: spacing.md, borderWidth: 1, borderColor: colors.border,
    backgroundColor: colors.surface,
  },
  btnOutlineText: { fontSize: typography.base, fontWeight: "600", color: colors.body },

  sectionLabel: {
    fontSize: typography.xs, color: colors.sub, fontWeight: "600",
    textTransform: "uppercase", letterSpacing: 1,
  },
  center: { padding: spacing.xl, alignItems: "center" },
  emptyCard: {
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1,
    borderColor: colors.border, padding: spacing.xl, alignItems: "center", gap: spacing.sm,
  },
  emptyTitle: { fontSize: typography.base, fontWeight: "600", color: colors.head },
  emptySub: { fontSize: typography.sm, color: colors.sub, textAlign: "center", lineHeight: 20 },

  keyRow: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm,
    paddingVertical: spacing.md, paddingHorizontal: spacing.sm,
  },
  keyRowBorder: { borderBottomWidth: 1, borderBottomColor: colors.border },
  keyIconBox: {
    width: 36, height: 36, borderRadius: radius.md, backgroundColor: "rgba(77,168,255,0.1)",
    alignItems: "center", justifyContent: "center",
    borderWidth: 1, borderColor: "rgba(77,168,255,0.2)",
  },
  keyService: { fontSize: typography.sm, fontWeight: "600", color: colors.head },
  keyMeta: { flexDirection: "row", alignItems: "center", gap: spacing.xs },
  keyAuthType: { fontSize: typography.xs, color: colors.sub },
  keyDot: { fontSize: typography.xs, color: colors.dim },
  keyDate: { fontSize: typography.xs, color: colors.sub },
  keyAction: { padding: spacing.sm },

  secNote: {
    flexDirection: "row", alignItems: "center", gap: spacing.xs, justifyContent: "center",
  },
  secNoteText: { fontSize: typography.xs, color: colors.dim },
});
