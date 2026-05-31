import type { Metadata, Viewport } from "next";
import { Bricolage_Grotesque, Plus_Jakarta_Sans, JetBrains_Mono } from "next/font/google";
import "./globals.css";

const bricolage = Bricolage_Grotesque({
  subsets: ["latin"],
  variable: "--font-display",
  weight: ["300", "400", "500", "600", "700", "800"],
  display: "swap",
  adjustFontFallback: false,
});

const jakarta = Plus_Jakarta_Sans({
  subsets: ["latin"],
  variable: "--font-sans",
  weight: ["300", "400", "500", "600", "700"],
  display: "swap",
  adjustFontFallback: false,
});

const jetbrains = JetBrains_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
  weight: ["400", "500"],
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
  themeColor: "#020208",
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
      className={`dark ${bricolage.variable} ${jakarta.variable} ${jetbrains.variable}`}
    >
      <body className="void-bg antialiased">
        {children}
      </body>
    </html>
  );
}
