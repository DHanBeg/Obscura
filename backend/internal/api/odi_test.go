package api_test

// ODI (Obscura Display Identifier) endpoint testleri.
//
// GET /v1/users/{did} artık odi alanı döner, GET /v1/users/by-odi/{odi} ise
// ODI'den DID'e çözüm yapar (ODI'nin kendisi tek yönlü hash olduğundan bu
// çözüm sadece bu indexli DB araması üzerinden mümkün).

import (
	"encoding/json"
	"testing"

	"obscura.network/core/internal/identity"
)

func TestGetUserResponseIncludesODI(t *testing.T) {
	targetDID, _ := registerUserDirect(t, "+905559990301", "odi_target_001")
	_, viewerToken := registerUserDirect(t, "+905559990302", "odi_viewer_001")

	resp, code := get(t, "/v1/users/"+targetDID, viewerToken)
	if code != 200 || !resp.Success {
		t.Fatalf("GET /v1/users/{did} başarısız (code=%d): %s", code, resp.Error)
	}

	var body struct {
		DID string `json:"did"`
		Odi string `json:"odi"`
	}
	json.Unmarshal(resp.Data, &body)

	want := identity.DeriveODI(targetDID)
	if body.Odi != want {
		t.Fatalf("odi = %q, istenen %q", body.Odi, want)
	}
	if body.Odi == "" {
		t.Fatal("odi boş dönmemeli")
	}
}

func TestGetUserByODIResolvesDID(t *testing.T) {
	targetDID, _ := registerUserDirect(t, "+905559990303", "odi_target_002")
	_, viewerToken := registerUserDirect(t, "+905559990304", "odi_viewer_002")

	odi := identity.DeriveODI(targetDID)
	resp, code := get(t, "/v1/users/by-odi/"+odi, viewerToken)
	if code != 200 || !resp.Success {
		t.Fatalf("GET /v1/users/by-odi/{odi} başarısız (code=%d): %s", code, resp.Error)
	}

	var body struct {
		DID string `json:"did"`
		Odi string `json:"odi"`
	}
	json.Unmarshal(resp.Data, &body)

	if body.DID != targetDID {
		t.Fatalf("did = %q, istenen %q", body.DID, targetDID)
	}
}

func TestGetUserByODIUnknownReturns404(t *testing.T) {
	_, viewerToken := registerUserDirect(t, "+905559990305", "odi_viewer_003")

	resp, code := get(t, "/v1/users/by-odi/ODI-0000-0000-0000", viewerToken)
	if code != 404 {
		t.Fatalf("beklenen 404, alınan %d: %s", code, resp.Error)
	}
}
