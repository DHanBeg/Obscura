package consensus

import "testing"

func TestTryPropose_SkipsWhenNotProposer(t *testing.T) {
	published := 0
	publish := func(string, []byte) error { published++; return nil }
	e := NewEngine("node-1", 1, nil, publish, noopSubscribe, func() string { return "node-2" }, nil, nil, nil)
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
	e := NewEngine("node-1", 1, nil, publish, noopSubscribe, func() string { return "node-1" }, nil, nil, nil)
	mp := NewMempool()

	tryPropose(e, mp)

	if published != 0 {
		t.Fatalf("boş mempool ile blok önerilmemeliydi (log-spam koruması), publish çağrı sayısı=%d", published)
	}
}

func TestTryPropose_ProposesWhenProposerAndMempoolNonEmpty(t *testing.T) {
	published := 0
	publish := func(string, []byte) error { published++; return nil }
	e := NewEngine("node-1", 1, nil, publish, noopSubscribe, func() string { return "node-1" }, nil, nil, nil)
	mp := NewMempool()
	mp.Add("op-1")
	mp.Add("op-2")

	tryPropose(e, mp)

	// A5 (self-vote yerel-sayma fix): quorum=1 olduğu için ProposeBlock artık
	// sadece proposal'ı değil, kendi prevote'unu VE (kendi prevote'u tek
	// başına quorum'u karşıladığı için) kendi precommit'ini de yayınlıyor —
	// proposal(1) + precommit(1, collectVote(pv) içinden kaskad) + prevote(1)
	// = 3. A5 öncesi bu sayı 1'di çünkü proposer kendi oyunu HİÇ üretmiyordu
	// (bkz. A5 Faz 0 raporu) — quorum=1 gerçek tek-node'da bile hiç
	// tamamlanmıyordu, sadece bu testin publish-no-op'u yüzünden fark
	// edilmiyordu.
	if published != 3 {
		t.Fatalf("proposer + dolu mempool ile proposal+prevote+precommit (3) yayınlanmalıydı, got=%d", published)
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
	e := NewEngine("node-1", 1, nil, noopPublish, noopSubscribe, proposerFn, nil, nil, nil)
	mp := NewMempool()
	mp.Add("op-1")
	mp.Add("op-2")

	tryPropose(e, mp)

	if mp.Len() != 2 {
		t.Fatalf("ProposeBlock hatasında op'lar mempool'a geri konmalıydı, got len=%d", mp.Len())
	}
}
