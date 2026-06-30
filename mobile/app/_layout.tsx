import "react-native-get-random-values";
import { useEffect } from "react";
import { Stack } from "expo-router";
import * as SplashScreen from "expo-splash-screen";
import * as SecureStore from "expo-secure-store";
import { router } from "expo-router";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import { StatusBar } from "expo-status-bar";
import { Platform, Alert } from "react-native";
import * as Linking from "expo-linking";
import * as Notifications from "expo-notifications";
import { api, createWS } from "@/lib/api";
import { useStore } from "@/lib/store";
import * as rtcSession from "@/lib/rtcSession";

async function registerPushToken() {
  try {
    const { status } = await Notifications.getPermissionsAsync();
    if (status !== "granted") return;
    const token = await Notifications.getExpoPushTokenAsync();
    const platform = Platform.OS;
    await api.registerDevice(platform, token.data);
  } catch {}
}

SplashScreen.preventAutoHideAsync();

export default function RootLayout() {
  const {
    setUser, setConversations, addMessage, updateMsgStatus, setOnline, setWS, setWsConnected,
    setIncomingCall, setPendingCallAnswer,
  } = useStore();

  useEffect(() => {
    (async () => {
      try {
        const token = await SecureStore.getItemAsync("obscura_token");
        if (!token) {
          router.replace("/(auth)/login");
          return;
        }
        await registerPushToken();
        const [me, convs] = await Promise.all([api.getMe(), api.getConversations()]);
        api.getTurnCredentials().then((creds: any) => {
          if (creds?.urls && creds?.username && creds?.credential) {
            rtcSession.setTurnCredentials(creds);
          }
        }).catch(() => {});
        setUser(me);
        setConversations(convs || []);
        // WebSocket
        const ws = createWS(
          token,
          (msg) => {
            const p = msg.payload ?? msg.data;
            switch (msg.type) {
              case "new_message": if (p) addMessage(p); break;
              case "message_delivered": if (p?.msg_id) updateMsgStatus(p.msg_id, "delivered"); break;
              case "message_read": if (p?.msg_id) updateMsgStatus(p.msg_id, "read"); break;
              case "user_online": if (p?.did) setOnline(p.did, true); break;
              case "user_offline": if (p?.did) setOnline(p.did, false); break;
              case "call_invite":
                if (p?.call_id && p?.sdp_offer) {
                  setIncomingCall({
                    callId: p.call_id,
                    callerDid: p.caller_did || "",
                    callerName: p.caller_name || "Arayan",
                    sdpOffer: p.sdp_offer,
                    callType: p.call_type || "audio",
                  });
                  router.push("/(main)/incoming-call" as any);
                }
                break;
              case "call_answer":
                if (p?.sdp_answer) setPendingCallAnswer(p.sdp_answer);
                break;
              case "call_end":
                setIncomingCall(null);
                rtcSession.markEnded();
                break;
            }
          },
          () => setWsConnected(true),
          () => setWsConnected(false),
        );
        setWS(ws);
        router.replace("/(main)/chats");
      } catch {
        router.replace("/(auth)/login");
      } finally {
        SplashScreen.hideAsync();
      }
    })();

    // Deep link: obscura://join/{token}
    const handleUrl = async ({ url }: { url: string }) => {
      const parsed = Linking.parse(url);
      if (parsed.hostname === "join" && parsed.path) {
        const inviteToken = parsed.path.replace("/", "");
        try {
          const { conv_id } = await api.joinViaInvite(inviteToken);
          router.push(`/(main)/chat/${conv_id}` as any);
        } catch { Alert.alert("Hata", "Davete katılınamadı"); }
      }
    };
    Linking.getInitialURL().then((url) => { if (url) handleUrl({ url }); });
    const sub = Linking.addEventListener("url", handleUrl);
    return () => sub.remove();
  }, []);

  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <StatusBar style="light" />
      <Stack screenOptions={{ headerShown: false }} />
    </GestureHandlerRootView>
  );
}
