import React, { useState, useRef, useCallback } from "react";
import {
  View, Text, TextInput, TouchableOpacity, StyleSheet,
  KeyboardAvoidingView, Platform, ActivityIndicator,
  Animated, Easing,
} from "react-native";
import { router } from "expo-router";
import * as SecureStore from "expo-secure-store";
import * as Haptics from "expo-haptics";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";

const OTP_LENGTH = 6;
type Step = "phone" | "otp" | "username";

export default function LoginScreen() {
  const [step, setStep] = useState<Step>("phone");
  const [phone, setPhone] = useState("+90");
  const [otp, setOtp] = useState<string[]>(Array(OTP_LENGTH).fill(""));
  const [username, setUsername] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [countdown, setCountdown] = useState(0);
  const otpRefs = useRef<(TextInput | null)[]>([]);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const slideAnim = useRef(new Animated.Value(0)).current;

  const slideIn = () => {
    slideAnim.setValue(30);
    Animated.timing(slideAnim, {
      toValue: 0, duration: 300,
      easing: Easing.out(Easing.cubic),
      useNativeDriver: true,
    }).start();
  };

  const startCountdown = () => {
    setCountdown(60);
    if (timerRef.current) clearInterval(timerRef.current);
    timerRef.current = setInterval(() => {
      setCountdown((c) => { if (c <= 1) { clearInterval(timerRef.current!); return 0; } return c - 1; });
    }, 1000);
  };

  const sendOTP = useCallback(async () => {
    if (phone.length < 10) { setError("Geçerli bir numara girin"); return; }
    setError(""); setLoading(true);
    try {
      await api.requestOTP(phone);
      setStep("otp"); startCountdown(); slideIn();
      setTimeout(() => otpRefs.current[0]?.focus(), 300);
    } catch (e: any) { setError(e.message); }
    finally { setLoading(false); }
  }, [phone]);

  const verifyOTP = useCallback(async (code?: string, uname?: string) => {
    const otpCode = code ?? otp.join("");
    if (otpCode.length !== OTP_LENGTH) { setError("Kodu eksiksiz girin"); return; }
    setError(""); setLoading(true);
    try {
      const identityKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
      const data = await api.verifyOTP({
        phone, otp: otpCode,
        username: uname,
        identity_key: identityKey,
      });
      if (data.is_new && !uname) {
        setStep("username"); slideIn(); setLoading(false); return;
      }
      await SecureStore.setItemAsync("obscura_token", data.token);
      await Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      router.replace("/(main)/chats");
    } catch (e: any) {
      setError(e.message);
      await Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
      setOtp(Array(OTP_LENGTH).fill(""));
      setTimeout(() => otpRefs.current[0]?.focus(), 50);
    } finally { setLoading(false); }
  }, [phone, otp]);

  const handleOTPInput = (idx: number, val: string) => {
    const digit = val.replace(/\D/g, "").slice(-1);
    const next = [...otp]; next[idx] = digit; setOtp(next);
    if (digit && idx < OTP_LENGTH - 1) otpRefs.current[idx + 1]?.focus();
    if (next.every((d) => d) && idx === OTP_LENGTH - 1) {
      setTimeout(() => verifyOTP(next.join("")), 80);
    }
  };

  return (
    <KeyboardAvoidingView
      style={styles.container}
      behavior={Platform.OS === "ios" ? "padding" : "height"}
    >
      {/* Back button */}
      {step !== "phone" && (
        <TouchableOpacity
          style={styles.back}
          onPress={() => { setStep(step === "otp" ? "phone" : "otp"); setError(""); }}
        >
          <Ionicons name="chevron-back" size={24} color={colors.body} />
        </TouchableOpacity>
      )}

      {/* Logo */}
      <View style={styles.logoArea}>
        <View style={styles.logoCircle}>
          <Ionicons name="lock-closed" size={32} color={colors.accent} />
        </View>
        <Text style={styles.appName}>obscura</Text>
        <Text style={styles.tagline}>Zero-Knowledge Messaging</Text>
      </View>

      <Animated.View style={[styles.form, { transform: [{ translateY: slideAnim }] }]}>
        {/* Phone step */}
        {step === "phone" && (
          <>
            <Text style={styles.title}>Hoş geldiniz</Text>
            <Text style={styles.subtitle}>Numara yalnızca doğrulama için kullanılır</Text>
            <TextInput
              style={styles.input}
              value={phone}
              onChangeText={(v) => { setError(""); setPhone(v.startsWith("+") ? v : "+" + v); }}
              placeholder="+90 555 000 00 00"
              placeholderTextColor={colors.dim}
              keyboardType="phone-pad"
              returnKeyType="go"
              onSubmitEditing={sendOTP}
              autoFocus
            />
            {error ? <Text style={styles.error}>{error}</Text> : null}
            <TouchableOpacity
              style={[styles.btn, (loading || phone.length < 10) && styles.btnDisabled]}
              onPress={sendOTP}
              disabled={loading || phone.length < 10}
            >
              {loading
                ? <ActivityIndicator color={colors.void} />
                : <><Text style={styles.btnText}>Kod gönder</Text><Ionicons name="arrow-forward" size={18} color={colors.void} /></>
              }
            </TouchableOpacity>
            <View style={styles.hint}>
              <Ionicons name="shield-checkmark" size={14} color={colors.dim} />
              <Text style={styles.hintText}>Uçtan uca şifreli iletişim</Text>
            </View>
          </>
        )}

        {/* OTP step */}
        {step === "otp" && (
          <>
            <Text style={styles.title}>Kodu girin</Text>
            <Text style={styles.subtitle}>{phone} numarasına gönderildi</Text>
            <View style={styles.otpRow}>
              {otp.map((digit, idx) => (
                <TextInput
                  key={idx}
                  ref={(el) => { otpRefs.current[idx] = el; }}
                  style={[styles.otpBox, digit ? styles.otpBoxFilled : null]}
                  value={digit}
                  onChangeText={(v) => handleOTPInput(idx, v)}
                  onKeyPress={({ nativeEvent }) => {
                    if (nativeEvent.key === "Backspace" && !otp[idx] && idx > 0) {
                      otpRefs.current[idx - 1]?.focus();
                    }
                  }}
                  keyboardType="number-pad"
                  maxLength={1}
                  textAlign="center"
                  selectTextOnFocus
                />
              ))}
            </View>
            {error ? <Text style={styles.error}>{error}</Text> : null}
            <TouchableOpacity
              style={[styles.btn, (loading || otp.some((d) => !d)) && styles.btnDisabled]}
              onPress={() => verifyOTP()}
              disabled={loading || otp.some((d) => !d)}
            >
              {loading ? <ActivityIndicator color={colors.void} /> : <Text style={styles.btnText}>Doğrula</Text>}
            </TouchableOpacity>
            <View style={styles.resendRow}>
              {countdown > 0
                ? <Text style={styles.hintText}>Tekrar gönder: <Text style={{ color: colors.sub }}>{countdown}s</Text></Text>
                : <TouchableOpacity onPress={sendOTP}><Text style={{ color: colors.accent, fontSize: typography.sm }}>Kodu tekrar gönder</Text></TouchableOpacity>
              }
            </View>
          </>
        )}

        {/* Username step */}
        {step === "username" && (
          <>
            <Text style={[styles.title, { textAlign: "center" }]}>👋</Text>
            <Text style={styles.title}>Kullanıcı adı seç</Text>
            <Text style={styles.subtitle}>Başkalarının sizi bulacağı isim</Text>
            <View style={styles.inputRow}>
              <Text style={styles.atSign}>@</Text>
              <TextInput
                style={[styles.input, styles.inputWithAt]}
                value={username}
                onChangeText={(v) => { setError(""); setUsername(v.toLowerCase().replace(/[^a-z0-9_.]/g, "")); }}
                placeholder="kullanici_adi"
                placeholderTextColor={colors.dim}
                autoFocus
                autoCapitalize="none"
                returnKeyType="done"
                onSubmitEditing={() => username.length >= 3 && verifyOTP(undefined, username)}
              />
            </View>
            {error ? <Text style={styles.error}>{error}</Text> : null}
            <TouchableOpacity
              style={[styles.btn, (loading || username.length < 3) && styles.btnDisabled]}
              onPress={() => verifyOTP(undefined, username)}
              disabled={loading || username.length < 3}
            >
              {loading ? <ActivityIndicator color={colors.void} /> : <Text style={styles.btnText}>Hesabı oluştur →</Text>}
            </TouchableOpacity>
          </>
        )}
      </Animated.View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.void,
    paddingHorizontal: spacing.lg,
    justifyContent: "center",
  },
  back: {
    position: "absolute",
    top: 60,
    left: spacing.md,
    width: 40, height: 40,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: radius.lg,
    backgroundColor: colors.raised,
  },
  logoArea: { alignItems: "center", marginBottom: spacing.xxl },
  logoCircle: {
    width: 72, height: 72, borderRadius: 36,
    backgroundColor: colors.accentDeep,
    borderWidth: 1, borderColor: colors.accentDim + "44",
    alignItems: "center", justifyContent: "center",
    marginBottom: spacing.md,
  },
  appName: { fontSize: typography.xxl, fontWeight: "700", color: colors.head, letterSpacing: -0.5 },
  tagline: { fontSize: typography.xs, color: colors.dim, letterSpacing: 3, textTransform: "uppercase", marginTop: 4 },
  form: { gap: spacing.sm },
  title: { fontSize: typography.xl, fontWeight: "600", color: colors.head, marginBottom: 4 },
  subtitle: { fontSize: typography.sm, color: colors.sub, marginBottom: spacing.md },
  input: {
    backgroundColor: colors.raised,
    borderWidth: 1, borderColor: colors.border,
    borderRadius: radius.xl,
    height: 52, paddingHorizontal: spacing.md,
    color: colors.body, fontSize: typography.base,
  },
  inputRow: { flexDirection: "row", alignItems: "center" },
  atSign: { color: colors.dim, fontSize: typography.base, paddingLeft: spacing.md, position: "absolute", zIndex: 1 },
  inputWithAt: { flex: 1, paddingLeft: 36 },
  error: { color: colors.red, fontSize: typography.xs, textAlign: "center" },
  btn: {
    height: 52, borderRadius: radius.full,
    backgroundColor: colors.accent,
    flexDirection: "row", alignItems: "center", justifyContent: "center",
    gap: 8, marginTop: spacing.sm,
  },
  btnDisabled: { opacity: 0.4 },
  btnText: { color: colors.void, fontSize: typography.base, fontWeight: "600" },
  hint: { flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 6, marginTop: spacing.md },
  hintText: { color: colors.dim, fontSize: typography.xs },
  otpRow: { flexDirection: "row", gap: 10, justifyContent: "center" },
  otpBox: {
    width: 48, height: 56, borderRadius: radius.lg,
    backgroundColor: colors.raised,
    borderWidth: 1, borderColor: colors.border,
    color: colors.head, fontSize: typography.xl, fontWeight: "600",
    textAlign: "center",
  },
  otpBoxFilled: { borderColor: colors.accent + "80", backgroundColor: colors.accentDeep },
  resendRow: { alignItems: "center", marginTop: spacing.sm },
});
