-- 5 node için ayrı database (node başına izolasyon).
-- postgres image bunu container ilk kez başlarken (boş data dir) otomatik
-- çalıştırır: /docker-entrypoint-initdb.d/*.sql, alfabetik sırayla, tek sefer.
CREATE DATABASE node1_db;
CREATE DATABASE node2_db;
CREATE DATABASE node3_db;
CREATE DATABASE node4_db;
CREATE DATABASE node5_db;
