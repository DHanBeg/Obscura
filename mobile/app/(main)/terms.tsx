import React from "react";
import { View, Text, ScrollView, TouchableOpacity, StyleSheet } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, typography } from "@/lib/theme";

function Section({ title, children }: { title: string; children: string }) {
  return (
    <View style={styles.section}>
      <Text style={styles.sectionTitle}>{title}</Text>
      <Text style={styles.sectionBody}>{children}</Text>
    </View>
  );
}

export default function TermsScreen() {
  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Kullanım Koşulları</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        <Text style={styles.meta}>Son güncelleme: Haziran 2026</Text>

        <Section title="1. Kabul">
          {"Obscura uygulamasını kullanarak bu Kullanım Koşullarını kabul etmiş sayılırsınız. Koşulları kabul etmiyorsanız uygulamayı kullanmayınız."}
        </Section>

        <Section title="2. Hizmet Tanımı">
          {`Obscura, ZK Proof teknolojisi kullanan, uçtan uca şifreli bir mesajlaşma ve kimlik platformudur:\n• Şifreli bireysel ve grup mesajlaşma\n• ZK tabanlı kimlik doğrulama\n• Güvenli sesli ve görüntülü arama\n• Merkeziyetsiz kimlik (DID) yönetimi\n• Topluluk yönetimi ve oylama\n• Kripto varlık transferleri (şeffaf ve gizli)`}
        </Section>

        <Section title="3. Hesap ve Güvenlik">
          {`3.1 Hesap Oluşturma\nHesabınız telefon numaranızla doğrulanır. 12 kelimelik anımsatıcı ifadenizi güvenli bir yerde saklamak tamamen sizin sorumluluğunuzdadır.\n\n3.2 Hesap Güvenliği\nOturum bilgilerinizi, anımsatıcı ifadenizi veya özel anahtarlarınızı kimseyle paylaşmayınız.\n\n3.3 Hesap Devri\nObscura hesapları devredilemez. Kimlik doğrulaması kriptografik anahtara dayalıdır.`}
        </Section>

        <Section title="4. Kabul Edilemez Kullanım">
          {`Aşağıdaki kullanımlar kesinlikle yasaktır:\n• Yasadışı içerik paylaşımı veya dağıtımı\n• Başkalarını taciz etme veya tehdit etme\n• Spam, bot veya otomatik kötüye kullanım\n• Fikri mülkiyet ihlali\n• Platformun altyapısına zarar verme girişimleri\n• Başkasının kimliğine bürünme\n• 16 yaş altındaki bireylere yönelik zararlı içerik`}
        </Section>

        <Section title="5. Gizlilik">
          {"Kişisel verilerinizin nasıl işlendiği hakkında bilgi almak için Gizlilik Politikamızı inceleyin."}
        </Section>

        <Section title="6. Kripto Varlıklar">
          {"Obscura içindeki kripto varlık işlemleri blok zinciri üzerinde gerçekleşir ve geri alınamaz. Yanlış adrese gönderilen varlıklar kurtarılamaz. Obscura bir kripto para borsası değildir."}
        </Section>

        <Section title="7. Sorumluluk Sınırı">
          {"Obscura, kullanıcılar arasında iletilen içerikten sorumlu değildir. Uçtan uca şifreleme nedeniyle içerik denetimi teknik olarak mümkün değildir; ancak kullanıcı şikâyeti üzerine hesaplar incelenebilir."}
        </Section>

        <Section title="8. Fikri Mülkiyet">
          {"Obscura'nın açık kaynak bileşenleri ilgili lisanslar kapsamındadır (MIT, Apache 2.0). Ticari marka ve uygulama tasarımı Obscura'ya aittir."}
        </Section>

        <Section title="9. Uygulanacak Hukuk">
          {"Bu sözleşme Türk hukukuna tabidir. Anlaşmazlıklar İstanbul Mahkemelerinde çözümlenir."}
        </Section>

        <Section title="10. İletişim">
          {"Kullanım koşulları hakkında sorularınız için Ayarlar › Hakkında › İletişim bölümünden bize ulaşabilirsiniz."}
        </Section>

        <View style={styles.footer}>
          <Text style={styles.footerText}>
            Bu koşullar Türk hukuku çerçevesinde hazırlanmıştır.{"\n"}Obscura açık kaynaklı bir protokoldür.
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
    fontSize: typography.sm, fontWeight: "700", color: colors.head, marginBottom: spacing.sm,
  },
  sectionBody: { fontSize: typography.sm, color: colors.body, lineHeight: 22 },
  footer: {
    marginTop: spacing.xl, paddingTop: spacing.lg,
    borderTopWidth: 1, borderTopColor: colors.border,
  },
  footerText: { fontSize: typography.xs, color: colors.dim, textAlign: "center", lineHeight: 18 },
});
