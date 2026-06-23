import React, { useEffect, useRef } from "react";
import {
  View, Text, Image, TouchableOpacity, StyleSheet,
  Animated, Easing, Dimensions, ScrollView,
} from "react-native";
import { router } from "expo-router";
import * as SecureStore from "expo-secure-store";
import { LinearGradient } from "expo-linear-gradient";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";

const { width } = Dimensions.get("window");

const FEATURES = [
  {
    icon: "shield-checkmark-outline" as const,
    title: "Uçtan Uca Şifreli",
    sub: "X3DH + Double Ratchet aktif",
  },
  {
    icon: "flash-outline" as const,
    title: "Sıfır Bilgi Kanıtı",
    sub: "Kimliğin doğrulanır, paylaşılmaz",
  },
  {
    icon: "globe-outline" as const,
    title: "Merkezi Olmayan",
    sub: "P2P ağ — hiçbir sunucu izlemez",
  },
];

export default function WelcomeScreen() {
  const ring1 = useRef(new Animated.Value(0)).current;
  const ring2 = useRef(new Animated.Value(0)).current;
  const ring3 = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    SecureStore.getItemAsync("obscura_token").then((t) => {
      if (t) router.replace("/(main)/chats");
    });

    const spin = (val: Animated.Value, dur: number, reverse = false) =>
      Animated.loop(
        Animated.timing(val, {
          toValue: reverse ? -1 : 1,
          duration: dur,
          easing: Easing.linear,
          useNativeDriver: true,
        })
      ).start();

    spin(ring1, 22000);
    spin(ring2, 16000, true);
    spin(ring3, 30000);
  }, []);

  const toRotate = (val: Animated.Value) =>
    val.interpolate({ inputRange: [-1, 1], outputRange: ["-360deg", "360deg"] });

  const LOGO_SIZE = 120;
  const CONTAINER = 200;

  return (
    <View style={styles.root}>
      <LinearGradient
        colors={["rgba(0,229,160,0.07)", "transparent"]}
        style={styles.ambientGlow}
        start={{ x: 0.5, y: 0 }}
        end={{ x: 0.5, y: 1 }}
        pointerEvents="none"
      />

      <ScrollView
        contentContainerStyle={styles.scroll}
        showsVerticalScrollIndicator={false}
      >
        {/* Hero */}
        <View style={styles.hero}>
          {/* Logo + rings */}
          <View style={{ width: CONTAINER, height: CONTAINER, alignItems: "center", justifyContent: "center", marginBottom: spacing.lg }}>
            <Animated.View
              style={[styles.ring, { width: 180, height: 180, borderRadius: 90, transform: [{ rotate: toRotate(ring1) }] }]}
            />
            <Animated.View
              style={[styles.ring, { width: 152, height: 152, borderRadius: 76, opacity: 0.7, transform: [{ rotate: toRotate(ring2) }] }]}
            />
            <Animated.View
              style={[styles.ring, { width: 120, height: 120, borderRadius: 60, opacity: 0.4, transform: [{ rotate: toRotate(ring3) }] }]}
            />
            <View style={[styles.logoWrap, { width: LOGO_SIZE, height: LOGO_SIZE, borderRadius: LOGO_SIZE / 2 }]}>
              <Image
                source={require("@/assets/icon.png")}
                style={{ width: LOGO_SIZE, height: LOGO_SIZE, borderRadius: LOGO_SIZE / 2 }}
                resizeMode="cover"
              />
            </View>
          </View>

          <Text style={styles.title}>OBSCURA</Text>
          <Text style={styles.tagline}>Her Adım Gizli</Text>
        </View>

        {/* Features */}
        <View style={styles.featuresWrap}>
          {FEATURES.map(({ icon, title, sub }, i) => (
            <View key={title} style={styles.featureRow}>
              <View style={styles.featureIcon}>
                <Ionicons name={icon} size={18} color={colors.accent} />
              </View>
              <View style={{ flex: 1 }}>
                <Text style={styles.featureTitle}>{title}</Text>
                <Text style={styles.featureSub}>{sub}</Text>
              </View>
            </View>
          ))}
        </View>

        {/* CTA */}
        <View style={styles.ctaWrap}>
          <TouchableOpacity
            style={styles.ctaBtn}
            onPress={() => router.push("/(auth)/login")}
            activeOpacity={0.85}
          >
            <Text style={styles.ctaBtnText}>Başla</Text>
            <Ionicons name="arrow-forward" size={18} color={colors.void} />
          </TouchableOpacity>

          <Text style={styles.tos}>
            Devam ederek{" "}
            <Text style={styles.tosLink}>Gizlilik Politikası</Text>
            {"'nı ve "}
            <Text style={styles.tosLink}>Kullanım Şartları</Text>
            {"'nı kabul etmiş olursunuz."}
          </Text>
        </View>
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  root: {
    flex: 1,
    backgroundColor: colors.void,
  },
  ambientGlow: {
    position: "absolute",
    top: 0,
    left: width / 2 - 200,
    width: 400,
    height: 400,
  },
  scroll: {
    paddingHorizontal: spacing.lg,
    paddingTop: 80,
    paddingBottom: 48,
    alignItems: "center",
  },
  hero: {
    alignItems: "center",
    marginBottom: spacing.xl,
  },
  ring: {
    position: "absolute",
    borderWidth: 1,
    borderColor: "rgba(0,229,160,0.12)",
    borderStyle: "dashed",
  },
  logoWrap: {
    overflow: "hidden",
    borderWidth: 1.5,
    borderColor: "rgba(0,229,160,0.22)",
    shadowColor: colors.accent,
    shadowOffset: { width: 0, height: 0 },
    shadowOpacity: 0.3,
    shadowRadius: 24,
    elevation: 12,
  },
  title: {
    fontFamily: "SpaceGrotesk-Bold",
    fontSize: 42,
    fontWeight: "800",
    color: colors.head,
    letterSpacing: -1.5,
    lineHeight: 46,
    marginBottom: 12,
  },
  tagline: {
    fontSize: 13,
    color: colors.sub,
    letterSpacing: 3,
    textTransform: "uppercase",
  },
  featuresWrap: {
    width: "100%",
    maxWidth: 360,
    gap: 10,
    marginBottom: 40,
  },
  featureRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.md,
    paddingHorizontal: spacing.md,
    paddingVertical: 14,
    backgroundColor: colors.surface,
    borderRadius: radius.xl,
    borderWidth: 1,
    borderColor: colors.border,
  },
  featureIcon: {
    width: 40,
    height: 40,
    borderRadius: radius.lg,
    backgroundColor: "rgba(74,222,128,0.08)",
    alignItems: "center",
    justifyContent: "center",
  },
  featureTitle: {
    fontSize: typography.sm,
    fontWeight: "600",
    color: colors.head,
    marginBottom: 2,
  },
  featureSub: {
    fontSize: 12,
    color: colors.sub,
    lineHeight: 16,
  },
  ctaWrap: {
    width: "100%",
    maxWidth: 360,
    alignItems: "center",
    gap: spacing.md,
  },
  ctaBtn: {
    width: "100%",
    height: 56,
    borderRadius: 18,
    backgroundColor: colors.accent,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 10,
    shadowColor: colors.accent,
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.4,
    shadowRadius: 16,
    elevation: 8,
  },
  ctaBtnText: {
    fontSize: 16,
    fontWeight: "700",
    color: colors.void,
    letterSpacing: -0.3,
  },
  tos: {
    fontSize: 11,
    color: colors.dim,
    textAlign: "center",
    lineHeight: 17,
    maxWidth: 280,
  },
  tosLink: {
    color: colors.sub,
    textDecorationLine: "underline",
  },
});
