import React, { useEffect, useState } from "react";
import { View, Text, TouchableOpacity, StyleSheet, ActivityIndicator } from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, typography, radius } from "@/lib/theme";
import { api } from "@/lib/api";

export default function GovernanceScreen() {
  const insets = useSafeAreaInsets();
  const [status, setStatus] = useState<"loading" | "no_obs" | "error">("loading");

  useEffect(() => {
    (async () => {
      try {
        const { conv_id } = await api.joinOBSCommunity();
        router.replace(`/(main)/chat/${conv_id}` as any);
      } catch (e: any) {
        if (e?.message?.includes("en az 1 OBS") || e?.message?.includes("403")) {
          setStatus("no_obs");
        } else {
          setStatus("error");
        }
      }
    })();
  }, []);

  if (status === "loading") {
    return (
      <View style={[styles.center, { paddingTop: insets.top }]}>
        <ActivityIndicator size="large" color={colors.accent} />
        <Text style={styles.loadingText}>OBS Topluluk'a bağlanıyor...</Text>
      </View>
    );
  }

  return (
    <View style={[styles.center, { paddingTop: insets.top }]}>
      <Ionicons name="lock-closed-outline" size={48} color={colors.dim} style={{ marginBottom: spacing.lg }} />
      <Text style={styles.title}>OBS Topluluğu</Text>
      <Text style={styles.sub}>
        {status === "no_obs"
          ? "Bu alana erişmek için en az 1 OBS gereklidir."
          : "Bağlantı hatası. Tekrar dene."}
      </Text>
      {status === "no_obs" && (
        <TouchableOpacity style={styles.btn} onPress={() => router.push("/(main)/wallet" as any)}>
          <Text style={styles.btnText}>Cüzdana Git</Text>
        </TouchableOpacity>
      )}
      {status === "error" && (
        <TouchableOpacity style={styles.btn} onPress={() => setStatus("loading")}>
          <Text style={styles.btnText}>Tekrar Dene</Text>
        </TouchableOpacity>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  center: { flex: 1, backgroundColor: colors.void, alignItems: "center", justifyContent: "center", padding: spacing.xl },
  loadingText: { marginTop: spacing.md, fontSize: typography.sm, color: colors.dim },
  title: { fontSize: typography.lg, fontWeight: "700", color: colors.head, marginBottom: spacing.sm },
  sub: { fontSize: typography.sm, color: colors.sub, textAlign: "center", marginBottom: spacing.xl, lineHeight: 20 },
  btn: { backgroundColor: colors.accent, paddingHorizontal: spacing.xl, paddingVertical: spacing.md, borderRadius: radius.lg },
  btnText: { fontSize: typography.sm, fontWeight: "700", color: colors.void },
});
