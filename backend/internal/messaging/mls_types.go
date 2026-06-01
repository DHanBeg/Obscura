package messaging

// MLS WebSocket mesaj tipleri (RFC 9420 — spec Bölüm 6.3).
//
// Bu sabitler hub broadcast'lerinde tip güvenliği sağlar. Handler'lar artık
// string literal yerine bu sabitleri kullanır.
//
// Routing kuralları:
//   - MsgTypeMlsWelcome: sadece hedef DID'ye (yeni eklenen üye)
//   - MsgTypeMlsCommit:  grup_id'ye göre TÜM üyelere (commit broadcast)
//   - MsgTypeMlsMessage: grup_id'ye göre TÜM üyelere (uygulama mesajı)
//   - MsgTypeMlsRemoved: sadece çıkarılan üyeye (bilgilendirme)
//   - MsgTypeMlsKeyUpdate: grup_id'ye göre TÜM üyelere (leaf key rotasyonu commit'i)
//   - MsgTypeMlsKPRotate: sahip kullanıcıya (KeyPackage süresi doldu — yeni KP yükle)
const (
	MsgTypeMlsWelcome   = "mls_welcome"
	MsgTypeMlsCommit    = "mls_commit"
	MsgTypeMlsMessage   = "mls_message"
	MsgTypeMlsRemoved   = "mls_removed"
	MsgTypeMlsKeyUpdate = "mls_key_update"
	MsgTypeMlsKPRotate  = "key_package_rotation_needed"
)
