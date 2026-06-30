import React, { useState, useEffect } from "react";
import {
  View, Text, ScrollView, Switch, StyleSheet, StatusBar, TouchableOpacity,
} from "react-native";
import { router } from "expo-router";
import AsyncStorage from "@react-native-async-storage/async-storage";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";

const KEYS = {
  push: "notif_push",
  messages: "notif_messages",
  sound: "notif_sound",
  vibration: "notif_vibration",
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

export default function SettingsNotificationsScreen() {
  const insets = useSafeAreaInsets();

  const [push, setPush] = useState(true);
  const [messages, setMessages] = useState(true);
  const [sound, setSound] = useState(true);
  const [vibration, setVibration] = useState(true);

  useEffect(() => {
    AsyncStorage.multiGet([KEYS.push, KEYS.messages, KEYS.sound, KEYS.vibration]).then(
      (pairs) => {
        pairs.forEach(([key, val]) => {
          if (val === null) return;
          const parsed = val === "true";
          if (key === KEYS.push) setPush(parsed);
          if (key === KEYS.messages) setMessages(parsed);
          if (key === KEYS.sound) setSound(parsed);
          if (key === KEYS.vibration) setVibration(parsed);
        });
      }
    );
  }, []);

  function handlePush(v: boolean) {
    setPush(v);
    AsyncStorage.setItem(KEYS.push, String(v));
  }

  function handleMessages(v: boolean) {
    setMessages(v);
    AsyncStorage.setItem(KEYS.messages, String(v));
  }

  function handleSound(v: boolean) {
    setSound(v);
    AsyncStorage.setItem(KEYS.sound, String(v));
  }

  function handleVibration(v: boolean) {
    setVibration(v);
    AsyncStorage.setItem(KEYS.vibration, String(v));
  }

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />

      {/* Header */}
      <View style={styles.header}>
        <TouchableOpacity style={styles.backBtn} onPress={() => router.back()} activeOpacity={0.7}>
          <Ionicons name="chevron-back" size={22} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Bildirimler</Text>
        <View style={styles.backBtn} />
      </View>

      <ScrollView contentContainerStyle={{ paddingBottom: insets.bottom + 40 }}>
        <View style={styles.section}>
          <View style={styles.sectionContent}>
            <Row
              label="Push Bildirimleri"
              toggle={{ value: push, onChange: handlePush }}
            />
            <Row
              label="Mesaj Bildirimleri"
              sublabel="Yeni mesaj geldiğinde bildir"
              toggle={{ value: messages, onChange: handleMessages }}
            />
            <Row
              label="Ses"
              sublabel="Bildirim sesi çalsın"
              toggle={{ value: sound, onChange: handleSound }}
            />
            <Row
              label="Titreşim"
              toggle={{ value: vibration, onChange: handleVibration }}
            />
          </View>
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
});
