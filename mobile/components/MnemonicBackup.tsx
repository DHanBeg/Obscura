/**
 * MnemonicBackup — Mobile BIP39 12 kelime yedekleme ekranı
 *
 * Spec Bölüm 5.2 Adım 7
 * Stack navigation içinde tam ekran render edilir.
 *
 * Stage akışı: show → verify → done → onComplete()
 */

import React, { useState, useRef, useCallback, useEffect } from "react";
import {
  View,
  Text,
  TextInput,
  TouchableOpacity,
  ScrollView,
  FlatList,
  StyleSheet,
  Alert,
  Clipboard,
  ActivityIndicator,
} from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import {
  getMnemonicWords,
  pickVerificationWords,
  markMnemonicBackedUp,
} from "@/lib/mnemonic";

// ─── Tipler ───────────────────────────────────────────────────────────────────

interface Props {
  mnemonic: string;
  onComplete: () => void;
}

type Stage = "show" | "verify" | "done";

interface VerifySlot {
  index: number;
  word: string;
  value: string;
  status: "idle" | "correct" | "wrong";
}

// ─── Sub: WordCell ────────────────────────────────────────────────────────────

function WordCell({ index, word }: { index: number; word: string }) {
  return (
    <View style={styles.wordCell}>
      <Text style={styles.wordIndex}>{index + 1}</Text>
      <Text style={styles.wordText}>{word}</Text>
    </View>
  );
}

// ─── Stage: Göster ────────────────────────────────────────────────────────────

function ShowStage({
  mnemonic,
  onConfirm,
}: {
  mnemonic: string;
  onConfirm: () => void;
}) {
  const words = getMnemonicWords(mnemonic);
  const [checked, setChecked] = useState(false);
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    Clipboard.setString(mnemonic);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  // 2 kolonlu grid için çift-sıra eşleştirmesi
  const pairs: [string, string, number, number][] = [];
  for (let i = 0; i < 12; i += 2) {
    pairs.push([words[i], words[i + 1], i, i + 1]);
  }

  return (
    <ScrollView
      style={styles.stageScroll}
      contentContainerStyle={styles.stageContent}
      showsVerticalScrollIndicator={false}
    >
      {/* İkon */}
      <View style={styles.iconCircle}>
        <Ionicons name="shield-checkmark" size={28} color={colors.accent} />
      </View>

      <Text style={styles.stageTitle}>Kurtarma İfadenizi Kaydedin</Text>
      <Text style={styles.stageSub}>
        Bu 12 kelime hesabınızın tek yedeğidir. Güvenli bir yere not alın.
      </Text>

      {/* Uyarı */}
      <View style={styles.warningBand}>
        <Ionicons name="warning" size={14} color={colors.amber} style={{ marginTop: 1 }} />
        <Text style={styles.warningText}>
          Bu kelimeleri kimseyle paylaşmayın. Obscura bunları hiçbir zaman sormaz.
        </Text>
      </View>

      {/* Kelime grid: 2 kolon */}
      <View style={styles.wordGrid}>
        {pairs.map(([w1, w2, i1, i2]) => (
          <View key={i1} style={styles.wordRow}>
            <View style={styles.wordCellWrap}>
              <WordCell index={i1} word={w1} />
            </View>
            <View style={styles.wordCellWrap}>
              <WordCell index={i2} word={w2} />
            </View>
          </View>
        ))}
      </View>

      {/* Kopyala */}
      <TouchableOpacity style={styles.copyBtn} onPress={handleCopy} activeOpacity={0.7}>
        <Ionicons
          name={copied ? "checkmark" : "copy-outline"}
          size={14}
          color={copied ? colors.accent : colors.sub}
        />
        <Text style={[styles.copyBtnText, copied && { color: colors.accent }]}>
          {copied ? "Kopyalandı" : "Kelimeleri kopyala"}
        </Text>
      </TouchableOpacity>

      {/* Onay kutusu */}
      <TouchableOpacity
        style={styles.checkRow}
        onPress={() => setChecked((v) => !v)}
        activeOpacity={0.7}
        accessibilityRole="checkbox"
        accessibilityState={{ checked }}
      >
        <View style={[styles.checkbox, checked && styles.checkboxChecked]}>
          {checked && <Ionicons name="checkmark" size={14} color={colors.void} />}
        </View>
        <Text style={styles.checkLabel}>
          12 kelimeyi güvenli bir yere not aldım. Bu ekranı kapattıktan sonra
          bu kelimelere erişemeyeceğimi anlıyorum.
        </Text>
      </TouchableOpacity>

      {/* İleri */}
      <TouchableOpacity
        style={[styles.primaryBtn, !checked && styles.primaryBtnDisabled]}
        onPress={onConfirm}
        disabled={!checked}
        activeOpacity={0.85}
      >
        <Text style={styles.primaryBtnText}>Devam et</Text>
        <Ionicons name="arrow-forward" size={18} color={colors.void} />
      </TouchableOpacity>
    </ScrollView>
  );
}

// ─── Stage: Doğrula ───────────────────────────────────────────────────────────

