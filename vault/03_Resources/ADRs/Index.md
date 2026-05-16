# ADR Index

Tam ADR'lar: `E:\obscura\docs\adr\`

## Liste

| # | Başlık | Durum |
|---|---|---|
| 0001 | modernc.org/sqlite seçimi | Accepted |
| 0002 | Flutter yerine RN+Next+Tauri | Accepted |
| 0003 | HTTP gossip FAZ 1, libp2p sonra | Accepted |
| 0004 | Crypto Go (FAZ 1 sapma) | Accepted |
| 0005 | Aztec zk-Rollup seçimi | Accepted |
| 0006 | Dev single-contributor trusted setup | Accepted |
| 0007 | openmls (RFC 9420) | Accepted |
| 0008 | FAZ 1 code-complete | Accepted |
| 0009 | FAZ 1 post-audit hardening (6 critical fix) | Accepted |
| 0010 | OBS token economics | Proposed |
| 0011 | Staking + slashing parametreleri | Proposed |
| 0012 | Governance mekanizması | Proposed |
| 0013 | ZK-ML moderation (heuristic + ezkl) | Accepted |

## Süreç

Yeni karar → `06_Metadata/Templates/ADR.md` kopyala → `E:\obscura\docs\adr\NNNN-kisa-baslik.md` (4-haneli sequential, asla yeniden kullanma).

Karar gerektirmeyen şeyler ADR yazma — değişken yeniden adlandırma, küçük bugfix vb.

ADR-0008'in (FAZ 1 code-complete) verdiği ders: ADR yazmadan ÖNCE alt-ajanlardan onay al. ADR-0008 fazla iyimserdi, sub-agent denetimi 6 critical buldu → ADR-0009 düzeltme oldu.
