import * as SecureStore from "expo-secure-store";

// SecureStore çağrıları test ortamında native modül olmadan sessizce no-op
// döner (native köprü yok) — identity.ts/prekeys.ts bu arayüz üzerinden
// enjekte edilebilir bir store kullanır, testler gerçek in-memory bir
// implementasyon geçer.
export interface KeyValueStore {
  getItem(key: string): Promise<string | null>;
  setItem(key: string, value: string): Promise<void>;
  deleteItem(key: string): Promise<void>;
}

export const secureStore: KeyValueStore = {
  getItem: (key) => SecureStore.getItemAsync(key),
  setItem: (key, value) => SecureStore.setItemAsync(key, value),
  deleteItem: (key) => SecureStore.deleteItemAsync(key),
};