function VerifyStage({
  mnemonic,
  onVerified,
}: {
  mnemonic: string;
  onVerified: () => void;
}) {
  const [slots] = useState<VerifySlot[]>(() =>
    pickVerificationWords(mnemonic).map(({ index, word }) => ({
      index,
      word,
      value: "",
      status: "idle" as const,
    }))
  );
  const [values, setValues] = useState<string[]>(slots.map(() => ""));
  const [errors, setErrors] = useState<boolean[]>(slots.map(() => false));
  const inputRefs = useRef<(TextInput | null)[]>([]);

  const handleVerify = useCallback(async () => {
    const newErrors = slots.map((s, i) => values[i].trim() !== s.word);
    setErrors(newErrors);

    if (newErrors.every((e) => !e)) {
      await markMnemonicBackedUp();
      onVerified();
    } else {
      Alert.alert(
        "Yanlış Kelimeler",
        "Bazı kelimeler hatalı. Lütfen tekrar kontrol edin.",
        [{ text: "Tamam" }]
      );
    }
  }, [slots, values, onVerified]);

  return (
    <ScrollView
      style={styles.stageScroll}
      contentContainerStyle={styles.stageContent}
      keyboardShouldPersistTaps="handled"
      showsVerticalScrollIndicator={false}
    >
      <Text style={styles.stageTitle}>Doğrulama</Text>
      <Text style={styles.stageSub}>
        Not aldığınızı doğrulamak için aşağıdaki kelimeleri girin.
      </Text>

      {slots.map((slot, i) => (
        <View key={slot.index} style={styles.verifyField}>
          <Text style={styles.verifyLabel}>{slot.index + 1}. kelime</Text>
          <TextInput
            ref={(el) => { inputRefs.current[i] = el; }}
            style={[
              styles.verifyInput,
              errors[i] && styles.verifyInputError,
              !errors[i] && values[i] && values[i].trim() === slot.word && styles.verifyInputOk,
            ]}
            value={values[i]}
            onChangeText={(v) => {
              const clean = v.toLowerCase().replace(/[^a-z]/g, "");
              const next = [...values];
              next[i] = clean;
              setValues(next);
              const nextErrors = [...errors];
              nextErrors[i] = false;
              setErrors(nextErrors);
            }}
            placeholder={`${slot.index + 1}. kelimeyi girin`}
            placeholderTextColor={colors.dim}
            autoCapitalize="none"
            autoCorrect={false}
            returnKeyType={i < slots.length - 1 ? "next" : "done"}
            onSubmitEditing={() => {
              if (i < slots.length - 1) inputRefs.current[i + 1]?.focus();
              else handleVerify();
            }}
          />
          {errors[i] && (
            <Text style={styles.fieldError}>Yanlış kelime</Text>
          )}
        </View>
      ))}

      <TouchableOpacity
        style={[
          styles.primaryBtn,
          values.some((v) => !v.trim()) && styles.primaryBtnDisabled,
        ]}
        onPress={handleVerify}
        disabled={values.some((v) => !v.trim())}
        activeOpacity={0.85}
      >
        <Text style={styles.primaryBtnText}>Doğrula</Text>
      </TouchableOpacity>
    </ScrollView>
  );
}

// ─── Stage: Tamamlandı ────────────────────────────────────────────────────────

function DoneStage({ onComplete }: { onComplete: () => void }) {
  useEffect(() => {
    const t = setTimeout(onComplete, 1800);
    return () => clearTimeout(t);
  }, [onComplete]);

  return (
    <View style={styles.doneContainer}>
      <View style={styles.doneCircle}>
        <Ionicons name="shield-checkmark" size={36} color={colors.accent} />
      </View>
      <Text style={styles.doneTitle}>Yedeğiniz Güvende</Text>
      <Text style={styles.doneSub}>
        Hesabınız artık kurtarılabilir. Yönlendiriliyor…
      </Text>
      <ActivityIndicator color={colors.accent} style={{ marginTop: spacing.lg }} />
    </View>
  );
}

// ─── Ana Component ────────────────────────────────────────────────────────────

