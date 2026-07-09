import { Tabs } from "expo-router";
import { GravityWell } from "@/components/GravityWell";

export default function TabsLayout() {
  return (
    <Tabs
      tabBar={(props) => <GravityWell {...props} />}
      screenOptions={{ headerShown: false, tabBarStyle: { backgroundColor: 'transparent', borderTopWidth: 0, elevation: 0 } }}
    >
      <Tabs.Screen name="chats" />
      <Tabs.Screen name="calls" />
      <Tabs.Screen name="wallet" />
      <Tabs.Screen name="governance" />
      <Tabs.Screen name="apps" />
      <Tabs.Screen name="settings" />
    </Tabs>
  );
}
