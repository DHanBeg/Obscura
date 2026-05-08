import React from "react";
import { View, Text, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { colors } from "@/lib/theme";

interface Props {
  verified?: boolean;
  showLabel?: boolean;
}

export function EncryptionBadge({ verified = true, showLabel }: Props) {
  return (
    <View style={styles.row}>
      <Ionicons
        name={verified ? "shield-checkmark" : "shield"}
        size={13}
        color={verified ? colors.accent : colors.amber}
      />
      {showLabel && (
        <Text style={[styles.label, { color: verified ? colors.accent : colors.amber }]}>
          {verified ? "Uçtan uca şifreli" : "Doğrulama bekleniyor"}
        </Text>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: "row", alignItems: "center", gap: 4 },
  label: { fontSize: 11, fontWeight: "500" },
});
