/** 公共站顶栏下拉：对齐 ofox「模型 / 文档 / 资源」信息架构。文案走 messages.mega。 */

export type MegaLink = { href: string; labelKey?: string; hintKey?: string; literal?: string };

export type MegaColumn = { titleKey: string; links: MegaLink[] };

export type MegaMenu = {
  id: string;
  labelKey: string;
  href?: string;
  columns: MegaColumn[];
};

export const MEGA_MENUS: MegaMenu[] = [
  {
    id: "models",
    labelKey: "models",
    href: "/models",
    columns: [
      {
        titleKey: "browse",
        links: [
          { href: "/models", labelKey: "allModels", hintKey: "allModelsHint" },
          { href: "/models?kind=text", labelKey: "textModels", hintKey: "textModelsHint" },
          { href: "/image", labelKey: "imageModels", hintKey: "imageModelsHint" },
          { href: "/video", labelKey: "videoModels", hintKey: "videoModelsHint" },
        ],
      },
      {
        titleKey: "tools",
        links: [
          { href: "/model-finder", labelKey: "finder", hintKey: "finderHint" },
          { href: "/best-value", labelKey: "bestValue", hintKey: "bestValueHint" },
          { href: "/leaderboards/models", labelKey: "leaderboard", hintKey: "leaderboardHint" },
          { href: "/compare", labelKey: "compare", hintKey: "compareHint" },
          { href: "/verify", labelKey: "verify", hintKey: "verifyHint" },
        ],
      },
      {
        titleKey: "hot",
        links: [
          { href: "/models/openai/gpt-5.6-sol", literal: "GPT-5.6 Sol" },
          { href: "/models/anthropic/claude-fable-5", literal: "Claude Fable 5" },
          { href: "/models/google/gemini-3.7-flash", literal: "Gemini 3.7 Flash" },
          { href: "/models/bytedance/seedance-2.5", literal: "Seedance 2.5" },
        ],
      },
    ],
  },
  {
    id: "docs",
    labelKey: "docs",
    href: "/docs",
    columns: [
      {
        titleKey: "start",
        links: [
          { href: "/quickstart", labelKey: "quickstart", hintKey: "quickstartHint" },
          { href: "/docs", labelKey: "devDocs", hintKey: "devDocsHint" },
          { href: "/vibe-coding", labelKey: "vibe", hintKey: "vibeHint" },
        ],
      },
      {
        titleKey: "integrate",
        links: [
          { href: "/vibe-coding", literal: "Claude Code", hintKey: "claudeCodeHint" },
          { href: "/vibe-coding", literal: "Codex", hintKey: "codexHint" },
          { href: "/docs", labelKey: "openaiSdk", hintKey: "openaiSdkHint" },
          { href: "/docs/integrations", labelKey: "cline", hintKey: "clineHint" },
        ],
      },
      {
        titleKey: "reference",
        links: [
          { href: "/docs/develop", labelKey: "errors", hintKey: "errorsHint" },
          { href: "/docs/changelog", labelKey: "changelog", hintKey: "changelogHint" },
        ],
      },
    ],
  },
  {
    id: "resources",
    labelKey: "resources",
    columns: [
      {
        titleKey: "product",
        links: [
          { href: "/desktop", labelKey: "desktop" },
          { href: "/enterprise", labelKey: "enterpriseSvc" },
          { href: "/promo", labelKey: "promo" },
          { href: "/awesome-ofox", labelKey: "awesome" },
          { href: "/pricing", labelKey: "pricing" },
        ],
      },
      {
        titleKey: "trust",
        links: [
          { href: "/trust", labelKey: "trustCenter" },
          { href: "/trust/subprocessors", labelKey: "subprocessors" },
          { href: "/privacy", labelKey: "privacy" },
          { href: "/terms", labelKey: "terms" },
        ],
      },
      {
        titleKey: "content",
        links: [
          { href: "/blog", labelKey: "blog" },
          { href: "/leaderboards/apps", labelKey: "appsBoard" },
          { href: "/leaderboards/labs", labelKey: "labsBoard" },
          { href: "/compare", labelKey: "compare" },
          { href: "/vs/openrouter", labelKey: "vsOr" },
        ],
      },
    ],
  },
];

export const TOP_LINKS = [{ href: "/enterprise", labelKey: "enterprise" }] as const;
