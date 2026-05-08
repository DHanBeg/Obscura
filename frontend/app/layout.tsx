import type { Metadata, Viewport } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Obscura",
  description: "Zero-Knowledge Secure Messaging",
  manifest: "/manifest.json",
  icons: { icon: "/favicon.ico", apple: "/apple-icon.png" },
  appleWebApp: { capable: true, statusBarStyle: "black-translucent", title: "Obscura" },
};

export const viewport: Viewport = {
  themeColor: "#080808",
  width: "device-width",
  initialScale: 1,
  maximumScale: 1,
  userScalable: false,
  viewportFit: "cover",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="tr" className="dark">
      <head>
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
        <link
          href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&display=swap"
          rel="stylesheet"
        />
      </head>
      <body className="void-bg antialiased">
        {children}
      </body>
    </html>
  );
}
