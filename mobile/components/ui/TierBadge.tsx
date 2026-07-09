import React from "react";
import { View, Text, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { colors, radius, typography } from "@/lib/theme";

export const TIER_LABELS: Record<number, string> = {
  0: "Yeni",
  1: "Bronz",
  2: "Gümüş",
  3: "Altın",
  4: "Platin",
  5: "Elmas",
};

export const TIER_COLORS: Record<number, string> = {
  0: colors.dim,
  1: colors.tier1,
  2: colors.tier2,
  3: colors.tier3,
  4: colors.tier4,
  5: colors.tier5,
};

export function tierColor(tier: number): string {
  return TIER_COLORS[tier] ?? colors.dim;
}

export function tierLabel(tier: number): string {
  return TIER_LABELS[tier] ?? String(tier);
}

interface Props {
  tier: number;
  size?: "sm" | "md";
}

export function TierBadge({ tier, size = "md" }: Props) {
  const c = tierColor(tier);
  const small = size === "sm";
  return (
    <View style={[styles.badge, { borderColor: c + "55", backgroundColor: c + "18" }, small && styles.badgeSm]}>
      <Ionicons name="shield-checkmark-outline" size={small ? 10 : 12} color={c} />
      <Text style={[styles.text, { color: c }, small && styles.textSm]}>{tierLabel(tier)}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  badge: {
    flexDirection: "row", alignItems: "center", gap: 4,
    borderRadius: radius.full, borderWidth: 1,
    paddingHorizontal: 8, paddingVertical: 3,
  },
  badgeSm: { paddingHorizontal: 5, paddingVertical: 1 },
  text: { fontSize: typography.xs, fontWeight: "700" },
  textSm: { fontSize: 10 },
});