export default function MnemonicBackup({ mnemonic, onComplete }: Props) {
  const [stage, setStage] = useState<Stage>("show");

  // Adım göstergesi barı
  const stages: Stage[] = ["show", "verify", "done"];
  const currentIdx = stages.indexOf(stage);

  return (
    <View style={styles.container}>
      {/* Başlık bar */}
      <View style={styles.topBar}>
        <Text style={styles.topBarTitle}>Kurtarma İfadesi</Text>
        <View style={styles.stepDots}>
          {stages.map((s, i) => (
            <View
              key={s}
              style={[
                styles.stepDot,
                i === currentIdx && styles.stepDotActive,
                i < currentIdx && styles.stepDotPast,
              ]}
            />
          ))}
        </View>
      </View>

      {/* Stage içeriği */}
      {stage === "show" && (
        <ShowStage mnemonic={mnemonic} onConfirm={() => setStage("verify")} />
      )}
      {stage === "verify" && (
        <VerifyStage mnemonic={mnemonic} onVerified={() => setStage("done")} />
      )}
      {stage === "done" && <DoneStage onComplete={onComplete} />}
    </View>
  );
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.void,
  },
  topBar: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm + 2,
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
  },
  topBarTitle: {
    fontSize: typography.base,
    fontWeight: "600",
    color: colors.head,
  },
  stepDots: {
    flexDirection: "row",
    gap: 6,
    alignItems: "center",
  },
  stepDot: {
    width: 6,
    height: 6,
    borderRadius: 3,
    backgroundColor: colors.muted,
  },
  stepDotActive: {
    width: 18,
    backgroundColor: colors.accent,
  },
  stepDotPast: {
    backgroundColor: colors.accentDim,
  },

  // Stage scroll
  stageScroll: { flex: 1 },
  stageContent: {
    padding: spacing.md,
    gap: spacing.md,
    paddingBottom: spacing.xxl,
  },
  iconCircle: {
    width: 56,
    height: 56,
    borderRadius: 28,
    backgroundColor: colors.accentDeep,
    borderWidth: 1,
    borderColor: colors.accentDim + "66",
    alignItems: "center",
    justifyContent: "center",
    alignSelf: "center",
    marginBottom: spacing.xs,
  },
  stageTitle: {
    fontSize: typography.lg,
    fontWeight: "600",
    color: colors.head,
    textAlign: "center",
  },
  stageSub: {
    fontSize: typography.sm,
    color: colors.sub,
    textAlign: "center",
    lineHeight: 20,
  },

  // Uyarı bandı
  warningBand: {
    flexDirection: "row",
    gap: spacing.xs,
    alignItems: "flex-start",
    backgroundColor: colors.amber + "10",
    borderWidth: 1,
    borderColor: colors.amber + "30",
    borderRadius: radius.md,
    padding: spacing.sm + 2,
  },
  warningText: {
    flex: 1,
    fontSize: typography.xs,
    color: colors.amber,
    lineHeight: 17,
  },

  // Kelime grid
  wordGrid: { gap: spacing.xs },
  wordRow: { flexDirection: "row", gap: spacing.xs },
  wordCellWrap: { flex: 1 },
  wordCell: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.xs,
    backgroundColor: colors.raised,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: radius.md,
    paddingVertical: spacing.xs + 2,
    paddingHorizontal: spacing.sm,
  },
  wordIndex: {
    fontSize: typography.xs,
    color: colors.dim,
    width: 16,
    textAlign: "right",
    fontVariant: ["tabular-nums"],
  },
  wordText: {
    fontSize: typography.sm,
    color: colors.head,
    fontWeight: "500",
  },

  // Kopyala butonu
  copyBtn: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: spacing.xs,
    paddingVertical: spacing.sm + 2,
    borderRadius: radius.xl,
    backgroundColor: colors.raised,
    borderWidth: 1,
    borderColor: colors.border,
  },
  copyBtnText: {
    fontSize: typography.xs,
    color: colors.sub,
    fontWeight: "500",
  },

  // Onay kutusu
  checkRow: {
    flexDirection: "row",
    alignItems: "flex-start",
    gap: spacing.sm,
  },
  checkbox: {
    width: 20,
    height: 20,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: colors.muted,
    alignItems: "center",
    justifyContent: "center",
    marginTop: 2,
  },
  checkboxChecked: {
    backgroundColor: colors.accent,
    borderColor: colors.accent,
  },
  checkLabel: {
    flex: 1,
    fontSize: typography.xs,
    color: colors.sub,
    lineHeight: 18,
  },

  // Primary buton
  primaryBtn: {
    height: 52,
    borderRadius: radius.full,
    backgroundColor: colors.accent,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 8,
    marginTop: spacing.xs,
  },
  primaryBtnDisabled: { opacity: 0.4 },
  primaryBtnText: {
    color: colors.void,
    fontSize: typography.base,
    fontWeight: "600",
  },

  // Verify fields
  verifyField: { gap: spacing.xs - 2 },
  verifyLabel: { fontSize: typography.xs, color: colors.dim, fontWeight: "500" },
  verifyInput: {
    backgroundColor: colors.raised,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: radius.xl,
    height: 52,
    paddingHorizontal: spacing.md,
    color: colors.body,
    fontSize: typography.base,
  },
  verifyInputError: {
    borderColor: colors.red + "88",
    backgroundColor: colors.red + "08",
  },
  verifyInputOk: {
    borderColor: colors.accent + "66",
    backgroundColor: colors.accentDeep,
  },
  fieldError: { fontSize: typography.xs, color: colors.red },

  // Done
  doneContainer: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    padding: spacing.xl,
  },
  doneCircle: {
    width: 72,
    height: 72,
    borderRadius: 36,
    backgroundColor: colors.accentDeep,
    borderWidth: 1,
    borderColor: colors.accentDim + "66",
    alignItems: "center",
    justifyContent: "center",
    marginBottom: spacing.lg,
  },
  doneTitle: {
    fontSize: typography.xl,
    fontWeight: "700",
    color: colors.head,
    textAlign: "center",
    marginBottom: spacing.xs,
  },
  doneSub: {
    fontSize: typography.sm,
    color: colors.sub,
    textAlign: "center",
    lineHeight: 20,
  },
});
