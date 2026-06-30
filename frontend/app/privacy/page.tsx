"use client";

import { ChevronLeft } from "lucide-react";
import { useRouter } from "next/navigation";
import { AppShell } from "@/components/AppShell";

export default function PrivacyPage() {
  const router = useRouter();

  return (
    <AppShell>
      <div className="flex flex-col h-full">
        {/* Header */}
        <div className="page-header">
          <button onClick={() => router.back()} className="btn-icon" aria-label="Geri">
            <ChevronLeft size={20} />
          </button>
          <span className="page-title">Gizlilik Politikası</span>
          <div className="w-10" />
        </div>

        {/* Content */}
        <div className="scroll-area flex-1 px-4 py-6">
          <div style={{ maxWidth: 600, marginLeft: "auto", marginRight: "auto" }}>
            <p className="text-xs mb-6" style={{ color: "var(--text-3)" }}>
              Son güncelleme: Haziran 2026
            </p>

            <Section title="1. Giriş">
              Obscura ("biz", "uygulama"), kullanıcıların gizliliğini en üst düzeyde korumayı temel ilke olarak benimser. Bu Gizlilik Politikası, Obscura uygulamasını kullanırken hangi verilerin işlendiğini, nasıl korunduğunu ve haklarınızı açıklar.
              {"\n\n"}
              Obscura, Sıfır Bilgi Kanıtı (ZK Proof) teknolojisi üzerine inşa edilmiş bir mesajlaşma ve kimlik protokolüdür. Temel tasarım amacımız: kimliğinizi doğrulayabilmek için kimliğinizi ifşa etmemek.
            </Section>

            <Section title="2. Toplanan Veriler">
              <SubTitle>2.1 Telefon Numarası</SubTitle>
              Telefon numaranız yalnızca hesap doğrulaması için kullanılır. Tek seferlik doğrulama kodu (OTP) gönderildikten sonra, numaranız şifreli olarak saklanır ve hiçbir üçüncü tarafla paylaşılmaz. Pazarlama veya reklam amacıyla kullanılmaz.

              <SubTitle>2.2 Merkeziyetsiz Kimlik (DID)</SubTitle>
              Hesabınız bir DID (Decentralized Identifier) ile temsil edilir. Bu tanımlayıcı gerçek kimliğinize bağlı değildir; cihazınızda üretilen kriptografik bir değerden türetilir.

              <SubTitle>2.3 Anımsatıcı Kelimeler (Mnemonic)</SubTitle>
              12 kelimelik kurtarma ifadeniz yalnızca cihazınızda saklanır. Bu ifade hiçbir zaman sunucularımıza gönderilmez. Biz dahil hiç kimse bu ifadeye erişemez.

              <SubTitle>2.4 Mesajlar</SubTitle>
              Tüm mesajlar gönderimden önce cihazınızda uçtan uca şifrelenir (Signal Protokolü: X3DH + Double Ratchet). Sunucularımız mesajlarınızın içeriğini göremez.

              <SubTitle>2.5 Teknik Veriler</SubTitle>
              Hizmetin düzgün çalışması için gerekli minimum teknik veriler (bağlantı zamanı, hata logları) tutulabilir. Bu veriler kişisel bilgi içermez ve 30 gün içinde silinir.
            </Section>

            <Section title="3. ZK Kanıtları ve Kimlik">
              Obscura, kimlik doğrulamasında Groth16 tabanlı Sıfır Bilgi Kanıtı kullanır. Bu sistem sayesinde:
              {"\n"}• Gerçek kimliğinizi açıklamadan sizi doğrulayabiliriz.{"\n"}
              • Kanıtlar tek kullanımlıktır (nullifier mekanizması).{"\n"}
              • Kanıt oluşturma işlemi tamamen cihazınızda gerçekleşir.{"\n"}
              • Sunucular yalnızca matematiksel geçerliliği doğrular; kim olduğunuzu bilemez.
            </Section>

            <Section title="4. Veri Güvenliği">
              <SubTitle>4.1 Şifreleme</SubTitle>
              • Mesajlar: AES-256-GCM (uçtan uca){"\n"}
              • Kimlik anahtarları: X25519 ECDH + Ed25519{"\n"}
              • Post-kuantum katman: Kyber-768 + Dilithium3{"\n"}
              • Depolama: yerel şifreli depolama (cihaz bazlı)

              <SubTitle>4.2 Sunucu Güvenliği</SubTitle>
              Sunucularımız hiçbir özel anahtarınızı veya mesaj içeriğinizi göremez. Tek açığa çıkan veri: şifrelenmiş mesaj zarfı ve zaman damgasıdır.

              <SubTitle>4.3 Denetim</SubTitle>
              Obscura güvenlik kodu bağımsız güvenlik firmaları tarafından denetlenmiştir. Denetim raporları uygulamada kamuya açık olarak paylaşılmaktadır.
            </Section>

            <Section title="5. Veri Saklama">
              • Mesajlar: cihazınızda, siz silinceye kadar.{"\n"}
              • Sunucu tarafındaki geçici mesaj kaydı: teslimattan sonra otomatik silinir.{"\n"}
              • OTP kayıtları: 5 dakika (otomatik silinir).{"\n"}
              • Hesap kapatma: tüm veriler 30 gün içinde kalıcı olarak silinir.
            </Section>

            <Section title="6. Üçüncü Taraf Paylaşımı">
              Kişisel verilerinizi reklam, analitik veya pazarlama amaçlı hiçbir üçüncü tarafla paylaşmıyoruz. Yalnızca zorunlu hizmet altyapısı (SMS doğrulama sağlayıcısı) için minimum veri aktarımı yapılır ve bu aktarım sözleşmeyle kısıtlanmıştır.
            </Section>

            <Section title="7. Haklarınız (KVKK)">
              6698 Sayılı Kişisel Verilerin Korunması Kanunu kapsamında:{"\n"}
              • Verilerinize erişim hakkı{"\n"}
              • Düzeltme hakkı{"\n"}
              • Silme hakkı ("unutulma hakkı"){"\n"}
              • İşlemeye itiraz hakkı{"\n"}
              • Veri taşınabilirliği hakkı{"\n\n"}
              Bu haklarınızı kullanmak için uygulama içinden bizimle iletişime geçebilirsiniz.
            </Section>

            <Section title="8. Çerezler ve İzleme">
              Obscura hiçbir izleme çerezi kullanmaz. Davranışsal analitik, reklam takibi veya parmak izi toplama yapılmaz.
            </Section>

            <Section title="9. Çocukların Gizliliği">
              Obscura 16 yaşın altındaki bireyler için tasarlanmamıştır. 16 yaş altı kullanıcıların verileri tespit edilmesi durumunda derhal silinir.
            </Section>

            <Section title="10. Değişiklikler">
              Bu politikada yapılan önemli değişiklikler uygulama içi bildirimle duyurulur. Değişikliği kabul etmiyorsanız hesabınızı kapatabilirsiniz.
            </Section>

            <Section title="11. İletişim">
              Gizlilik ile ilgili sorularınız için uygulama içindeki destek kanalından veya Ayarlar {">"} Hakkında {">"} İletişim bölümünden bize ulaşabilirsiniz.
            </Section>

            <div className="mt-8 pt-6" style={{ borderTop: "1px solid var(--border-1)" }}>
              <p className="text-[11px] text-center" style={{ color: "var(--text-3)" }}>
                Bu politika Türk hukuku kapsamında hazırlanmıştır.{"\n"}Obscura, açık kaynak kodlu bir projedir.
              </p>
            </div>
          </div>
        </div>
      </div>
    </AppShell>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="mb-6">
      <h2
        className="mb-3"
        style={{
          fontFamily: "var(--font-display)",
          fontSize: 15,
          fontWeight: 700,
          color: "var(--text-1)",
          letterSpacing: "-0.01em",
        }}
      >
        {title}
      </h2>
      <p
        className="leading-relaxed whitespace-pre-line"
        style={{ fontSize: 13, color: "var(--text-2)", lineHeight: 1.75 }}
      >
        {children}
      </p>
    </div>
  );
}

function SubTitle({ children }: { children: React.ReactNode }) {
  return (
    <span
      style={{
        display: "block",
        fontFamily: "var(--font-display)",
        fontWeight: 600,
        fontSize: 13,
        color: "var(--text-1)",
        marginTop: 12,
        marginBottom: 4,
      }}
    >
      {children}
    </span>
  );
}
