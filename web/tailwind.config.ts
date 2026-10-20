import type { Config } from "tailwindcss";

// The alpha placeholder lets Tailwind emit /opacity utilities while keeping
// runtime OEM colors in hex CSS variables.
const semanticColor = (name: string) => `color-mix(in srgb, var(--${name}) calc(<alpha-value> * 100%), transparent)`;

const config: Config = {
  darkMode: "class",
  content: {
    relative: true,
    files: ["./app/**/*.{ts,tsx}", "./lib/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}"],
  },
  theme: {
    extend: {
      fontFamily: {
        sans: [
          "var(--font-sans)",
          "PingFang SC",
          "Hiragino Sans",
          "Noto Sans SC",
          "Noto Sans JP",
          "system-ui",
          "sans-serif",
        ],
        mono: ["var(--font-mono)", "ui-monospace", "monospace"],
      },
      colors: Object.fromEntries([
        "canvas", "canvas-raised", "ink", "ink-secondary", "ink-mute", "hairline", "brand", "brand-press", "brand-soft", "brand-emphasis", "on-brand", "success", "hold", "danger", "degraded", "code", "code-ink", "code-hairline", "scrim",
      ].map(name => [name, semanticColor(name)])),
      borderRadius: {
        stamp: "10px",
        card: "16px",
        control: "8px",
      },
    },
  },
  plugins: [],
};

export default config;
