import { useEffect } from "react";
import { Stack } from "expo-router";
import * as SplashScreen from "expo-splash-screen";
import * as SecureStore from "expo-secure-store";
import { router } from "expo-router";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import { StatusBar } from "expo-status-bar";
import { api, createWS } from "@/lib/api";
import { useStore } from "@/lib/store";

SplashScreen.preventAutoHideAsync();

export default function RootLayout() {
  const { setUser, setConversations, addMessage, updateMsgStatus, setOnline, setWS } = useStore();

  useEffect(() => {
    (async () => {
      try {
        const token = await SecureStore.getItemAsync("obscura_token");
        if (!token) {
          router.replace("/(auth)/login");
          return;
        }
        const [me, convs] = await Promise.all([api.getMe(), api.getConversations()]);
        setUser(me);
        setConversations(convs || []);
        // WebSocket
        const ws = createWS(token, (msg) => {
          switch (msg.type) {
            case "new_message": addMessage(msg.data); break;
            case "message_delivered": updateMsgStatus(msg.data.msg_id, "delivered"); break;
            case "message_read": updateMsgStatus(msg.data.msg_id, "read"); break;
            case "user_online": setOnline(msg.data.did, true); break;
            case "user_offline": setOnline(msg.data.did, false); break;
          }
        });
        setWS(ws);
        router.replace("/(main)/chats");
      } catch {
        router.replace("/(auth)/login");
      } finally {
        SplashScreen.hideAsync();
      }
    })();
  }, []);

  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <StatusBar style="light" />
      <Stack screenOptions={{ headerShown: false }} />
    </GestureHandlerRootView>
  );
}
