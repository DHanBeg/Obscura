import React from "react";
import { View, Text, ScrollView, TouchableOpacity, StyleSheet } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";

function Section({ title, children }: { title: string; children: string }) {
  return (
    <View style={styles.section}>
      <Text style={styles.sectionTitle}>{title}</Text>
      <Text style={styles.sectionBody}>{children}</Text>
    </View>
  );
}

export default function PrivacyScreen() {
  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Gizlilik Politikası</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        <Text style={styles.meta}>Son güncelleme: Haziran 2026</Text>

        <Section title="1. Giriş">
          {`Obscura ("biz", "uygulama"), kullanıcıların gizliliğini en üst düzeyde korumayı temel ilke olarak benimser. Bu Gizlilik Politikası, Obscura uygulamasını kullanırken hangi verilerin işlendiğini, nasıl korunduğunu ve haklarınızı açıklar.\n\nObscura, Sıfır Bilgi Kanıtı (ZK Proof) teknolojisi üzerine inşa edilmiş bir mesajlaşma ve kimlik protokolüdür. Temel tasarım amacımız: kimliğinizi doğrulayabilmek için kimliğinizi ifşa etmemek.`}
        </Section>

        <Section title="2. Toplanan Veriler">
          {`2.1 Telefon Numarası\nTelefon numaranız yalnızca hesap doğrulaması için kullanılır. OTP gönderildikten sonra numaranız şifreli olarak saklanır ve hiçbir üçüncü tarafla paylaşılmaz.\n\n2.2 Merkeziyetsiz Kimlik (DID)\nHesabınız bir DID ile temsil edilir. Bu tanımlayıcı gerçek kimliğinize bağlı değildir.\n\n2.3 Anımsatıcı Kelimeler\n12 kelimelik kurtarma ifadeniz yalnızca cihazınızda saklanır. Sunucularımıza gönderilmez.\n\n2.4 Mesajlar\nTüm mesajlar gönderimden önce cihazınızda uçtan uca şifrelenir (Signal Protokolü: X3DH + Double Ratchet).\n\n2.5 Teknik Veriler\nHizmetin düzgün çalışması için gerekli minimum teknik veriler 30 gün içinde silinir.`}
        </Section>

        <Section title="3. ZK Kanıtları ve Kimlik">
          {`Obscura, kimlik doğrulamasında Groth16 tabanlı Sıfır Bilgi Kanıtı kullanır:\n• Gerçek kimliğinizi açıklamadan sizi doğrulayabiliriz.\n• Kanıtlar tek kullanımlıktır (nullifier mekanizması).\n• Kanıt oluşturma tamamen cihazınızda gerçekleşir.\n• Sunucular yalnızca matematiksel geçerliliği doğrular.`}
        </Section>

        <Section title="4. Veri Güvenliği">
          {`• Mesajlar: AES-256-GCM (uçtan uca)\n• Kimlik anahtarları: X25519 ECDH + Ed25519\n• Post-kuantum katman: Kyber-768 + Dilithium3\n• Depolama: yerel şifreli depolama (cihaz bazlı)\n\nSunucularımız hiçbir özel anahtarınızı veya mesaj içeriğinizi göremez.`}
        </Section>

        <Section title="5. Veri Saklama">
          {`• Mesajlar: cihazınızda, siz silinceye kadar.\n• Geçici mesaj kaydı: teslimattan sonra otomatik silinir.\n• OTP kayıtları: 5 dakika.\n• Hesap kapatma: tüm veriler 30 gün içinde silinir.`}
        </Section>

        <Section title="6. Üçüncü Taraf Paylaşımı">
          {"Kişisel verilerinizi reklam, analitik veya pazarlama amaçlı hiçbir üçüncü tarafla paylaşmıyoruz. Yalnızca SMS doğrulama için minimum veri aktarımı yapılır."}
        </Section>

        <Section title="7. Haklarınız (KVKK)">
          {`6698 Sayılı Kanun kapsamında:\n• Verilerinize erişim hakkı\n• Düzeltme hakkı\n• Silme hakkı ("unutulma hakkı")\n• İşlemeye itiraz hakkı\n• Veri taşınabilirliği hakkı`}
        </Section>

        <Section title="8. Çerezler ve İzleme">
          {"Obscura hiçbir izleme çerezi kullanmaz. Davranışsal analitik, reklam takibi veya parmak izi toplama yapılmaz."}
        </Section>

        <Section title="9. Çocukların Gizliliği">
          {"Obscura 16 yaşın altındaki bireyler için tasarlanmamıştır. 16 yaş altı kullanıcıların verileri tespit edilmesi durumunda derhal silinir."}
        </Section>

        <Section title="10. İletişim">
          {"Gizlilik ile ilgili sorularınız için Ayarlar › Hakkında › İletişim bölümünden bize ulaşabilirsiniz."}
        </Section>

        <View style={styles.footer}>
          <Text style={styles.footerText}>
            Bu politika Türk hukuku kapsamında hazırlanmıştır.{"\n"}Obscura açık kaynak kodlu bir projedir.
          </Text>
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: colors.void },
  header: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    paddingHorizontal: spacing.lg, paddingVertical: spacing.md,
    borderBottomWidth: 1, borderBottomColor: colors.border,
  },
  backBtn: { padding: 4, width: 40 },
  headerTitle: { fontSize: typography.base, fontWeight: "700", color: colors.head },
  content: { padding: spacing.lg, paddingBottom: 48 },
  meta: { fontSize: typography.xs, color: colors.sub, marginBottom: spacing.lg },
  section: { marginBottom: spacing.lg },
  sectionTitle: {
    fontSize: typography.sm, fontWeight: "700", color: colors.head,
    marginBottom: spacing.sm,
  },
  sectionBody: {
    fontSize: typography.sm, color: colors.body, lineHeight: 22,
  },
  footer: {
    marginTop: spacing.xl, paddingTop: spacing.lg,
    borderTopWidth: 1, borderTopColor: colors.border,
  },
  footerText: { fontSize: typography.xs, color: colors.dim, textAlign: "center", lineHeight: 18 },
});
