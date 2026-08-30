import type { Config } from "tailwindcss";

const config: Config = {
  darkMode: "class",
  content: ["./app/**/*.{ts,tsx}", "./lib/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        canvas: "var(--canvas)",
        "canvas-raised": "var(--canvas-raised)",
        ink: "var(--ink)",
        "ink-secondary": "var(--ink-secondary)",
        "ink-mute": "var(--ink-mute)",
        hairline: "var(--hairline)",
        brand: "var(--brand)",
        "brand-press": "var(--brand-press)",
        "brand-soft": "var(--brand-soft)",
        "brand-emphasis": "var(--brand-emphasis)",
        "on-brand": "var(--on-brand)",
        success: "var(--success)",
        hold: "var(--hold)",
        danger: "var(--danger)",
        degraded: "var(--degraded)",
      },
      borderRadius: {
        stamp: "6px",
        control: "4px",
      },
      fontFamily: {
        sans: ["var(--font-sans)", "system-ui", "sans-serif"],
        mono: ["var(--font-mono)", "ui-monospace", "monospace"],
      },
    },
  },
  plugins: [],
};

export default config;
