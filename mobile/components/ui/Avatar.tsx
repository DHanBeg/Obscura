import React from "react";
import { View, Text, StyleSheet } from "react-native";
import { colors, radius } from "@/lib/theme";
import { initials } from "@/lib/format";

const COLORS = [
  ["#3b1a6e", "#5b2d8e"],
  ["#1a2e6e", "#2d4d8e"],
  ["#1a5c3b", "#2d8e5b"],
  ["#6e1a2e", "#8e2d4d"],
  ["#6e4a1a", "#8e6b2d"],
  ["#1a5c6e", "#2d8e8e"],
  ["#3b2d6e", "#5b4d8e"],
];

function avatarColor(name: string): string[] {
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash);
  return COLORS[Math.abs(hash) % COLORS.length];
}

const sizes = {
  xs: 28, sm: 36, md: 44, lg: 56, xl: 80,
};

const fontSizes = {
  xs: 11, sm: 13, md: 15, lg: 18, xl: 26,
};

interface Props {
  name: string;
  size?: keyof typeof sizes;
  online?: boolean;
  tier?: number;
}

export function Avatar({ name = "?", size = "md", online, tier }: Props) {
  const s = sizes[size];
  const fs = fontSizes[size];
  const [bg] = avatarColor(name);
  const dotSize = s * 0.28;

  const tierColor = tier && tier >= 3
    ? { 3: "#a78bfa", 4: "#f59e0b", 5: "#5ec46e" }[tier]
    : null;

  return (
    <View style={{ width: s, height: s }}>
      <View style={[
        styles.circle,
        { width: s, height: s, borderRadius: s / 2, backgroundColor: bg },
        tierColor ? { borderWidth: 1.5, borderColor: tierColor + "66" } : null,
      ]}>
        <Text style={[styles.text, { fontSize: fs }]}>{initials(name)}</Text>
      </View>
      {online !== undefined && (
        <View style={[
          styles.dot,
          {
            width: dotSize, height: dotSize,
            borderRadius: dotSize / 2,
            backgroundColor: online ? colors.accent : colors.dim,
          }
        ]} />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  circle: {
    alignItems: "center",
    justifyContent: "center",
    overflow: "hidden",
  },
  text: {
    color: "rgba(240,240,240,0.85)",
    fontWeight: "600",
  },
  dot: {
    position: "absolute",
    bottom: 0,
    right: 0,
    borderWidth: 2,
    borderColor: colors.void,
  },
});
