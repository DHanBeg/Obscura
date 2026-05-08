import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./pages/**/*.{js,ts,jsx,tsx,mdx}",
    "./components/**/*.{js,ts,jsx,tsx,mdx}",
    "./app/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  theme: {
    extend: {
      colors: {
        void:    "#080808",
        ground:  "#0e0e0e",
        surface: "#111111",
        raised:  "#171717",
        border:  "#1f1f1f",
        muted:   "#2a2a2a",
        dim:     "#444444",
        sub:     "#888888",
        body:    "#c8c8c8",
        head:    "#f0f0f0",
        white:   "#ffffff",
        // Obscura accent
        accent:  {
          DEFAULT: "#5ec46e",
          dim:     "#257830",
          glow:    "#5ec46e33",
          deep:    "#0d2211",
        },
        // Tier colors
        tier1: "#888888",
        tier2: "#4a9eff",
        tier3: "#a78bfa",
        tier4: "#f59e0b",
        tier5: "#5ec46e",
        // Danger
        red:   "#ef4444",
        amber: "#f59e0b",
      },
      fontFamily: {
        sans: ["var(--font-sans)", "system-ui", "sans-serif"],
        mono: ["var(--font-mono)", "monospace"],
      },
      borderRadius: {
        "2xl": "1rem",
        "3xl": "1.5rem",
        "4xl": "2rem",
        pill: "9999px",
      },
      boxShadow: {
        "void-sm":  "0 2px 8px rgba(0,0,0,0.6)",
        "void-md":  "0 4px 24px rgba(0,0,0,0.7)",
        "void-lg":  "0 8px 48px rgba(0,0,0,0.8)",
        "accent-glow": "0 0 24px rgba(94,196,110,0.15)",
        "accent-ring": "0 0 0 1.5px rgba(94,196,110,0.35)",
      },
      animation: {
        "fade-in":    "fadeIn 0.2s ease",
        "slide-up":   "slideUp 0.3s cubic-bezier(0.16,1,0.3,1)",
        "slide-down": "slideDown 0.25s cubic-bezier(0.16,1,0.3,1)",
        "scale-in":   "scaleIn 0.2s cubic-bezier(0.16,1,0.3,1)",
        "pulse-soft": "pulseSoft 3s ease-in-out infinite",
        "float":      "float 6s ease-in-out infinite",
        "shimmer":    "shimmer 1.5s ease-in-out infinite",
        "spin-slow":  "spin 8s linear infinite",
      },
      keyframes: {
        fadeIn:    { from: { opacity: "0" }, to: { opacity: "1" } },
        slideUp:   { from: { opacity: "0", transform: "translateY(12px)" }, to: { opacity: "1", transform: "translateY(0)" } },
        slideDown: { from: { opacity: "0", transform: "translateY(-8px)" }, to: { opacity: "1", transform: "translateY(0)" } },
        scaleIn:   { from: { opacity: "0", transform: "scale(0.94)" }, to: { opacity: "1", transform: "scale(1)" } },
        pulseSoft: { "0%,100%": { opacity: "0.4" }, "50%": { opacity: "0.8" } },
        float:     { "0%,100%": { transform: "translateY(0)" }, "50%": { transform: "translateY(-8px)" } },
        shimmer:   { "0%": { backgroundPosition: "-200% 0" }, "100%": { backgroundPosition: "200% 0" } },
      },
      transitionTimingFunction: {
        spring: "cubic-bezier(0.16,1,0.3,1)",
        snappy: "cubic-bezier(0.4,0,0.2,1)",
      },
      spacing: {
        "safe-b": "env(safe-area-inset-bottom)",
        "safe-t": "env(safe-area-inset-top)",
      },
      backdropBlur: {
        xs: "2px",
      },
    },
  },
  plugins: [],
};

export default config;
