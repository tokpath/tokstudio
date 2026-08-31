/** 公共站顶栏下拉：对齐 ofox「模型 / 文档 / 资源」信息架构。 */

export type MegaLink = { href: string; label: string; hint?: string };

export type MegaColumn = { title: string; links: MegaLink[] };

export type MegaMenu = {
  id: string;
  label: string;
  href?: string;
  columns: MegaColumn[];
};

export const MEGA_MENUS: MegaMenu[] = [
  {
    id: "models",
    label: "模型",
    href: "/models",
    columns: [
      {
        title: "浏览",
        links: [
          { href: "/models", label: "全部模型", hint: "价目表" },
          { href: "/models?kind=text", label: "文本 / 编程", hint: "Chat · Agent" },
          { href: "/image", label: "图像模型", hint: "生成与编辑" },
          { href: "/video", label: "视频模型", hint: "异步任务" },
        ],
      },
      {
        title: "工具",
        links: [
          { href: "/model-finder", label: "模型推荐器", hint: "按场景选" },
          { href: "/best-value", label: "性价比模型", hint: "低价排序" },
          { href: "/leaderboards/models", label: "使用排行榜", hint: "份额" },
          { href: "/compare", label: "模型对比", hint: "并排价目" },
          { href: "/verify", label: "模型验真", hint: "路由回单" },
        ],
      },
      {
        title: "热门",
        links: [
          { href: "/models/openai/gpt-5.6-sol", label: "GPT-5.6 Sol" },
          { href: "/models/anthropic/claude-fable-5", label: "Claude Fable 5" },
          { href: "/models/google/gemini-3.7-flash", label: "Gemini 3.7 Flash" },
          { href: "/models/bytedance/seedance-2.5", label: "Seedance 2.5" },
        ],
      },
    ],
  },
  {
    id: "docs",
    label: "文档",
    href: "/docs",
    columns: [
      {
        title: "开始",
        links: [
          { href: "/quickstart", label: "快速开始", hint: "3 分钟" },
          { href: "/docs", label: "开发者文档", hint: "curl / SDK" },
          { href: "/vibe-coding", label: "Vibe Coding", hint: "编程工具" },
        ],
      },
      {
        title: "集成",
        links: [
          { href: "/vibe-coding", label: "Claude Code", hint: "BASE_URL" },
          { href: "/vibe-coding", label: "Codex", hint: "config.toml" },
          { href: "/docs", label: "OpenAI SDK", hint: "兼容协议" },
          { href: "/docs/integrations", label: "Cline / OpenCode", hint: "自定义端点" },
        ],
      },
      {
        title: "参考",
        links: [
          { href: "/docs/develop", label: "错误与限流", hint: "402 / 429" },
          { href: "/docs/changelog", label: "更新日志", hint: "版本" },
        ],
      },
    ],
  },
  {
    id: "resources",
    label: "资源",
    columns: [
      {
        title: "产品",
        links: [
          { href: "/desktop", label: "Desktop" },
          { href: "/enterprise", label: "企业服务" },
          { href: "/promo", label: "合作推广" },
          { href: "/awesome-ofox", label: "生态应用" },
          { href: "/pricing", label: "定价" },
        ],
      },
      {
        title: "信任",
        links: [
          { href: "/trust", label: "信任中心" },
          { href: "/trust/subprocessors", label: "第三方服务商" },
          { href: "/privacy", label: "隐私政策" },
          { href: "/terms", label: "服务条款" },
        ],
      },
      {
        title: "内容",
        links: [
          { href: "/blog", label: "博客" },
          { href: "/leaderboards/apps", label: "应用排行" },
          { href: "/leaderboards/labs", label: "厂商排行" },
          { href: "/compare", label: "模型对比" },
          { href: "/vs/openrouter", label: "对比 OpenRouter" },
        ],
      },
    ],
  },
];

export const TOP_LINKS = [{ href: "/enterprise", label: "企业" }] as const;
