import { KeyValueStore } from "../lib/keyValueStore";

// Gerçek SecureStore test ortamında native köprü olmadan sessizce no-op
// döner (doğrulandı) — testler bu in-memory implementasyonu enjekte eder.
export function createMemoryStore(): KeyValueStore {
  const map = new Map<string, string>();
  return {
    async getItem(key) {
      return map.has(key) ? map.get(key)! : null;
    },
    async setItem(key, value) {
      map.set(key, value);
    },
    async deleteItem(key) {
      map.delete(key);
    },
  };
}
