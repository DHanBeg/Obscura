import React, { useState, useRef, useCallback, useEffect } from "react";
import {
  View, Text, Pressable, Animated, Image, StyleSheet,
  Dimensions, TouchableOpacity,
} from "react-native";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors } from "@/lib/theme";
import { useStore } from "@/lib/store";

const W = Dimensions.get("window").width;
const ORB = 60;
const PILL_H = 60;

type NavItem = {
  id: string;
  icon: React.ComponentProps<typeof Ionicons>["name"];
  route: string;
  label: string;
};

const NAV: NavItem[] = [
  { id: "chats",      icon: "chatbubble-ellipses-outline", route: "/(main)/chats",      label: "Sohbetler"   },
  { id: "calls",      icon: "call-outline",                route: "/(main)/calls",      label: "Aramalar"    },
  { id: "wallet",     icon: "wallet-outline",              route: "/(main)/wallet",     label: "Cüzdan"      },
  { id: "governance", icon: "business-outline",            route: "/(main)/governance", label: "Yönetim"     },
  { id: "apps",       icon: "layers-outline",              route: "/(main)/apps",       label: "Uygulamalar" },
  { id: "settings",   icon: "settings-outline",            route: "/(main)/settings",   label: "Ayarlar"     },
];

const HIDE_ROUTES = new Set([
  "chat/[id]", "new-chat",
  "new-group", "new-channel", "new-community",
  "user-profile", "contacts",
  "voice-call", "video-call", "incoming-call",
  "airdrop", "profile", "settings-privacy", "settings-notifications",
  "settings-appearance", "settings-labs", "settings-about",
  "settings-advanced", "settings-developer", "settings-recovery",
  "settings-cross-signing", "bots-new", "apps-new", "apps-api-connect",
  "wallet-shielded", "event/[id]", "proposal/[id]",
]);

const FAB_ACTIONS = [
  { icon: "chatbubble-outline" as const, label: "Yeni Sohbet", route: "/(main)/new-chat" },
  { icon: "people-outline" as const, label: "Yeni Grup", route: "/(main)/new-group" },
  { icon: "megaphone-outline" as const, label: "Yeni Kanal", route: "/(main)/new-channel" },
  { icon: "earth-outline" as const, label: "Yeni Topluluk", route: "/(main)/new-community" },
];

