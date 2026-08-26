// Node'un native WebSocket'i (v22+) SADECE 2-argümanlı (url, {headers}) şeklini
// destekliyor. React Native'in WebSocket'i (node_modules/react-native/Libraries/
// WebSocket/WebSocket.js:98-148) 3-argümanlı (url, protocols, {headers}) şeklini
// native modüle geçiriyor — bu, lib/api.ts:createWS()'in GERÇEK React Native
// çalışma zamanında kullandığı, doğru ve kanıtlanmış şekil (bkz. FAZ A WS auth
// fix'i, B6 oturum notu). createWS() BU YÜZDEN değiştirilmedi — sadece Node/Jest
// smoke testlerinin bu üçüncü argümanı Node'un anladığı 2-argüman şekline
// çevirmesi gerekiyor, aksi halde header'lar sessizce yutulur (401 → 1006 close,
// kanıtlandı: ws-arg-shape-probe, 3-ARG çağrı CLOSE, 2-ARG çağrı OPEN).
export function installNodeWebSocketHeaderShim(): void {
  const RealWebSocket: any = (globalThis as any).WebSocket;
  function ShimmedWebSocket(this: any, url: string, protocols?: any, options?: any) {
    if (options && typeof options === "object") {
      return new RealWebSocket(url, options);
    }
    return new RealWebSocket(url, protocols);
  }
  ShimmedWebSocket.prototype = RealWebSocket.prototype;
  Object.assign(ShimmedWebSocket, RealWebSocket);
  (globalThis as any).WebSocket = ShimmedWebSocket;
}
