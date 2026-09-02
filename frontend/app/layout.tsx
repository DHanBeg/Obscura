import type { Metadata, Viewport } from "next";
import { Space_Grotesk, JetBrains_Mono } from "next/font/google";
import { cssVars } from "@obscura/theme";
import { ToastProvider } from "@/components/Toast";
import "./globals.css";

const spaceGrotesk = Space_Grotesk({
  subsets: ["latin"],
  variable: "--font-sans",
  weight: ["300", "400", "500", "600", "700"],
  display: "swap",
  adjustFontFallback: false,
});

// Reuse Space Grotesk for display as well (--font-display)
const spaceGroteskDisplay = Space_Grotesk({
  subsets: ["latin"],
  variable: "--font-display",
  weight: ["300", "400", "500", "600", "700"],
  display: "swap",
  adjustFontFallback: false,
});

const jetbrains = JetBrains_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
  weight: ["400", "500", "700"],
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
      className={`dark ${spaceGrotesk.variable} ${spaceGroteskDisplay.variable} ${jetbrains.variable}`}
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