export function GravityWell({ state, navigation }: any) {
  const insets = useSafeAreaInsets();
  const [expanded, setExpanded] = useState(false);
  const [fabMenuOpen, setFabMenuOpen] = useState(false);
  const widthAnim = useRef(new Animated.Value(ORB)).current;
  const opacityAnim = useRef(new Animated.Value(0)).current;
  const fabMenuAnim = useRef(new Animated.Value(0)).current;
  const conversations = useStore((s) => s.conversations);
  const totalUnread = conversations.reduce((a, c) => a + c.unread_count, 0);

  const currentRoute: string = state?.routes?.[state?.index]?.name ?? "";
  const isHidden = HIDE_ROUTES.has(currentRoute);
  const activeId = NAV.find((n) => n.id === currentRoute)?.id ?? "chats";

  useEffect(() => {
    expand(false);
    closeFabMenu();
  }, [currentRoute]);

  const closeFabMenu = useCallback(() => {
    setFabMenuOpen(false);
    Animated.timing(fabMenuAnim, { toValue: 0, duration: 150, useNativeDriver: true }).start();
  }, [fabMenuAnim]);

  const toggleFabMenu = useCallback(() => {
    const opening = !fabMenuOpen;
    setFabMenuOpen(opening);
    Animated.spring(fabMenuAnim, {
      toValue: opening ? 1 : 0,
      useNativeDriver: true,
      tension: 120,
      friction: 12,
    }).start();
  }, [fabMenuOpen, fabMenuAnim]);

  const expand = useCallback((open: boolean) => {
    setExpanded(open);
    Animated.parallel([
      Animated.spring(widthAnim, {
        toValue: open ? Math.min(W - 32, 360) : ORB,
        useNativeDriver: false,
        overshootClamping: true,
        tension: 130,
        friction: 15,
      }),
      Animated.timing(opacityAnim, {
        toValue: open ? 1 : 0,
        duration: 180,
        useNativeDriver: false,
      }),
    ]).start();
  }, [widthAnim, opacityAnim]);

  const bottom = insets.bottom + 16;

  if (isHidden) return <View style={{ height: insets.bottom }} />;

  return (
    <View style={[styles.wrap, { height: PILL_H + bottom }]} pointerEvents="box-none">
      {/* Pill */}
      <Animated.View style={[styles.pill, { width: widthAnim, bottom }]}>
        {/* Orb */}
        <Pressable
          style={styles.orb}
          onPress={() => expand(!expanded)}
          accessibilityLabel="Navigasyon menüsü"
          accessibilityRole="button"
        >
          <Image source={require("@/assets/logo.jpeg")} style={styles.orbImg} />
          {totalUnread > 0 && !expanded && <View style={styles.unreadDot} />}
        </Pressable>

        {/* Expanded nav */}
        {expanded && (
          <Animated.View style={[styles.navWrap, { opacity: opacityAnim }]}>
            <View style={styles.divider} />
            {NAV.map((item) => {
              const active = item.id === activeId;
              return (
                <TouchableOpacity
                  key={item.id}
                  style={[styles.navBtn, active && styles.navBtnActive]}
                  onPress={() => {
                    navigation?.navigate(item.id);
                    expand(false);
                  }}
                  activeOpacity={0.7}
                  accessibilityLabel={item.label}
                >
                  <Ionicons
                    name={item.icon}
                    size={20}
                    color={active ? colors.accent : "rgba(232,232,240,0.5)"}
                  />
                  {item.id === "chats" && totalUnread > 0 && (
                    <View style={styles.navDot} />
                  )}
                </TouchableOpacity>
              );
            })}
          </Animated.View>
        )}
      </Animated.View>

      {/* FAB menu items */}
      {!expanded && activeId === "chats" && fabMenuOpen && (
        <View style={[styles.fabMenuWrap, { bottom: bottom + 52 }]} pointerEvents="box-none">
          {FAB_ACTIONS.map((action, i) => {
            const translateY = fabMenuAnim.interpolate({
              inputRange: [0, 1],
              outputRange: [20, 0],
            });
            const opacity = fabMenuAnim.interpolate({
              inputRange: [0, 0.5, 1],
              outputRange: [0, 0, 1],
            });
            return (
              <Animated.View
                key={action.route}
                style={{ transform: [{ translateY: Animated.multiply(translateY, new Animated.Value(i + 1)) }], opacity }}
              >
                <TouchableOpacity
                  style={styles.fabMenuItem}
                  onPress={() => {
                    closeFabMenu();
                    router.push(action.route as any);
                  }}
                  activeOpacity={0.8}
                >
                  <Text style={styles.fabMenuLabel}>{action.label}</Text>
                  <View style={styles.fabMenuIcon}>
                    <Ionicons name={action.icon} size={18} color={colors.accent} />
                  </View>
                </TouchableOpacity>
              </Animated.View>
            );
          })}
        </View>
      )}

      {/* FAB — backdrop tap to close */}
      {!expanded && activeId === "chats" && fabMenuOpen && (
        <TouchableOpacity
          style={StyleSheet.absoluteFillObject}
          onPress={closeFabMenu}
          activeOpacity={1}
        />
      )}

      {/* FAB button */}
      {!expanded && activeId === "chats" && (
        <TouchableOpacity
          style={[styles.fab, { bottom }, fabMenuOpen && styles.fabActive]}
          onPress={toggleFabMenu}
          activeOpacity={0.85}
          accessibilityLabel="Yeni oluştur"
        >
          <Animated.View style={{
            transform: [{
              rotate: fabMenuAnim.interpolate({ inputRange: [0, 1], outputRange: ["0deg", "45deg"] }),
            }],
          }}>
            <Ionicons name="add" size={20} color={fabMenuOpen ? colors.void : colors.accent} />
          </Animated.View>
        </TouchableOpacity>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: {
    position: "relative",
    alignItems: "center",
    backgroundColor: colors.void,
  },
  pill: {
    position: "absolute",
    height: PILL_H,
    borderRadius: PILL_H / 2,
    flexDirection: "row",
    alignItems: "center",
    backgroundColor: "rgba(18,18,28,0.95)",
    borderWidth: 1,
    borderColor: "rgba(74,222,128,0.18)",
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 8 },
    shadowOpacity: 0.75,
    shadowRadius: 24,
    elevation: 14,
    overflow: "hidden",
  },
  orb: {
    width: ORB,
    height: PILL_H,
    alignItems: "center",
    justifyContent: "center",
    flexShrink: 0,
  },
  orbImg: {
    width: 32,
    height: 32,
    borderRadius: 16,
    borderWidth: 1.5,
    borderColor: "rgba(74,222,128,0.25)",
  },
  unreadDot: {
    position: "absolute",
    top: 13, right: 13,
    width: 9, height: 9,
    borderRadius: 5,
    backgroundColor: colors.accent,
    borderWidth: 2,
    borderColor: "#0d0d14",
  },
  navWrap: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    paddingRight: 6,
  },
  divider: {
    width: 1,
    height: 24,
    backgroundColor: "rgba(255,255,255,0.1)",
    marginHorizontal: 4,
    flexShrink: 0,
  },
  navBtn: {
    flex: 1,
    height: 40,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: 12,
    position: "relative",
  },
  navBtnActive: {
    backgroundColor: "rgba(74,222,128,0.1)",
  },
  navDot: {
    position: "absolute",
    top: 7, right: 7,
    width: 7, height: 7,
    borderRadius: 4,
    backgroundColor: colors.accent,
    borderWidth: 1.5,
    borderColor: "#0d0d14",
  },
  fab: {
    position: "absolute",
    right: 16,
    width: 44, height: 44,
    borderRadius: 22,
    backgroundColor: "rgba(74,222,128,0.08)",
    borderWidth: 1,
    borderColor: "rgba(74,222,128,0.2)",
    alignItems: "center",
    justifyContent: "center",
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.4,
    shadowRadius: 8,
    elevation: 6,
  },
  fabActive: {
    backgroundColor: colors.accent,
    borderColor: colors.accent,
  },
  fabMenuWrap: {
    position: "absolute",
    right: 16,
    gap: 8,
    alignItems: "flex-end",
  },
  fabMenuItem: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
  },
  fabMenuLabel: {
    fontSize: 13,
    fontWeight: "600",
    color: colors.head,
    backgroundColor: "rgba(18,18,28,0.92)",
    paddingHorizontal: 12,
    paddingVertical: 6,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: "rgba(74,222,128,0.15)",
    overflow: "hidden",
  },
  fabMenuIcon: {
    width: 40,
    height: 40,
    borderRadius: 20,
    backgroundColor: "rgba(18,18,28,0.95)",
    borderWidth: 1,
    borderColor: "rgba(74,222,128,0.2)",
    alignItems: "center",
    justifyContent: "center",
  },
});
