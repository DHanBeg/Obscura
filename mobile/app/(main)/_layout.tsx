import { Tabs } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors } from "@/lib/theme";
import { useStore } from "@/lib/store";
import { BlurView } from "expo-blur";
import { Platform, StyleSheet } from "react-native";

export default function MainLayout() {
  const conversations = useStore((s) => s.conversations);
  const totalUnread = conversations.reduce((s, c) => s + c.unread_count, 0);

  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarStyle: styles.tabBar,
        tabBarBackground: () =>
          Platform.OS === "ios" ? (
            <BlurView intensity={80} tint="dark" style={StyleSheet.absoluteFill} />
          ) : null,
        tabBarActiveTintColor: colors.accent,
        tabBarInactiveTintColor: colors.dim,
        tabBarLabelStyle: { fontSize: 10, fontWeight: "600", marginBottom: 2 },
      }}
    >
      <Tabs.Screen
        name="chats"
        options={{
          title: "Sohbetler",
          tabBarIcon: ({ color, size }) => (
            <Ionicons name="chatbubbles" size={size} color={color} />
          ),
          tabBarBadge: totalUnread > 0 ? totalUnread : undefined,
          tabBarBadgeStyle: { backgroundColor: colors.accent, color: colors.void, fontSize: 10 },
        }}
      />
      <Tabs.Screen
        name="calls"
        options={{
          title: "Aramalar",
          tabBarIcon: ({ color, size }) => (
            <Ionicons name="call" size={size} color={color} />
          ),
        }}
      />
      <Tabs.Screen
        name="settings"
        options={{
          title: "Ayarlar",
          tabBarIcon: ({ color, size }) => (
            <Ionicons name="settings" size={size} color={color} />
          ),
        }}
      />
      {/* Hidden screens */}
      <Tabs.Screen name="chat/[id]" options={{ href: null }} />
      <Tabs.Screen name="new-chat" options={{ href: null }} />
      <Tabs.Screen name="call" options={{ href: null }} />
    </Tabs>
  );
}

const styles = StyleSheet.create({
  tabBar: {
    position: "absolute",
    backgroundColor: Platform.OS === "ios" ? "transparent" : "rgba(8,8,8,0.95)",
    borderTopWidth: 1,
    borderTopColor: "rgba(31,31,31,0.8)",
  },
});
