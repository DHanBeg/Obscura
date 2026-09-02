import type { Metadata, Viewport } from "next";
import { Inter } from "next/font/google";
import { cssVars } from "@obscura/theme";
import { ToastProvider } from "@/components/Toast";
import "./globals.css";

// Non-Apple fallback for --font-sans/--font-display (SF system stack, see
// globals.css). Only loads a font FILE for the non-Apple case; Apple devices
// resolve -apple-system/BlinkMacSystemFont first and never touch this.
const inter = Inter({
  subsets: ["latin"],
  variable: "--font-inter",
  weight: ["300", "400", "500", "600", "700"],
  display: "swap",
  adjustFontFallback: false,
});

export const metadata: Metadata = {
  title: "Obscura",
  description: "Zero-Knowledge Secure Messaging",
  manifest: "/manifest.json",
  icons: { icon: "/logo.jpeg", apple: "/logo.jpeg" },
  appleWebApp: { capable: true, statusBarStyle: "black-translucent", title: "Obscura" },
};

export const viewport: Viewport = {
  themeColor: "#0A0A0C",
  width: "device-width",
  initialScale: 1,
  maximumScale: 1,
  userScalable: false,
  viewportFit: "cover",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html
      lang="tr"
      className={`dark ${inter.variable}`}
    >
      <body className="void-bg antialiased">
        {/* #30 — packages/theme (@obscura/theme) TEK KAYNAK, marketplace
            ekranları var(--color-*)/var(--spacing-*)/var(--radius-*)/
            var(--text-*) tüketir. globals.css'in mevcut --accent/--bg
            token'larına DOKUNMUYOR (ayrı isim uzayı, --color-* öneki) —
            sadece marketplace bileşenleri bu yeni değişkenleri kullanır. */}
        <style dangerouslySetInnerHTML={{ __html: cssVars() }} />
        <ToastProvider>{children}</ToastProvider>
      </body>
    </html>
  );
}
