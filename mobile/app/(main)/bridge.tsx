import React, { useState, useEffect, useCallback } from "react";
import {
  View, Text, StyleSheet, ScrollView, TouchableOpacity,
  TextInput, ActivityIndicator, Alert, RefreshControl,
} from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { colors } from "@/lib/theme";
import { api } from "@/lib/api";

type Chain = "ethereum" | "polkadot";

const CHAINS: Record<Chain, { name: string; color: string }> = {
  ethereum: { name: "Ethereum", color: "#627EEA" },
  polkadot: { name: "Polkadot", color: "#E6007A" },
};

export default function BridgeScreen() {
  const [status, setStatus]     = useState<any>(null);
  const [loading, setLoading]   = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [fromChain, setFrom]    = useState<Chain>("ethereum");
  const [toChain, setTo]        = useState<Chain>("polkadot");
  const [amount, setAmount]     = useState("");
  const [senderAddr, setSender] = useState("");
  const [recipientAddr, setRecip] = useState("");
  const [locking, setLocking]   = useState(false);

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true);
    try {
      const res = await api.bridgeStatus();
      setStatus(res);
    } catch {}
    finally { setLoading(false); setRefreshing(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const swap = () => {
    setFrom(toChain);
    setTo(fromChain);
  };

  const handleLock = async () => {
    if (!amount.trim() || !senderAddr.trim() || !recipientAddr.trim()) {
      Alert.alert("Eksik Alan", "Tüm alanları doldurun.");
      return;
    }
    setLocking(true);
    try {
      const res = await api.bridgeLock({
        from_chain: fromChain,
        to_chain: toChain,
        amount: amount.trim(),
        sender_addr: senderAddr.trim(),
        recipient_addr: recipientAddr.trim(),
      });
      Alert.alert(
        "Başlatıldı",
        `TX: ${res?.tx_id || "stub"}\nDurum: ${res?.status || "pending"}`,
      );
      setAmount(""); setSender(""); setRecip("");
    } catch (e: any) {
      Alert.alert("Hata", e.message);
    } finally {
      setLocking(false);
    }
  };

  if (loading) {
    return <View style={styles.center}><ActivityIndicator color={colors.accent} size="large" /></View>;
  }

  const fromInfo = CHAINS[fromChain];
  const toInfo   = CHAINS[toChain];

  return (
    <View style={styles.container}>
      <ScrollView
        contentContainerStyle={styles.scroll}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => load(true)} tintColor={colors.accent} />}
      >
        {/* Header */}
        <Text style={styles.title}>Cross-Chain Bridge</Text>
        <Text style={styles.subtitle}>OBS tokenini zincirler arası taşı</Text>

        {/* Status banner */}
        {status && (
          <View style={styles.statusBanner}>
            <Ionicons name="checkmark-circle" size={14} color={colors.accent} />
            <Text style={styles.statusText}>
              Bridge aktif — {Array.isArray(status.chains) ? status.chains.join(", ") : ""}
            </Text>
          </View>
        )}

        {/* Chain selector */}
        <View style={styles.card}>
          <Text style={styles.sectionLabel}>ZİNCİR</Text>
          <View style={styles.chainRow}>
            <View style={[styles.chainBox, { borderColor: fromInfo.color + "55" }]}>
              <Text style={styles.chainLabel}>Kaynak</Text>
              <View style={[styles.chainDot, { backgroundColor: fromInfo.color + "30" }]}>
                <View style={[styles.chainDotInner, { backgroundColor: fromInfo.color }]} />
              </View>
              <Text style={[styles.chainName, { color: fromInfo.color }]}>{fromInfo.name}</Text>
            </View>

            <TouchableOpacity style={styles.swapBtn} onPress={swap}>
              <Ionicons name="swap-horizontal" size={18} color={colors.sub} />
            </TouchableOpacity>

            <View style={[styles.chainBox, { borderColor: toInfo.color + "55" }]}>
              <Text style={styles.chainLabel}>Hedef</Text>
              <View style={[styles.chainDot, { backgroundColor: toInfo.color + "30" }]}>
                <View style={[styles.chainDotInner, { backgroundColor: toInfo.color }]} />
              </View>
              <Text style={[styles.chainName, { color: toInfo.color }]}>{toInfo.name}</Text>
            </View>
          </View>
        </View>

        {/* Form */}
        <View style={styles.card}>
          <Text style={styles.sectionLabel}>İŞLEM DETAYI</Text>

          <Text style={styles.inputLabel}>Miktar (OBS)</Text>
          <TextInput
            value={amount} onChangeText={setAmount}
            placeholder="0.0000" placeholderTextColor={colors.dim}
            style={styles.input} keyboardType="decimal-pad"
          />

          <Text style={styles.inputLabel}>Gönderici Adres ({fromInfo.name})</Text>
          <TextInput
            value={senderAddr} onChangeText={setSender}
            placeholder="0x..." placeholderTextColor={colors.dim}
            style={[styles.input, styles.mono]} autoCapitalize="none"
          />

          <Text style={styles.inputLabel}>Alıcı Adres ({toInfo.name})</Text>
          <TextInput
            value={recipientAddr} onChangeText={setRecip}
            placeholder="0x..." placeholderTextColor={colors.dim}
            style={[styles.input, styles.mono]} autoCapitalize="none"
          />
        </View>

        {/* Warning */}
        <View style={styles.warning}>
          <Ionicons name="warning-outline" size={13} color={colors.amber} />
          <Text style={styles.warningText}>FAZ 3 stub — gerçek relayer FAZ 4'te aktif olacak.</Text>
        </View>

        {/* Submit */}
        <TouchableOpacity
          style={[styles.lockBtn, (locking || !amount || !senderAddr || !recipientAddr) && styles.disabledBtn]}
          onPress={handleLock}
          disabled={locking || !amount.trim() || !senderAddr.trim() || !recipientAddr.trim()}
        >
          {locking
            ? <ActivityIndicator color="#000" size="small" />
            : <>
                <Ionicons name="lock-closed" size={16} color="#000" />
                <Text style={styles.lockBtnText}>OBS Kilitle & Köprüle</Text>
              </>
          }
        </TouchableOpacity>
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  container:    { flex: 1, backgroundColor: colors.void },
  center:       { flex: 1, alignItems: "center", justifyContent: "center", backgroundColor: colors.void },
  scroll:       { padding: 16, paddingTop: 60, paddingBottom: 100 },
  title:        { fontSize: 26, fontWeight: "700", color: colors.head, marginBottom: 4 },
  subtitle:     { fontSize: 13, color: colors.dim, marginBottom: 16 },

  statusBanner: {
    flexDirection: "row", alignItems: "center", gap: 6,
    backgroundColor: colors.accentDeep, borderRadius: 12,
    borderWidth: 1, borderColor: colors.accentDim,
    paddingHorizontal: 12, paddingVertical: 8, marginBottom: 16,
  },
  statusText:   { fontSize: 12, color: colors.accent },

  card: {
    backgroundColor: colors.surface, borderRadius: 20,
    borderWidth: 1, borderColor: colors.border,
    padding: 14, marginBottom: 12,
  },
  sectionLabel: { fontSize: 11, color: colors.sub, letterSpacing: 1.5, fontWeight: "700", marginBottom: 12 },

  chainRow:     { flexDirection: "row", alignItems: "center", gap: 8 },
  chainBox: {
    flex: 1, borderRadius: 14, borderWidth: 1,
    backgroundColor: colors.raised, padding: 12, alignItems: "center", gap: 6,
  },
  chainLabel:   { fontSize: 11, color: colors.sub },
  chainDot:     { width: 28, height: 28, borderRadius: 14, alignItems: "center", justifyContent: "center" },
  chainDotInner: { width: 12, height: 12, borderRadius: 6 },
  chainName:    { fontSize: 13, fontWeight: "700" },
  swapBtn: {
    width: 38, height: 38, borderRadius: 12,
    backgroundColor: colors.raised, borderWidth: 1, borderColor: colors.border,
    alignItems: "center", justifyContent: "center",
  },

  inputLabel:   { fontSize: 12, color: colors.sub, marginBottom: 6 },
  input: {
    backgroundColor: colors.raised, borderWidth: 1, borderColor: colors.border,
    borderRadius: 12, paddingHorizontal: 12, paddingVertical: 10,
    fontSize: 14, color: colors.body, marginBottom: 12,
  },
  mono:         { fontFamily: "Courier", fontSize: 12 },

  warning: {
    flexDirection: "row", alignItems: "center", gap: 6,
    backgroundColor: "rgba(245,158,11,0.08)", borderRadius: 12,
    borderWidth: 1, borderColor: "rgba(245,158,11,0.2)",
    paddingHorizontal: 12, paddingVertical: 8, marginBottom: 16,
  },
  warningText:  { flex: 1, fontSize: 12, color: colors.amber },

  lockBtn: {
    flexDirection: "row", alignItems: "center", justifyContent: "center",
    gap: 8, height: 50, borderRadius: 16, backgroundColor: colors.accent,
  },
  lockBtnText:  { fontSize: 15, fontWeight: "700", color: "#000" },
  disabledBtn:  { opacity: 0.4 },
});
