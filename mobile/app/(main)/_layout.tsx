import { useEffect } from "react";
import { Stack, router } from "expo-router";
import * as SecureStore from "expo-secure-store";

export default function MainLayout() {
  useEffect(() => {
    SecureStore.getItemAsync("obscura_token").then((t) => {
      if (!t) router.replace("/(auth)/login");
    });
  }, []);

  return (
    <Stack screenOptions={{ headerShown: false }}>
      <Stack.Screen name="(tabs)" />
      <Stack.Screen name="chat/[id]" />
      <Stack.Screen name="new-chat" />
      <Stack.Screen name="profile" />
      <Stack.Screen name="group-profile" />
      <Stack.Screen name="user-profile" />
      <Stack.Screen name="contacts" />
      <Stack.Screen name="discover" />
      <Stack.Screen name="settings-privacy" />
      <Stack.Screen name="settings-notifications" />
      <Stack.Screen name="settings-appearance" />
      <Stack.Screen name="settings-labs" />
      <Stack.Screen name="settings-about" />
      <Stack.Screen name="settings-advanced" />
      <Stack.Screen name="settings-developer" />
      <Stack.Screen name="settings-recovery" />
      <Stack.Screen name="settings-cross-signing" />
      <Stack.Screen name="airdrop" />
      <Stack.Screen name="bots" />
      <Stack.Screen name="bots-new" />
      <Stack.Screen name="dev-tools" />
      <Stack.Screen name="new-group" />
      <Stack.Screen name="new-channel" />
      <Stack.Screen name="new-community" />
      <Stack.Screen name="privacy" />
      <Stack.Screen name="terms" />
      <Stack.Screen name="wallet-shielded" />
      <Stack.Screen name="apps-my" />
      <Stack.Screen name="apps-api-connect" />
      <Stack.Screen name="apps-new" />
      <Stack.Screen name="event/[id]" />
      <Stack.Screen name="proposal/[id]" />
      <Stack.Screen name="voice-call" />
      <Stack.Screen name="video-call" />
      <Stack.Screen name="incoming-call" />
      <Stack.Screen name="staking" />
      <Stack.Screen name="nodes" />
      <Stack.Screen name="admin-review" />
      <Stack.Screen name="bridge" />
      <Stack.Screen name="dao" />
      <Stack.Screen name="sequencer" />
      <Stack.Screen name="nfc" />
      <Stack.Screen name="location" />
      <Stack.Screen name="events" />
    </Stack>
  );
}
