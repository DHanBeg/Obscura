package api

import (
	"errors"
	"testing"
)

type stubDIDResolver struct {
	did string
	err error
}

func (s stubDIDResolver) FindDIDByPhone(string) (string, error) { return s.did, s.err }

func TestFindDIDByPhone(t *testing.T) {
	t.Cleanup(func() { SetDIDResolver(nil) })

	tests := []struct {
		name     string
		resolver DIDResolver
		want     string
	}{
		{"returns empty when no resolver installed", nil, ""},
		{"returns DID from resolver", stubDIDResolver{did: "did:obscura:abc"}, "did:obscura:abc"},
		{"returns empty when resolver errors", stubDIDResolver{did: "did:obscura:abc", err: errors.New("boom")}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetDIDResolver(tt.resolver)
			if got := findDIDByPhone("+905551112233"); got != tt.want {
				t.Fatalf("findDIDByPhone = %q, want %q", got, tt.want)
			}
		})
	}
}
