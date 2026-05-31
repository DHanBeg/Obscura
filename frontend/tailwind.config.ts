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
        /* Depth layers */
        void:        "#020208",
        ground:      "#040410",
        surface:     "#07071a",
        "surface-2": "#0c0c22",
        "surface-3": "#11112c",
        overlay:     "#171736",
        /* Borders */
        border:      "rgba(255,255,255,0.045)",
        "border-2":  "rgba(255,255,255,0.085)",
        "border-3":  "rgba(255,255,255,0.13)",
        /* Text */
        "text-1":    "#eef0ff",
        "text-2":    "#8090b8",
        "text-3":    "#404466",
        "text-4":    "#1e2040",
        /* Accent — Electric Mint */
        accent: {
          DEFAULT: "#00e5a0",
          soft:    "#00c48a",
          muted:   "rgba(0,229,160,0.15)",
          glow:    "rgba(0,229,160,0.10)",
          deep:    "rgba(0,229,160,0.04)",
        },
        /* Signal — Electric Blue */
        signal: {
          DEFAULT: "#4da8ff",
          muted:   "rgba(77,168,255,0.12)",
        },
        /* Semantic */
        success: "#00e5a0",
        warning: "#ffaa00",
        error:   "#ff4058",
        info:    "#4da8ff",
        /* Tier badges */
        tier1: "#6b7380",
        tier2: "#4da8ff",
        tier3: "#a78bfa",
        tier4: "#ffaa00",
        tier5: "#00e5a0",
      },
      fontFamily: {
        display: ["var(--font-display)", "system-ui", "sans-serif"],
        sans:    ["var(--font-sans)", "system-ui", "sans-serif"],
        mono:    ["var(--font-mono)", "monospace"],
      },
      borderRadius: {
        "2xl": "16px",
        "3xl": "20px",
        "4xl": "24px",
        "5xl": "32px",
        pill:  "9999px",
      },
      boxShadow: {
        "1":      "0 1px 3px rgba(0,0,0,0.7), 0 1px 2px rgba(0,0,0,0.5)",
        "2":      "0 4px 16px rgba(0,0,0,0.75), 0 2px 4px rgba(0,0,0,0.6)",
        "3":      "0 8px 32px rgba(0,0,0,0.8), 0 4px 8px rgba(0,0,0,0.7)",
        "4":      "0 20px 60px rgba(0,0,0,0.9), 0 8px 24px rgba(0,0,0,0.8)",
        "accent": "0 0 20px rgba(0,229,160,0.2), 0 0 60px rgba(0,229,160,0.06)",
        "signal": "0 0 20px rgba(77,168,255,0.2), 0 0 60px rgba(77,168,255,0.06)",
      },
      animation: {
        "fade-in":      "fadeIn 150ms ease both",
        "slide-up":     "slideUp 280ms cubic-bezier(0.16,1,0.3,1) both",
        "slide-down":   "slideDown 150ms cubic-bezier(0.16,1,0.3,1) both",
        "slide-left":   "slideLeft 280ms cubic-bezier(0.16,1,0.3,1) both",
        "scale-in":     "scaleIn 150ms cubic-bezier(0.16,1,0.3,1) both",
        "float":        "float 6s ease-in-out infinite",
        "pulse-soft":   "pulseSoft 3s ease-in-out infinite",
        "pulse-accent": "pulseAccent 2s ease-in-out infinite",
        "spin-slow":    "spin 10s linear infinite",
        "shimmer":      "shimmer 2s ease-in-out infinite",
        "orb":          "orb 12s ease-in-out infinite",
      },
      keyframes: {
        fadeIn:    { from: { opacity: "0" }, to: { opacity: "1" } },
        slideUp:   { from: { opacity: "0", transform: "translateY(14px)" }, to: { opacity: "1", transform: "translateY(0)" } },
        slideDown: { from: { opacity: "0", transform: "translateY(-10px)" }, to: { opacity: "1", transform: "translateY(0)" } },
        slideLeft: { from: { opacity: "0", transform: "translateX(16px)" }, to: { opacity: "1", transform: "translateX(0)" } },
        scaleIn:   { from: { opacity: "0", transform: "scale(0.93)" }, to: { opacity: "1", transform: "scale(1)" } },
        shimmer:   { "0%": { backgroundPosition: "-200% 0" }, "100%": { backgroundPosition: "200% 0" } },
        pulseSoft: { "0%,100%": { opacity: "0.35" }, "50%": { opacity: "0.7" } },
        pulseAccent: {
          "0%,100%": { boxShadow: "0 0 0 0 rgba(0,229,160,0.4)" },
          "50%":     { boxShadow: "0 0 0 8px rgba(0,229,160,0)" },
        },
        float: { "0%,100%": { transform: "translateY(0)" }, "50%": { transform: "translateY(-6px)" } },
        orb: {
          "0%":   { transform: "translate(0, 0) scale(1)" },
          "33%":  { transform: "translate(20px, -15px) scale(1.05)" },
          "66%":  { transform: "translate(-10px, 10px) scale(0.97)" },
          "100%": { transform: "translate(0, 0) scale(1)" },
        },
      },
      transitionTimingFunction: {
        "out-expo": "cubic-bezier(0.16, 1, 0.3, 1)",
        "in-expo":  "cubic-bezier(0.7, 0, 0.84, 0)",
        "in-out":   "cubic-bezier(0.65, 0, 0.35, 1)",
        "spring":   "cubic-bezier(0.16, 1, 0.3, 1)",
      },
      transitionDuration: {
        "80":  "80ms",
        "150": "150ms",
        "280": "280ms",
        "420": "420ms",
      },
      spacing: {
        "safe-b": "env(safe-area-inset-bottom)",
        "safe-t": "env(safe-area-inset-top)",
      },
      backdropBlur: {
        xs:   "4px",
        xl:   "24px",
        "2xl": "40px",
      },
    },
  },
  plugins: [],
};

export default config;
