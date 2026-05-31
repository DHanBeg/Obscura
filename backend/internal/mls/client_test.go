package mls

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func mlsBinPath(t testing.TB) string {
	wd, _ := os.Getwd()
	// backend/internal/mls/ → ../../../crypto/target/release/mls-cli{,.exe}
	root := filepath.Join(wd, "..", "..", "..", "crypto", "target", "release")
	bin := filepath.Join(root, "mls-cli")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("mls-cli not built — run: cd crypto && cargo build --release --bin mls-cli\n  (looked at %s)", bin)
	}
	return bin
}

func TestClient_Ping(t *testing.T) {
	c, err := NewClient(mlsBinPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestClient_FullTwoPartyFlow(t *testing.T) {
	bin := mlsBinPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Two clients = two subprocesses, one per identity (would be different devices in prod)
	alice, err := NewClient(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer alice.Close()

	bob, err := NewClient(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer bob.Close()

	// 1. Each generates an identity
	aliceID, err := alice.NewIdentity(ctx, "did:obs:alice")
	if err != nil {
		t.Fatal(err)
	}
	bobID, err := bob.NewIdentity(ctx, "did:obs:bob")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Bob publishes a KeyPackage
	bobKP, err := bob.GenerateKeyPackage(ctx, bobID)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Alice creates a group
	aliceGroupID, err := alice.CreateGroup(ctx, aliceID)
	if err != nil {
		t.Fatal(err)
	}

	// 4. Alice adds Bob (server would broadcast commit + welcome)
	add, err := alice.AddMember(ctx, aliceGroupID, aliceID, bobKP)
	if err != nil {
		t.Fatal(err)
	}
	if add.Epoch != 1 {
		t.Fatalf("expected epoch 1, got %d", add.Epoch)
	}

	// 5. Bob processes the Welcome
	join, err := bob.ProcessWelcome(ctx, bobID, add.WelcomeB64)
	if err != nil {
		t.Fatal(err)
	}
	if join.Epoch != 1 {
		t.Fatalf("bob expected epoch 1, got %d", join.Epoch)
	}

	bobGroupID := join.GroupID
	if bobGroupID != aliceGroupID {
		t.Fatalf("group IDs differ: alice=%s bob=%s", aliceGroupID, bobGroupID)
	}

	// 6. Alice encrypts a message
	plaintext := []byte("hello bob, mls e2ee via Go subprocess RPC")
	ct, err := alice.Encrypt(ctx, aliceGroupID, aliceID, plaintext)
	if err != nil {
		t.Fatal(err)
	}

	// 7. Bob decrypts
	pt, epoch, err := bob.ProcessMessage(ctx, bobGroupID, ct)
	if err != nil {
		t.Fatalf("bob process: %v", err)
	}
	if string(pt) != string(plaintext) {
		t.Fatalf("plaintext mismatch:\n  want: %s\n  got:  %s", plaintext, pt)
	}
	if epoch != 1 {
		t.Fatalf("expected epoch 1 after decrypt, got %d", epoch)
	}

	// 8. Round-trip: Bob → Alice
	pt2 := []byte("hi alice")
	ct2, err := bob.Encrypt(ctx, bobGroupID, bobID, pt2)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := alice.ProcessMessage(ctx, aliceGroupID, ct2)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(pt2) {
		t.Fatalf("round-trip mismatch")
	}
}

func TestClient_RejectsLargeBase64Garbage(t *testing.T) {
	c, err := NewClient(mlsBinPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx := context.Background()
	id, _ := c.NewIdentity(ctx, "did:obs:test")
	groupID, _ := c.CreateGroup(ctx, id)

	garbage := base64.StdEncoding.EncodeToString(make([]byte, 1024))
	_, _, err = c.ProcessMessage(ctx, groupID, garbage)
	if err == nil {
		t.Fatal("expected error for garbage input")
	}
}
