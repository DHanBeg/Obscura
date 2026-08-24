// E1 CANLI SMOKE — api.ts:9 yorumu Node test/proof script'lerinin gerçek
// local backend'e karşı apiFetch çağırabilmesini varsayıyor, ama
// XMLHttpRequest Jest'in node testEnvironment'ında hiç tanımlı değil
// (doğrulandı: "XMLHttpRequest is not defined"). Bu, api.ts'in xhrFetch'inin
// KULLANDIĞI minimal yüzeyin (open/setRequestHeader/timeout/onload/onerror/
// ontimeout/send) Node'un http modülü üzerinden bir polyfill'i — test-only,
// production kodu DEĞİŞMEDİ.
import * as http from "http";
import { URL } from "url";

class NodeXHR {
  timeout = 30000;
  status = 0;
  responseText = "";
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  ontimeout: (() => void) | null = null;
  private method = "GET";
  private url = "";
  private headers: Record<string, string> = {};

  open(method: string, url: string) {
    this.method = method;
    this.url = url;
  }

  setRequestHeader(k: string, v: string) {
    this.headers[k] = v;
  }

  send(body?: string) {
    const u = new URL(this.url);
    const req = http.request(
      {
        hostname: u.hostname,
        port: u.port,
        path: u.pathname + u.search,
        method: this.method,
        headers: this.headers,
        timeout: this.timeout,
      },
      (res) => {
        if (process.env.OBSCURA_XHR_DEBUG) {
          console.log("[nodeXhrPolyfill] reqPath=", u.pathname + u.search, "resHeaders=", JSON.stringify(res.headers));
        }
        let data = "";
        res.on("data", (chunk) => { data += chunk; });
        res.on("end", () => {
          this.status = res.statusCode || 0;
          this.responseText = data;
          if (process.env.OBSCURA_XHR_DEBUG) {
            console.log("[nodeXhrPolyfill] status=", this.status, "len=", data.length, "body=", data.slice(0, 200));
          }
          this.onload?.();
        });
      }
    );
    req.on("timeout", () => { req.destroy(); this.ontimeout?.(); });
    req.on("error", () => { this.onerror?.(); });
    req.end(body);
  }
}

export function installNodeXhrPolyfill(): void {
  (global as any).XMLHttpRequest = NodeXHR;
}
