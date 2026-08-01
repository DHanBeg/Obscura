// proposer_loop.go — mempool'u ProposeBlock'a bağlayan arka plan zamanlayıcı.
//
// ADR-0017: BFT'nin "blok-üretim scheduler'ı" eksik parçası. Leader election
// sequencer'dan geliyor (bkz. bft.go proposerFn), txRoot mempool'dan +
// sequencer.ComputeMerkleRoot'tan geliyor — bu dosya sadece ikisini periyodik
// olarak birleştiriyor.
package consensus

import (
	"context"
	"log"
	"time"
)

// ProposalInterval, mempool'da bekleyen op varsa ne sıklıkla blok önerisi
// denensin. RoundTimeout'tan kısa — bir round içinde birden fazla deneme
// şansı olsun diye (ama gereksiz sık log/CPU olmasın diye 8s'den küçük ama
// makul bir değer).
const ProposalInterval = 3 * time.Second

// StartProposerLoop, bu node proposer olduğunda VE mempool'da bekleyen op
// varsa periyodik olarak ProposeBlock çağıran arka plan döngüsünü başlatır.
// ctx iptal edilince durur. Mempool boşsa hiçbir şey yapmaz (boş blok
// önerilmez — orijinal "if false" kapatma sebebi olan log-spam/CPU sorununu
// tekrarlamamak için, bkz. ADR-0017).
func StartProposerLoop(ctx context.Context, e *Engine, mp *Mempool) {
	go func() {
		ticker := time.NewTicker(ProposalInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tryPropose(e, mp)
			}
		}
	}()
}

func tryPropose(e *Engine, mp *Mempool) {
	if !e.IsProposer() {
		return
	}
	ops := mp.Drain()
	if len(ops) == 0 {
		return
	}
	if err := e.ProposeBlock(ops); err != nil {
		log.Printf("⚠️  BFT proposer loop: ProposeBlock hata, op'lar mempool'a geri konuyor: %v", err)
		// Op'lar kaybolmasın — bir sonraki denemede tekrar dahil edilsinler.
		for _, op := range ops {
			mp.Add(op)
		}
	}
}
