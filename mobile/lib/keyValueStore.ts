import * as SecureStore from "expo-secure-store";
import AsyncStorage from "@react-native-async-storage/async-storage";

// SecureStore çağrıları test ortamında native modül olmadan sessizce no-op
// döner (native köprü yok), AsyncStorage ise import anında crash eder
// (native modül null) — identity.ts/prekeys.ts/session-store.ts bu arayüz
// üzerinden enjekte edilebilir bir store kullanır, testler gerçek in-memory
// bir implementasyon geçer.
export interface KeyValueStore {
  getItem(key: string): Promise<string | null>;
  setItem(key: string, value: string): Promise<void>;
  deleteItem(key: string): Promise<void>;
}

// Küçük, hassas veriler (master key, identity/SPK private key'leri) — iOS
// Keychain/Android Keystore, pratik boyut sınırı düşük (~2KB/item).
export const secureStore: KeyValueStore = {
  getItem: (key) => SecureStore.getItemAsync(key),
  setItem: (key, value) => SecureStore.setItemAsync(key, value),
  deleteItem: (key) => SecureStore.deleteItemAsync(key),
};

// Büyük veriler (şifreli ratchet session blob'ları) — SecureStore'un boyut
// sınırını aşabilir. Kendisi şifrelemez; session-store.ts master-key ile
// AES-GCM şifreleyip buraya yazar.
export const asyncStore: KeyValueStore = {
  getItem: (key) => AsyncStorage.getItem(key),
  setItem: (key, value) => AsyncStorage.setItem(key, value),
  deleteItem: (key) => AsyncStorage.removeItem(key),
};
