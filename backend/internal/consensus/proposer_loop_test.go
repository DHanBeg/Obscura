package consensus

import "testing"

func TestTryPropose_SkipsWhenNotProposer(t *testing.T) {
	published := 0
	publish := func(string, []byte) error { published++; return nil }
	e := NewEngine("node-1", 1, nil, publish, noopSubscribe, func() string { return "node-2" }, nil)
	mp := NewMempool()
	mp.Add("op-1")

	tryPropose(e, mp)

	if published != 0 {
		t.Fatalf("proposer olmayan node blok yayınlamamalıydı, publish çağrı sayısı=%d", published)
	}
	if mp.Len() != 1 {
		t.Fatalf("proposer değilken mempool'a dokunulmamalıydı, got len=%d", mp.Len())
	}
}

func TestTryPropose_SkipsWhenMempoolEmpty(t *testing.T) {
	published := 0
	publish := func(string, []byte) error { published++; return nil }
	e := NewEngine("node-1", 1, nil, publish, noopSubscribe, func() string { return "node-1" }, nil)
	mp := NewMempool()

	tryPropose(e, mp)

	if published != 0 {
		t.Fatalf("boş mempool ile blok önerilmemeliydi (log-spam koruması), publish çağrı sayısı=%d", published)
	}
}

func TestTryPropose_ProposesWhenProposerAndMempoolNonEmpty(t *testing.T) {
	published := 0
	publish := func(string, []byte) error { published++; return nil }
	e := NewEngine("node-1", 1, nil, publish, noopSubscribe, func() string { return "node-1" }, nil)
	mp := NewMempool()
	mp.Add("op-1")
	mp.Add("op-2")

	tryPropose(e, mp)

	if published != 1 {
		t.Fatalf("proposer + dolu mempool ile tam olarak 1 blok yayınlanmalıydı, got=%d", published)
	}
	if mp.Len() != 0 {
		t.Fatalf("başarılı proposeden sonra mempool boşalmalıydı, got len=%d", mp.Len())
	}
}

func TestTryPropose_RequeuesOpsOnProposeBlockFailure(t *testing.T) {
	// proposerFn tryPropose'un IsProposer() kontrolünde "node-1" (proposer),
	// ama ProposeBlock'un KENDİ iç kontrolünde "node-2" (proposer DEĞİL)
	// döndürsün — race/hata senaryosunu simüle eder.
	calls := 0
	proposerFn := func() string {
		calls++
		if calls == 1 {
			return "node-1"
		}
		return "node-2"
	}
	e := NewEngine("node-1", 1, nil, noopPublish, noopSubscribe, proposerFn, nil)
	mp := NewMempool()
	mp.Add("op-1")
	mp.Add("op-2")

	tryPropose(e, mp)

	if mp.Len() != 2 {
		t.Fatalf("ProposeBlock hatasında op'lar mempool'a geri konmalıydı, got len=%d", mp.Len())
	}
}
