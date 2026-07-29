package api_test

// nickname (display_name) zorunlu alan testleri — PATCH /v1/users/me.
//
// display_name gönderilmediyse (nil) dokunulmaz; gönderilip boş/sadece-
// boşluk/sadece-bidi-kontrol-karakteriyse 400 döner; geçerliyse trim+bidi-
// temizlenmiş haliyle kaydedilir.

import (
	"encoding/json"
	"strings"
	"testing"

	"obscura.network/core/internal/api"
)

func strPtr(s string) *string { return &s }

func TestUpdateMeRejectsEmptyDisplayName(t *testing.T) {
	_, token := registerUserDirect(t, "+905559990401", "nick_empty_001")

	resp, code := patch(t, "/v1/users/me", api.UpdateMeRequest{DisplayName: strPtr("")}, token)
	if code != 400 {
		t.Fatalf("beklenen 400, alınan %d: %s", code, resp.Error)
	}
}

func TestUpdateMeRejectsWhitespaceOnlyDisplayName(t *testing.T) {
	_, token := registerUserDirect(t, "+905559990402", "nick_ws_001")

	resp, code := patch(t, "/v1/users/me", api.UpdateMeRequest{DisplayName: strPtr("   ")}, token)
	if code != 400 {
		t.Fatalf("beklenen 400, alınan %d: %s", code, resp.Error)
	}
}

func TestUpdateMeRejectsOversizedDisplayName(t *testing.T) {
	_, token := registerUserDirect(t, "+905559990403", "nick_long_001")

	resp, code := patch(t, "/v1/users/me", api.UpdateMeRequest{DisplayName: strPtr(strings.Repeat("a", 51))}, token)
	if code != 400 {
		t.Fatalf("beklenen 400 (51 karakter), alınan %d: %s", code, resp.Error)
	}
}

func TestUpdateMeTrimsDisplayNameWhitespace(t *testing.T) {
	_, token := registerUserDirect(t, "+905559990404", "nick_trim_001")

	resp, code := patch(t, "/v1/users/me", api.UpdateMeRequest{DisplayName: strPtr("  Ada Lovelace  ")}, token)
	if code != 200 {
		t.Fatalf("beklenen 200, alınan %d: %s", code, resp.Error)
	}
	var body struct {
		DisplayName string `json:"display_name"`
	}
	json.Unmarshal(resp.Data, &body)
	if body.DisplayName != "Ada Lovelace" {
		t.Fatalf("display_name = %q, istenen %q", body.DisplayName, "Ada Lovelace")
	}
}

func TestUpdateMeStripsBidiControlChars(t *testing.T) {
	_, token := registerUserDirect(t, "+905559990405", "nick_bidi_001")

	// U+202E (RLO) + "eviL" + U+202C benzeri desen — spoof denemesi
	poisoned := "‮nasiL"
	resp, code := patch(t, "/v1/users/me", api.UpdateMeRequest{DisplayName: strPtr(poisoned)}, token)
	if code != 200 {
		t.Fatalf("beklenen 200 (temizlenip kabul edilmeli), alınan %d: %s", code, resp.Error)
	}
	var body struct {
		DisplayName string `json:"display_name"`
	}
	json.Unmarshal(resp.Data, &body)
	if strings.ContainsRune(body.DisplayName, '‮') {
		t.Fatalf("display_name bidi kontrol karakteri içermemeli: %q", body.DisplayName)
	}
	if body.DisplayName != "nasiL" {
		t.Fatalf("display_name = %q, istenen %q", body.DisplayName, "nasiL")
	}
}

func TestUpdateMeOmittedDisplayNameDoesNotClear(t *testing.T) {
	_, token := registerUserDirect(t, "+905559990406", "nick_omit_001")

	one := 1
	resp, code := patch(t, "/v1/users/me", api.UpdateMeRequest{HideOnline: &one}, token)
	if code != 200 {
		t.Fatalf("beklenen 200, alınan %d: %s", code, resp.Error)
	}
	var body struct {
		DisplayName string `json:"display_name"`
	}
	json.Unmarshal(resp.Data, &body)
	if body.DisplayName != "nick_omit_001" {
		t.Fatalf("display_name gönderilmediğinde değişmemeli, alınan: %q", body.DisplayName)
	}
}

func TestUpdateMeRejectsBidiOnlyDisplayName(t *testing.T) {
	_, token := registerUserDirect(t, "+905559990407", "nick_bidionly_001")

	resp, code := patch(t, "/v1/users/me", api.UpdateMeRequest{DisplayName: strPtr("‮​")}, token)
	if code != 400 {
		t.Fatalf("sadece bidi/zero-width karakter → 400 beklenir, alınan %d: %s", code, resp.Error)
	}
}
