---
name: test-pyramid-obscura
description: Test strategy for Obscura — unit, integration, e2e, load, fuzz, property. Use when planning test coverage or writing new tests.
---

# Obscura Test Pyramid

## Targets (from spec Bölüm 15.1)

- Unit tests: > 80% coverage
- Integration: cross-component flows
- E2E: full user scenarios on real devices
- Performance: meet Bölüm 15.2 budgets

## Unit tests

### Go (backend)

`*_test.go` next to source. `testing` stdlib + `testify`.

```go
package api

import (
    "net/http"
    "net/http/httptest"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestSendMessage_HappyPath(t *testing.T) {
    // Arrange
    db := newTestDB(t)
    server := httptest.NewServer(buildRouter(db))
    defer server.Close()
    token := loginAsTestUser(t, server, "+905551234567")

    // Act
    resp := postJSON(t, server.URL+"/v1/messages", token, map[string]any{
        "conversation_id": "test-conv",
        "ciphertext":      "encrypted-bytes",
        "message_type":    "text",
    })

    // Assert
    require.Equal(t, http.StatusOK, resp.StatusCode)
    var body map[string]any
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
    assert.True(t, body["success"].(bool))
    assert.NotEmpty(t, body["data"].(map[string]any)["id"])
}

func TestSendMessage_RequiresAuth(t *testing.T) {
    server := httptest.NewServer(buildRouter(newTestDB(t)))
    defer server.Close()
    resp, _ := http.Post(server.URL+"/v1/messages", "application/json", nil)
    assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
```

Run:
```bash
cd backend && go test -race -cover ./...
```

### Rust (crypto)

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn x3dh_handshake_produces_matching_keys() {
        let (alice_kp, bob_bundle) = setup();
        let alice_master = x3dh_initiator(&alice_kp, &bob_bundle).unwrap();
        let bob_master = x3dh_responder(&bob_kp, &alice_pub, &bob_bundle).unwrap();
        assert_eq!(alice_master, bob_master);
    }

    #[test]
    fn encrypt_decrypt_roundtrip() {
        let key = [0u8; 32];
        let plaintext = b"hello obscura";
        let ct = encrypt(&key, plaintext, b"aad").unwrap();
        let pt = decrypt(&key, &ct, b"aad").unwrap();
        assert_eq!(plaintext, pt.as_slice());
    }
}
```

### TypeScript (frontend)

Vitest:
```ts
import { describe, it, expect, vi } from "vitest";
import { apiFetch } from "@/lib/api";

describe("apiFetch", () => {
  it("returns data on success", async () => {
    global.fetch = vi.fn().mockResolvedValue({
      json: async () => ({ success: true, data: { foo: "bar" } }),
    });
    const result = await apiFetch("/test");
    expect(result).toEqual({ foo: "bar" });
  });

  it("throws on error response", async () => {
    global.fetch = vi.fn().mockResolvedValue({
      json: async () => ({ success: false, error: "nope" }),
    });
    await expect(apiFetch("/test")).rejects.toThrow("nope");
  });
});
```

### Circom

`circuits/test/credit_threshold.test.js`:
```js
const wasm_tester = require("circom_tester").wasm;
const { expect } = require("chai");

describe("credit_threshold", () => {
  it("accepts score above threshold", async () => {
    const circuit = await wasm_tester("./credit_threshold.circom");
    const witness = await circuit.calculateWitness({
      currentScore: 80, threshold: 70,
      // ... rest of inputs
    });
    await circuit.checkConstraints(witness);
  });

  it("rejects score below threshold", async () => {
    const circuit = await wasm_tester("./credit_threshold.circom");
    await expect(
      circuit.calculateWitness({ currentScore: 50, threshold: 70, /*...*/ })
    ).to.be.rejected;
  });
});
```

## Integration tests

`backend/internal/api/integration_test.go` — full HTTP flow with real SQLite (in-memory).

```go
func TestE2EMessageFlow(t *testing.T) {
    server := setupTestServer(t)
    defer server.Close()

    // Register Alice + Bob
    aliceToken := registerUser(t, server, "+901111111111")
    bobToken := registerUser(t, server, "+902222222222")

    // Alice uploads prekeys
    uploadPrekeys(t, server, aliceToken, alicePrekeys)

    // Bob fetches Alice's bundle
    bundle := getPrekeyBundle(t, server, bobToken, aliceDID)
    require.NotEmpty(t, bundle.IdentityKey)

    // Bob sends encrypted message
    convID := createConversation(t, server, bobToken, aliceDID)
    msg := sendMessage(t, server, bobToken, convID, "encrypted-payload")
    require.NotEmpty(t, msg.ID)

    // Alice receives via WS
    ws := openWS(t, server, aliceToken)
    received := readWSMessage(t, ws, 5*time.Second)
    assert.Equal(t, msg.ID, received["data"].(map[string]any)["id"])
}
```

## E2E tests

### Web (Playwright)

`e2e/specs/full_flow.spec.ts`:
```ts
import { test, expect } from "@playwright/test";

