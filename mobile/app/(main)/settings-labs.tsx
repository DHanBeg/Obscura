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
  zkAuth: "labs_zk_auth",
  pqCrypto: "labs_pq_crypto",
  distStorage: "labs_dist_storage",
  fedMessaging: "labs_fed_messaging",
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

export default function SettingsLabsScreen() {
  const insets = useSafeAreaInsets();

  const [zkAuth, setZkAuth] = useState(false);
  const [pqCrypto, setPqCrypto] = useState(false);
  const [distStorage, setDistStorage] = useState(false);
  const [fedMessaging, setFedMessaging] = useState(false);

  useEffect(() => {
    AsyncStorage.multiGet([KEYS.zkAuth, KEYS.pqCrypto, KEYS.distStorage, KEYS.fedMessaging]).then(
      (pairs) => {
        pairs.forEach(([key, val]) => {
          if (val === null) return;
          const parsed = val === "true";
          if (key === KEYS.zkAuth) setZkAuth(parsed);
          if (key === KEYS.pqCrypto) setPqCrypto(parsed);
          if (key === KEYS.distStorage) setDistStorage(parsed);
          if (key === KEYS.fedMessaging) setFedMessaging(parsed);
        });
      }
    );
  }, []);

  function handleZkAuth(v: boolean) {
    setZkAuth(v);
    AsyncStorage.setItem(KEYS.zkAuth, String(v));
  }

  function handlePqCrypto(v: boolean) {
    setPqCrypto(v);
    AsyncStorage.setItem(KEYS.pqCrypto, String(v));
  }

  function handleDistStorage(v: boolean) {
    setDistStorage(v);
    AsyncStorage.setItem(KEYS.distStorage, String(v));
  }

  function handleFedMessaging(v: boolean) {
    setFedMessaging(v);
    AsyncStorage.setItem(KEYS.fedMessaging, String(v));
  }

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <StatusBar barStyle="light-content" />

      {/* Header */}
      <View style={styles.header}>
        <TouchableOpacity style={styles.backBtn} onPress={() => router.back()} activeOpacity={0.7}>
          <Ionicons name="chevron-back" size={22} color={colors.head} />
        </TouchableOpacity>
        <View style={styles.headerCenter}>
          <Text style={styles.headerTitle}>Atış Alanı</Text>
          <View style={styles.betaBadge}>
            <Text style={styles.betaText}>BETA</Text>
          </View>
        </View>
        <View style={styles.backBtn} />
      </View>

      <ScrollView contentContainerStyle={{ paddingBottom: insets.bottom + 40 }}>

        {/* Warning banner */}
        <View style={styles.warningBanner}>
          <Ionicons name="warning-outline" size={16} color={colors.amber} style={{ marginTop: 1 }} />
          <Text style={styles.warningText}>
            Deneysel özellikler kararsız olabilir
          </Text>
        </View>

        {/* Feature toggles */}
        <View style={styles.sectionContent}>
          <Row
            label="ZK Kimlik Doğrulama"
            sublabel="Groth16 tabanlı anonim giriş"
            toggle={{ value: zkAuth, onChange: handleZkAuth }}
          />
          <Row
            label="Post-Quantum Şifreleme"
            sublabel="Dilithium3 + Kyber1024"
            toggle={{ value: pqCrypto, onChange: handlePqCrypto }}
          />
          <Row
            label="Dağıtık Depolama"
            sublabel="IPFS entegrasyonu"
            toggle={{ value: distStorage, onChange: handleDistStorage }}
          />
          <Row
            label="Federe Mesajlaşma"
            sublabel="Matrix protokolü"
            toggle={{ value: fedMessaging, onChange: handleFedMessaging }}
          />
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
  headerCenter: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 8,
  },
  headerTitle: {
    fontSize: typography.md,
    fontWeight: "700",
    color: colors.head,
  },
  betaBadge: {
    backgroundColor: colors.amber,
    borderRadius: 6,
    paddingHorizontal: 7,
    paddingVertical: 2,
  },
  betaText: {
    fontSize: 10,
    fontWeight: "700",
    color: colors.void,
  },

  warningBanner: {
    flexDirection: "row",
    alignItems: "flex-start",
    gap: 10,
    marginHorizontal: spacing.md,
    marginBottom: spacing.md,
    backgroundColor: "rgba(245,158,11,0.08)",
    borderRadius: radius.xl,
    borderWidth: 1,
    borderColor: "rgba(245,158,11,0.2)",
    padding: spacing.md,
  },
  warningText: {
    flex: 1,
    fontSize: typography.sm,
    color: colors.body,
    lineHeight: 20,
  },

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
