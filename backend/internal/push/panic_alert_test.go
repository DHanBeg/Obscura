package push

import "testing"

// Madde 13 Adım 6 — PanicAlert() şablonu testleri.

func TestPanicAlertTitleContainsSenderName(t *testing.T) {
	msg := PanicAlert("Ayşe", "conv-1")
	if msg.Notification == nil {
		t.Fatal("Notification nil")
	}
	want := "🆘 Ayşe yardım istiyor"
	if msg.Notification.Title != want {
		t.Errorf("Title = %q, beklenen %q", msg.Notification.Title, want)
	}
}

// TestPanicAlertDataNeverLeaksLocation — Data alanı SADECE type+conv_id
// taşımalı. Konum (grid_id, lat, lon, semt adı — hiçbiri) push payload'ına
// asla girmemeli; konum yalnızca şifreli mesajın içindedir.
func TestPanicAlertDataNeverLeaksLocation(t *testing.T) {
	msg := PanicAlert("Ayşe", "conv-1")

	if len(msg.Data) != 2 {
		t.Fatalf("Data alanında %d anahtar var, beklenen tam 2 (type, conv_id): %#v", len(msg.Data), msg.Data)
	}
	if msg.Data["type"] != "panic_alert" {
		t.Errorf(`Data["type"] = %q, beklenen "panic_alert"`, msg.Data["type"])
	}
	if msg.Data["conv_id"] != "conv-1" {
		t.Errorf(`Data["conv_id"] = %q, beklenen "conv-1"`, msg.Data["conv_id"])
	}

	forbidden := []string{"grid_id", "lat", "lon", "latitude", "longitude", "location", "semt"}
	for _, key := range forbidden {
		if _, exists := msg.Data[key]; exists {
			t.Errorf("Data[%q] mevcut — konum push payload'ına sızmış", key)
		}
	}
}

func TestPanicAlertHighPriority(t *testing.T) {
	msg := PanicAlert("Ayşe", "conv-1")
	if msg.Android == nil || msg.Android.Priority != "high" {
		t.Error("Android priority \"high\" değil — panik bildirimi sessiz modda kaçabilir")
	}
	if msg.APNS == nil || msg.APNS.Headers["apns-priority"] != "10" {
		t.Error("APNS apns-priority \"10\" değil — iOS'ta gecikmeli teslim olabilir")
	}
}
