module.exports = {
  plugins: {
    tailwindcss: { config: require("node:path").join(__dirname, "tailwind.config.ts") },
    autoprefixer: {},
  },
};
