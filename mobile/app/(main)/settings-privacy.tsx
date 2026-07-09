import React, { useState, useEffect } from "react";
import {
  View, Text, ScrollView, Switch, StyleSheet, StatusBar, TouchableOpacity,
} from "react-native";
import { router } from "expo-router";
import * as SecureStore from "expo-secure-store";

const SECURE_OPTS: SecureStore.SecureStoreOptions = { keychainAccessible: SecureStore.WHEN_UNLOCKED };
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";

const KEYS = {
  readReceipts: "privacy_read_receipts",
  onlineStatus: "privacy_online_status",
  screenProtect: "privacy_screen_protect",
};

function Row({
  label, sublabel, toggle, disabled,
}: {
  label: string;
  sublabel?: string;
  toggle?: { value: boolean; onChange: (v: boolean) => void };
  disabled?: boolean;
}) {
  return (
    <View style={[styles.row, disabled && { opacity: 0.4 }]}>
      <View style={styles.rowContent}>
        <Text style={styles.rowLabel}>{label}</Text>
        {sublabel && <Text style={styles.rowSublabel}>{sublabel}</Text>}
      </View>
      {toggle && (
        <Switch
          value={toggle.value}
          onValueChange={toggle.onChange}
          trackColor={{ false: colors.muted, true: colors.accent }}
          thumbColor="#fff"
          disabled={disabled}
        />
      )}
    </View>
  );
}

export default function SettingsPrivacyScreen() {
  const insets = useSafeAreaInsets();

  const [readReceipts, setReadReceipts] = useState(true);
  const [onlineStatus, setOnlineStatus] = useState(true);
  const [screenProtect, setScreenProtect] = useState(false);

  useEffect(() => {
    Promise.all([
      SecureStore.getItemAsync(KEYS.readReceipts, SECURE_OPTS),
      SecureStore.getItemAsync(KEYS.onlineStatus, SECURE_OPTS),
      SecureStore.getItemAsync(KEYS.screenProtect, SECURE_OPTS),
    ]).then(([rr, os, sp]) => {
      if (rr !== null) setReadReceipts(rr === "true");
      if (os !== null) setOnlineStatus(os === "true");
      if (sp !== null) setScreenProtect(sp === "true");
    });
  }, []);

  function handleReadReceipts(v: boolean) {
    setReadReceipts(v);
    SecureStore.setItemAsync(KEYS.readReceipts, String(v), SECURE_OPTS);
  }

  function handleOnlineStatus(v: boolean) {
    setOnlineStatus(v);
    SecureStore.setItemAsync(KEYS.onlineStatus, String(v), SECURE_OPTS);
  }

  function handleScreenProtect(v: boolean) {
    setScreenProtect(v);
    SecureStore.setItemAsync(KEYS.screenProtect, String(v), SECURE_OPTS);
  }

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />

      {/* Header */}
      <View style={styles.header}>
        <TouchableOpacity style={styles.backBtn} onPress={() => router.back()} activeOpacity={0.7}>
          <Ionicons name="chevron-back" size={22} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Gizlilik ve Güvenlik</Text>
        <View style={styles.backBtn} />
      </View>

      <ScrollView contentContainerStyle={{ paddingBottom: insets.bottom + 40 }}>
        <View style={styles.section}>
          <View style={styles.sectionContent}>
            <Row
              label="Okundu Bildirimleri"
              sublabel="Mesajları okuduğunuzda bildirilir"
              toggle={{ value: readReceipts, onChange: handleReadReceipts }}
            />
            <Row
              label="Çevrimiçi Durumu"
              sublabel="Bağlıyken görünür"
              toggle={{ value: onlineStatus, onChange: handleOnlineStatus }}
            />
            <Row
              label="Ekran Koruma Modu"
              sublabel="Ekran görüntüsü almayı engeller"
              toggle={{ value: screenProtect, onChange: handleScreenProtect }}
            />
          </View>
        </View>

        <View style={styles.section}>
          <View style={styles.sectionContent}>
            <TouchableOpacity style={styles.row} onPress={() => router.push("/(main)/location" as any)}>
              <View style={styles.rowContent}>
                <Text style={styles.rowLabel}>Konum Kanıtı (ZK)</Text>
                <Text style={styles.rowSublabel}>Konumunuzu paylaşmadan doğrulayın</Text>
              </View>
              <Ionicons name="chevron-forward" size={18} color={colors.dim} />
            </TouchableOpacity>
          </View>
        </View>

        {/* Info card */}
        <View style={styles.infoCard}>
          <View style={styles.infoIconWrap}>
            <Ionicons name="shield-checkmark" size={20} color={colors.accent} />
          </View>
          <Text style={styles.infoText}>
            Tüm mesajlar X3DH + Double Ratchet ile uçtan uca şifrelenir
          </Text>
        </View>
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.void },

  header: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
    marginBottom: spacing.sm,
  },
  backBtn: {
    width: 36,
    height: 36,
    alignItems: "center",
    justifyContent: "center",
  },
  headerTitle: {
    fontSize: typography.md,
    fontWeight: "700",
    color: colors.head,
    flex: 1,
    textAlign: "center",
  },

  section: { marginBottom: spacing.md },
  sectionContent: {
    marginHorizontal: spacing.md,
    backgroundColor: colors.surface,
    borderRadius: radius.xl,
    borderWidth: 1,
    borderColor: colors.border,
    overflow: "hidden",
  },

  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: spacing.md,
    paddingVertical: 14,
    borderBottomWidth: 1,
    borderBottomColor: "rgba(255,255,255,0.05)",
  },
  rowContent: { flex: 1, marginRight: spacing.sm },
  rowLabel: { fontSize: typography.sm, color: colors.body, fontWeight: "500" },
  rowSublabel: { fontSize: 11, color: colors.dim, marginTop: 2 },

  infoCard: {
    flexDirection: "row",
    alignItems: "flex-start",
    gap: 12,
    marginHorizontal: spacing.md,
    marginTop: spacing.sm,
    backgroundColor: "rgba(74,222,128,0.06)",
    borderRadius: radius.xl,
    borderWidth: 1,
    borderColor: "rgba(74,222,128,0.15)",
    padding: spacing.md,
  },
  infoIconWrap: { marginTop: 1 },
  infoText: {
    flex: 1,
    fontSize: typography.sm,
    color: colors.body,
    lineHeight: 20,
  },
});
