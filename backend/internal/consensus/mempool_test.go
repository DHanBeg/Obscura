package consensus

import "testing"

func TestMempool_AddAndLen(t *testing.T) {
	m := NewMempool()
	if m.Len() != 0 {
		t.Fatalf("yeni mempool boş olmalı, got len=%d", m.Len())
	}
	m.Add("op-1")
	m.Add("op-2")
	if m.Len() != 2 {
		t.Fatalf("2 op eklendi, len=2 beklenirdi, got=%d", m.Len())
	}
}

func TestMempool_AddIgnoresEmptyString(t *testing.T) {
	m := NewMempool()
	m.Add("")
	if m.Len() != 0 {
		t.Fatalf("boş string eklenmemeliydi, got len=%d", m.Len())
	}
}

func TestMempool_DrainReturnsAllAndClears(t *testing.T) {
	m := NewMempool()
	m.Add("op-1")
	m.Add("op-2")
	m.Add("op-3")

	drained := m.Drain()
	if len(drained) != 3 {
		t.Fatalf("3 op drain edilmeliydi, got=%d", len(drained))
	}
	if m.Len() != 0 {
		t.Fatalf("drain sonrası mempool boş olmalı, got len=%d", m.Len())
	}
}

func TestMempool_DrainOnEmptyReturnsNil(t *testing.T) {
	m := NewMempool()
	drained := m.Drain()
	if drained != nil {
		t.Fatalf("boş mempool'dan drain nil dönmeli, got=%v", drained)
	}
}

func TestMempool_DrainPreservesOrder(t *testing.T) {
	m := NewMempool()
	m.Add("a")
	m.Add("b")
	m.Add("c")
	drained := m.Drain()
	if drained[0] != "a" || drained[1] != "b" || drained[2] != "c" {
		t.Fatalf("drain sırası korunmalı, got=%v", drained)
	}
}
