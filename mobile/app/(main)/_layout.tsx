import { useEffect } from "react";
import { Tabs, router } from "expo-router";
import * as SecureStore from "expo-secure-store";
import { GravityWell } from "@/components/GravityWell";

export default function MainLayout() {
  useEffect(() => {
    SecureStore.getItemAsync("obscura_token").then((t) => {
      if (!t) router.replace("/(auth)/login");
    });
  }, []);

  return (
    <Tabs
      tabBar={(props) => <GravityWell {...props} />}
      screenOptions={{ headerShown: false, tabBarStyle: { backgroundColor: 'transparent', borderTopWidth: 0, elevation: 0 } }}
    >
      <Tabs.Screen name="chats" />
      <Tabs.Screen name="calls" />
      <Tabs.Screen name="wallet" />
      <Tabs.Screen name="governance" />
      <Tabs.Screen name="staking" />
      <Tabs.Screen name="apps" />
      <Tabs.Screen name="nodes" />
      <Tabs.Screen name="bridge" />
      <Tabs.Screen name="dao" />
      <Tabs.Screen name="sequencer" />
      <Tabs.Screen name="nfc" />
      <Tabs.Screen name="location" />
      <Tabs.Screen name="settings" />
      <Tabs.Screen name="events" />
      <Tabs.Screen name="chat/[id]" options={{ href: null }} />
      <Tabs.Screen name="new-chat" options={{ href: null }} />
      <Tabs.Screen name="profile" options={{ href: null }} />
      <Tabs.Screen name="settings-privacy" options={{ href: null }} />
      <Tabs.Screen name="settings-notifications" options={{ href: null }} />
      <Tabs.Screen name="settings-appearance" options={{ href: null }} />
      <Tabs.Screen name="settings-labs" options={{ href: null }} />
      <Tabs.Screen name="settings-about" options={{ href: null }} />
      <Tabs.Screen name="settings-advanced" options={{ href: null }} />
      <Tabs.Screen name="settings-developer" options={{ href: null }} />
      <Tabs.Screen name="settings-recovery" options={{ href: null }} />
      <Tabs.Screen name="airdrop" options={{ href: null }} />
      <Tabs.Screen name="bots" options={{ href: null }} />
      <Tabs.Screen name="dev-tools" options={{ href: null }} />
      <Tabs.Screen name="new-group" options={{ href: null }} />
      <Tabs.Screen name="new-channel" options={{ href: null }} />
      <Tabs.Screen name="new-community" options={{ href: null }} />
      <Tabs.Screen name="privacy" options={{ href: null }} />
      <Tabs.Screen name="terms" options={{ href: null }} />
      <Tabs.Screen name="settings-cross-signing" options={{ href: null }} />
      <Tabs.Screen name="wallet-shielded" options={{ href: null }} />
      <Tabs.Screen name="apps-my" options={{ href: null }} />
      <Tabs.Screen name="apps-api-connect" options={{ href: null }} />
      <Tabs.Screen name="apps-new" options={{ href: null }} />
      <Tabs.Screen name="bots-new" options={{ href: null }} />
      <Tabs.Screen name="event/[id]" options={{ href: null }} />
      <Tabs.Screen name="proposal/[id]" options={{ href: null }} />
    </Tabs>
  );
}
