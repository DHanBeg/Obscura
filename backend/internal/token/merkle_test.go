package token

import (
	"testing"
)

// TestMerkleCacheConsistency: cache'li sonuç, beklenen davranışla tutarlı.
func TestMerkleEmptyRoot(t *testing.T) {
	tr := NewIncrementalMerkleTree()
	root := tr.ComputeRoot()
	if root != tr.zeros[TreeDepth] {
		t.Fatalf("boş ağaç kökü zero-root olmalı")
	}
}

// TestMerkleInsertAndProof: yaprak ekle, kanıt al, doğrula (happy path).
func TestMerkleInsertAndProof(t *testing.T) {
	tr := NewIncrementalMerkleTree()
	leaves := []string{
		"1111111111111111111111111111111111111111111111111111111111111111",
		"2222222222222222222222222222222222222222222222222222222222222222",
		"3333333333333333333333333333333333333333333333333333333333333333",
	}
	for _, l := range leaves {
		tr.InsertLeaf(l)
	}
	root := tr.ComputeRoot()
	for i, l := range leaves {
		proof, err := tr.GetProof(i)
		if err != nil {
			t.Fatalf("GetProof(%d): %v", i, err)
		}
		if !tr.VerifyProof(l, i, proof, root) {
			t.Fatalf("yaprak %d için kanıt doğrulanamadı", i)
		}
	}
}

// TestMerkleProofOutOfRange: geçersiz indeks hata döner (error case).
func TestMerkleProofOutOfRange(t *testing.T) {
	tr := NewIncrementalMerkleTree()
	tr.InsertLeaf("aa")
	if _, err := tr.GetProof(5); err == nil {
		t.Fatal("aralık dışı indeks hata vermeliydi")
	}
}

// TestMerkleCacheInvalidation: ekleme sonrası kök değişmeli ve cache tutarlı kalmalı.
func TestMerkleCacheInvalidation(t *testing.T) {
	tr := NewIncrementalMerkleTree()
	tr.InsertLeaf("1111111111111111111111111111111111111111111111111111111111111111")
	r1 := tr.ComputeRoot()
	r1b := tr.ComputeRoot() // cache hit, aynı olmalı
	if r1 != r1b {
		t.Fatal("cache hit farklı kök döndürdü")
	}
	tr.InsertLeaf("2222222222222222222222222222222222222222222222222222222222222222")
	r2 := tr.ComputeRoot()
	if r1 == r2 {
		t.Fatal("ekleme sonrası kök değişmeliydi (cache invalidate edilmedi)")
	}
}

// TestMerkleWrongProofFails: yanlış indeksle doğrulama başarısız.
func TestMerkleWrongProofFails(t *testing.T) {
	tr := NewIncrementalMerkleTree()
	tr.InsertLeaf("1111111111111111111111111111111111111111111111111111111111111111")
	tr.InsertLeaf("2222222222222222222222222222222222222222222222222222222222222222")
	root := tr.ComputeRoot()
	proof, _ := tr.GetProof(0)
	// yanlış indeksle doğrula
	if tr.VerifyProof("1111111111111111111111111111111111111111111111111111111111111111", 1, proof, root) {
		t.Fatal("yanlış indeksle doğrulama geçmemeliydi")
	}
}

// TestMerkleManyLeaves: çok sayıda yaprakla kök hesabı tutarlı (cache yararı).
func TestMerkleManyLeaves(t *testing.T) {
	tr := NewIncrementalMerkleTree()
	const n = 100
	for i := 0; i < n; i++ {
		// 32-byte hex leaf
		b := make([]byte, 32)
		b[31] = byte(i)
		b[30] = byte(i >> 8)
		leaf := ""
		for _, x := range b {
			const hexd = "0123456789abcdef"
			leaf += string(hexd[x>>4]) + string(hexd[x&0xf])
		}
		tr.InsertLeaf(leaf)
	}
	root := tr.ComputeRoot()
	proof, err := tr.GetProof(50)
	if err != nil {
		t.Fatalf("GetProof: %v", err)
	}
	// yaprak 50'yi yeniden üret
	b := make([]byte, 32)
	b[31] = 50
	leaf := ""
	for _, x := range b {
		const hexd = "0123456789abcdef"
		leaf += string(hexd[x>>4]) + string(hexd[x&0xf])
	}
	if !tr.VerifyProof(leaf, 50, proof, root) {
		t.Fatal("yaprak 50 kanıtı doğrulanamadı")
	}
}
