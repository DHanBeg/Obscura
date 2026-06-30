"use client";

import { ChevronLeft } from "lucide-react";
import { useRouter } from "next/navigation";
import { AppShell } from "@/components/AppShell";

export default function TermsPage() {
  const router = useRouter();

  return (
    <AppShell>
      <div className="flex flex-col h-full">
        {/* Header */}
        <div className="page-header">
          <button onClick={() => router.back()} className="btn-icon" aria-label="Geri">
            <ChevronLeft size={20} />
          </button>
          <span className="page-title">Kullanım Koşulları</span>
          <div className="w-10" />
        </div>

        {/* Content */}
        <div className="scroll-area flex-1 px-4 py-6">
          <div style={{ maxWidth: 600, marginLeft: "auto", marginRight: "auto" }}>
            <p className="text-xs mb-6" style={{ color: "var(--text-3)" }}>
              Son güncelleme: Haziran 2026
            </p>

            <Section title="1. Kabul">
              Obscura uygulamasını kullanarak bu Kullanım Koşullarını kabul etmiş sayılırsınız. Koşulları kabul etmiyorsanız uygulamayı kullanmayınız.
            </Section>

            <Section title="2. Hizmet Tanımı">
              Obscura, Sıfır Bilgi Kanıtı (ZK Proof) teknolojisi kullanan, uçtan uca şifreli bir mesajlaşma ve kimlik platformudur. Şunları sunar:{"\n"}
              • Şifreli bireysel ve grup mesajlaşma{"\n"}
              • ZK tabanlı kimlik doğrulama{"\n"}
              • Güvenli sesli ve görüntülü arama{"\n"}
              • Merkeziyetsiz kimlik (DID) yönetimi{"\n"}
              • Topluluk yönetimi ve oylama{"\n"}
              • Kripto varlık transferleri (şeffaf ve gizli)
            </Section>

            <Section title="3. Hesap ve Güvenlik">
              <SubTitle>3.1 Hesap Oluşturma</SubTitle>
              Hesabınız telefon numaranızla doğrulanır. 12 kelimelik anımsatıcı ifadenizi güvenli bir yerde saklamak tamamen sizin sorumluluğunuzdadır. Bu ifadenin kaybolması durumunda hesabınıza erişim sağlanamaz.

              <SubTitle>3.2 Hesap Güvenliği</SubTitle>
              Hesabınızın güvenliğinden siz sorumlusunuz. Oturum bilgilerinizi, anımsatıcı ifadenizi veya özel anahtarlarınızı kimseyle paylaşmayınız. Şüpheli bir erişim fark ettiğinizde bizi derhal bildiriniz.

              <SubTitle>3.3 Hesap Devri</SubTitle>
              Obscura hesapları devredilemez. Kimlik doğrulaması kriptografik anahtara dayalıdır.
            </Section>

            <Section title="4. Kabul Edilemez Kullanım">
              Aşağıdaki kullanımlar kesinlikle yasaktır:{"\n"}
              • Yasadışı içerik paylaşımı veya dağıtımı{"\n"}
              • Başkalarını taciz etme, tehdit etme veya nefret söylemi{"\n"}
              • Spam, bot veya otomatik kötüye kullanım{"\n"}
              • Fikri mülkiyet ihlali{"\n"}
              • Platformun altyapısına zarar verme girişimleri{"\n"}
              • Başkasının kimliğine bürünme{"\n"}
              • 16 yaşın altındaki bireylere yönelik zararlı içerik{"\n\n"}
              Bu ihlaller tespit edilmesi durumunda hesabınız askıya alınabilir veya kalıcı olarak kapatılabilir.
            </Section>

            <Section title="5. Gizlilik">
              Kişisel verilerinizin nasıl işlendiği hakkında bilgi almak için lütfen Gizlilik Politikamızı inceleyin. Gizlilik Politikası bu Kullanım Koşullarının ayrılmaz bir parçasıdır.
            </Section>

            <Section title="6. Kripto Varlıklar ve Transferler">
              Obscura içindeki kripto varlık işlemleri blok zinciri üzerinde gerçekleşir ve geri alınamaz. Yanlış adrese gönderilen varlıklar kurtarılamaz. Kullanıcılar transfer yapmadan önce adresi doğrulamakla yükümlüdür.{"\n\n"}
              Obscura bir kripto para borsası veya aracı kurum değildir. Varlık değer kayıplarından sorumlu tutulamaz.
            </Section>

            <Section title="7. Hizmetin Sürekliliği">
              Obscura, hizmetin kesintisiz ve hatasız çalışacağını garanti etmez. Ağ sorunları, bakım çalışmaları veya beklenmeyen durumlar nedeniyle hizmet geçici olarak aksayabilir.{"\n\n"}
              Ağ varlığı açık kaynak koda dayalı olduğundan, dağıtık yapı sayesinde tek bir tarafın hizmet aksaklığı tüm ağı etkilemez.
            </Section>

            <Section title="8. Sorumluluk Sınırı">
              Obscura, kullanıcılar arasında iletilen içerikten sorumlu değildir. Kullanıcılar kendi gönderdikleri içerikten tamamen sorumludur. Uçtan uca şifreleme nedeniyle içerik denetimi teknik olarak mümkün değildir; ancak kullanıcı şikâyeti üzerine hesaplar incelenebilir.
            </Section>

            <Section title="9. Fikri Mülkiyet">
              Obscura'nın açık kaynak bileşenleri ilgili lisanslar kapsamındadır (MIT, Apache 2.0). Ticari marka ve uygulama tasarımı Obscura'ya aittir. Açık kaynak lisans bilgileri için Ayarlar {">"} Hakkında bölümüne bakınız.
            </Section>

            <Section title="10. Değişiklikler">
              Bu koşullar önceden bildirimle değiştirilebilir. Önemli değişiklikler uygulamada bildirimle duyurulur. Değişikliği kabul etmiyorsanız hesabınızı kapatabilirsiniz.
            </Section>

            <Section title="11. Uygulanacak Hukuk">
              Bu sözleşme Türk hukukuna tabidir. Anlaşmazlıklar İstanbul Mahkemelerinde çözümlenir.
            </Section>

            <Section title="12. İletişim">
              Kullanım koşulları hakkında sorularınız için Ayarlar {">"} Hakkında {">"} İletişim bölümünden bize ulaşabilirsiniz.
            </Section>

            <div className="mt-8 pt-6" style={{ borderTop: "1px solid var(--border-1)" }}>
              <p className="text-[11px] text-center" style={{ color: "var(--text-3)" }}>
                Bu koşullar Türk hukuku çerçevesinde hazırlanmıştır.{"\n"}Obscura açık kaynaklı bir protokoldür.
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