test("user can send message end-to-end", async ({ page }) => {
  await page.goto("http://localhost:3000/login");
  await page.fill('input[name="phone"]', "+905551234567");
  await page.click('button:has-text("Devam")');
  // ... mock OTP flow
  await page.fill('input[name="otp"]', "123456");
  await page.click('button:has-text("Giriş")');

  await expect(page.locator("text=Yeni Sohbet")).toBeVisible();

  await page.click("text=Yeni Sohbet");
  await page.fill('input[name="search"]', "test_user");
  await page.click("text=test_user");

  await page.fill('textarea[placeholder*="Mesaj"]', "Test mesajı");
  await page.click('button[aria-label="Gönder"]');
  await expect(page.locator("text=Test mesajı")).toBeVisible();
});
```

### Mobile (Maestro)

`e2e/mobile/login.yaml`:
```yaml
appId: com.obscura.app
---
- launchApp
- tapOn: "Telefon"
- inputText: "+905551234567"
- tapOn: "Devam"
- waitForAnimationToEnd
- tapOn: "OTP"
- inputText: "123456"
- assertVisible: "Sohbetler"
```

Run: `maestro test e2e/mobile/login.yaml`

### Desktop (Tauri-driver + Playwright)

Use `tauri-driver` to run WebDriver against built Tauri app.

## Load tests

### k6 (HTTP + WebSocket)

`load/messaging.js`:
```js
import http from "k6/http";
import ws from "k6/ws";
import { check, sleep } from "k6";

export const options = {
  stages: [
    { duration: "30s", target: 100 },
    { duration: "1m", target: 1000 },
    { duration: "30s", target: 0 },
  ],
  thresholds: {
    http_req_duration: ["p(95)<300"],   // spec budget
    http_req_failed: ["rate<0.01"],
  },
};

export default function() {
  const token = login(`+9055500${__VU.toString().padStart(5, "0")}`);
  const res = http.post("http://localhost:8080/v1/messages",
    JSON.stringify({ conversation_id: "x", ciphertext: "y", message_type: "text" }),
    { headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" } });
  check(res, { "status 200": r => r.status === 200 });
  sleep(0.5);
}
```

Run: `k6 run load/messaging.js`

## Fuzz tests

### Go native

```go
func FuzzVerifyJWT(f *testing.F) {
    f.Add("valid-token-here")
    f.Fuzz(func(t *testing.T, input string) {
        _, _ = auth.VerifyJWT(input)  // must not panic
    })
}
```

Run: `go test -fuzz=FuzzVerifyJWT -fuzztime=30s`

### Rust (cargo-fuzz)

```rust
fuzz_target!(|data: &[u8]| {
    if let Ok(s) = std::str::from_utf8(data) {
        let _ = parse_did(s);  // must not panic
    }
});
```

Run: `cargo fuzz run did_parser`

## Property-based

```rust
use proptest::prelude::*;

proptest! {
    #[test]
    fn encrypt_decrypt_roundtrip(plaintext: Vec<u8>, key: [u8; 32]) {
        let ct = encrypt(&key, &plaintext, b"aad").unwrap();
        let pt = decrypt(&key, &ct, b"aad").unwrap();
        prop_assert_eq!(plaintext, pt);
    }
}
```

## CI integration

```yaml
# .github/workflows/test.yml
- name: Unit tests (Go)
  run: cd backend && go test -race -cover ./...
- name: Unit tests (TS)
  run: cd frontend && npm test
- name: Unit tests (Rust)
  run: cd crypto && cargo test
- name: Circuit tests
  run: cd circuits && npm test
- name: Integration
  run: cd backend && go test -tags=integration ./...
- name: E2E (web)
  run: docker compose up -d && npx playwright test
- name: Load test (smoke)
  run: k6 run --duration 30s load/messaging.js
```

## Rules

- Test name describes behavior: `TestSendMessage_ReturnsErrorWhenRecipientOffline`
- One logical assertion per test
- AAA structure: Arrange, Act, Assert
- No sleeps — sync primitives or fake clock
- Fixed seed for any random
- Coverage gates: 80% line, 70% branch
- New code without tests = rejected
- Flaky test = bug, quarantine + fix
