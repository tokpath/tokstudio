import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./app/**/*.{ts,tsx}", "./lib/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: ["var(--font-sans)", "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ["var(--font-mono)", "ui-monospace", "SFMono-Regular", "monospace"],
      },
      colors: {
        th: {
          fg: "var(--th-fg)",
          muted: "var(--th-muted)",
          faint: "var(--th-faint)",
          surface: "var(--th-surface-solid)",
          elevated: "var(--th-elevated)",
          border: "var(--th-border)",
          primary: "var(--brand-primary)",
        },
      },
      boxShadow: {
        glow: "0 0 0 1px var(--th-border), 0 18px 50px -24px rgba(0,0,0,0.7)",
        "glow-brand": "0 0 0 1px color-mix(in srgb, var(--brand-primary) 35%, transparent), 0 16px 40px -20px var(--th-glow)",
      },
      keyframes: {
        "th-enter": {
          from: { opacity: "0", transform: "translateY(8px)" },
          to: { opacity: "1", transform: "translateY(0)" },
        },
        "th-marquee": {
          from: { transform: "translateX(0)" },
          to: { transform: "translateX(-50%)" },
        },
      },
      animation: {
        "th-enter": "th-enter 420ms ease-out both",
        "th-marquee": "th-marquee 28s linear infinite",
      },
    },
  },
  plugins: [],
};

export default config;
