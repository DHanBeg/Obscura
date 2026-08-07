#!/bin/sh
# docker-entrypoint.sh — Railway (ve genel olarak Docker) volume mount'ları,
# build-time'da chown edilmiş bir dizinin ÜZERİNE kendi (genelde root-owned)
# dizinini getirir — image build sırasında yapılan `chown obscura:obscura`
# runtime'da mount edilen volume'a UYGULANMAZ. Bu yüzden konteyner HER ZAMAN
# root olarak başlar (Dockerfile'da artık USER yönergesi yok), burada
# DATA_DIR'i (varsa) yeniden chown'lar, SONRA obscura kullanıcısına düşüp
# asıl komutu çalıştırır (su-exec).
set -e

if [ -n "$DATA_DIR" ]; then
	mkdir -p "$DATA_DIR"
	chown -R obscura:obscura "$DATA_DIR" 2>/dev/null || true
fi

exec su-exec obscura "$@"
