import type { LucideIcon } from "lucide-react";
import {
  Activity,
  AppWindow,
  AudioLines,
  BadgeCheck,
  BadgePercent,
  Banknote,
  BarChart3,
  Bell,
  Binary,
  BookOpen,
  Boxes,
  Building2,
  Circle,
  CircleDollarSign,
  Clapperboard,
  ClipboardList,
  Compass,
  CreditCard,
  FileCode,
  FileText,
  FlaskConical,
  Gauge,
  Gift,
  GitBranch,
  GitCompare,
  Handshake,
  House,
  Image,
  Inbox,
  KeyRound,
  Landmark,
  Layers,
  LayoutDashboard,
  Library,
  Lock,
  LogIn,
  MessageSquareText,
  Monitor,
  Network,
  Newspaper,
  Palette,
  Percent,
  Play,
  Puzzle,
  Receipt,
  Route,
  Scale,
  ScrollText,
  Server,
  Settings,
  ShieldCheck,
  Sparkles,
  Store,
  Terminal,
  Ticket,
  TrendingUp,
  Trophy,
  UserRound,
  Users,
  Video,
  Wallet,
  Webhook,
} from "lucide-react";
import type { MegaLink } from "@/lib/mega-nav";

/** 路由 → 图标。侧栏、命令面板、页脚、mega 下拉共用，避免每页各写一套。 */
export const HREF_ICONS: Record<string, LucideIcon> = {
  "/": House,
  "/models": Boxes,
  "/models?kind=text": MessageSquareText,
  "/image": Image,
  "/video": Video,
  "/model-finder": Compass,
  "/best-value": BadgePercent,
  "/leaderboards/models": Trophy,
  "/leaderboards/apps": AppWindow,
  "/leaderboards/labs": FlaskConical,
  "/compare": GitCompare,
  "/verify": BadgeCheck,
  "/quickstart": Sparkles,
  "/docs": BookOpen,
  "/docs/integrations": Puzzle,
  "/docs/develop": FileCode,
  "/docs/changelog": Newspaper,
  "/vibe-coding": Terminal,
  "/desktop": Monitor,
  "/enterprise": Building2,
  "/promo": Handshake,
  "/promo/august": Ticket,
  "/awesome-ofox": Sparkles,
  "/pricing": CircleDollarSign,
  "/trust": ShieldCheck,
  "/trust/subprocessors": Server,
  "/privacy": Lock,
  "/terms": FileText,
  "/blog": Newspaper,
  "/vs/openrouter": Scale,
  "/login": LogIn,
  "/enter": LayoutDashboard,
  "/app": LayoutDashboard,
  "/app/playground": Play,
  "/app/keys": KeyRound,
  "/app/catalog": Boxes,
  "/app/wallet": Wallet,
  "/app/plans": CreditCard,
  "/app/usage": BarChart3,
  "/app/reconciliation": Scale,
  "/app/activity": Receipt,
  "/app/media": Clapperboard,
  "/app/referral": Gift,
  "/app/docs": BookOpen,
  "/app/profile": UserRound,
  "/app/settings": Settings,
  "/app/settings/team": Building2,
  "/app/settings/members": Users,
  "/app/settings/billing": FileText,
  "/app/settings/quotas": Gauge,
  "/app/settings/apps": AppWindow,
  "/app/settings/webhooks": Webhook,
  "/channel": LayoutDashboard,
  "/channel/users": Users,
  "/channel/keys": KeyRound,
  "/channel/models": Boxes,
  "/channel/plans": CreditCard,
  "/channel/payments": Banknote,
  "/channel/payments/rules": Scale,
  "/channel/payments/orders": Receipt,
  "/channel/promos": Ticket,
  "/channel/brand": Palette,
  "/channel/ledger": Handshake,
  "/channel/rules": Scale,
  "/channel/attribution": GitBranch,
  "/channel/usage": BarChart3,
  "/channel/reconciliation": Scale,
  "/channel/settlements": Landmark,
  "/channel/commissions": Percent,
  "/partner": Network,
  "/partner/users": Users,
  "/partner/commissions": Percent,
  "/partner/settlements": Landmark,
  "/admin": LayoutDashboard,
  "/admin/providers": Server,
  "/admin/models": Boxes,
  "/admin/routes": Route,
  "/admin/keys": KeyRound,
  "/admin/plans": CreditCard,
  "/admin/prices": CircleDollarSign,
  "/admin/payments": Banknote,
  "/admin/billing": Receipt,
  "/admin/usage": BarChart3,
  "/admin/margin": TrendingUp,
  "/admin/reconciliation": Scale,
  "/admin/channels": Store,
  "/admin/brands": Palette,
  "/admin/promos": Ticket,
  "/admin/commission": Percent,
  "/admin/metrics": Activity,
  "/admin/media": Clapperboard,
  "/admin/users": Users,
  "/admin/alerts": Bell,
  "/admin/runbooks": ScrollText,
  "/admin/audit": ClipboardList,
  "/admin/settings": Settings,
};

export const KIND_ICONS: Record<string, LucideIcon> = {
  all: Layers,
  text: MessageSquareText,
  image: Image,
  video: Video,
  embedding: Binary,
  audio: AudioLines,
};

export const PUBLIC_PAGE_ICONS: Record<string, LucideIcon> = {
  models: Boxes,
  quickstart: Sparkles,
  docs: BookOpen,
  docsIntegrations: Puzzle,
  docsDevelop: FileCode,
  docsChangelog: Newspaper,
  enterprise: Building2,
  trust: ShieldCheck,
  trustSubprocessors: Server,
  bestValue: BadgePercent,
  modelFinder: Compass,
  vibeCoding: Terminal,
  video: Video,
  image: Image,
  leaderboardsModels: Trophy,
  leaderboardsApps: AppWindow,
  leaderboardsLabs: FlaskConical,
  vsOpenrouter: Scale,
  compare: GitCompare,
  promo: Handshake,
  promoAugust: Ticket,
  desktop: Monitor,
  verify: BadgeCheck,
  awesome: Sparkles,
  pricing: CircleDollarSign,
  blog: Newspaper,
  terms: FileText,
  privacy: Lock,
};

export const MEGA_MENU_ICONS: Record<string, LucideIcon> = {
  models: Boxes,
  docs: BookOpen,
  resources: Library,
};

const MEGA_LITERAL_ICONS: Record<string, LucideIcon> = {
  "Claude Code": Terminal,
  Codex: FileCode,
  "GPT-5.6 Sol": Sparkles,
  "Claude Fable 5": MessageSquareText,
  "Gemini 3.7 Flash": Sparkles,
  "Seedance 2.5": Video,
};

export const TOOL_ICONS: Record<string, LucideIcon> = {
  "Claude Code": Terminal,
  Codex: FileCode,
  "Gemini CLI": Sparkles,
  OpenCode: Boxes,
  Cline: Puzzle,
  OpenClaw: Monitor,
  "OpenAI SDK": FileCode,
  "Python · Node · cURL": FileCode,
};

export const WHY_ICONS = [BadgePercent, GitBranch, Scale, Trophy, ShieldCheck, Building2] as const;

export const ENTERPRISE_CAP_ICONS = [Gauge, KeyRound, GitBranch, Palette, Percent, ClipboardList] as const;

export const TRUST_SUMMARY_ICONS = [Inbox, Lock, KeyRound, LayoutDashboard] as const;

export const TRUST_RETENTION_ICONS = [MessageSquareText, Clapperboard, Receipt] as const;

export const PRICING_KIND_ICONS = [MessageSquareText, Image, Video] as const;

export const DASHBOARD_HERO_ICONS: Record<string, LucideIcon> = {
  heroPending: ClipboardList,
  heroProfit: CircleDollarSign,
  heroCommission: Percent,
  heroHealth: Activity,
};

export const CHANNEL_HERO_ICONS = [Gauge, BarChart3, Lock, Landmark] as const;

export const QUICKSTART_STEP_ICONS = [LogIn, FileCode, LayoutDashboard] as const;

export const DESKTOP_CAP_ICONS = [Monitor, KeyRound, Boxes, Receipt] as const;

export const CHANGELOG_ICONS = [Boxes, Library, Layers] as const;

export const PROMO_STEP_ICONS = [Ticket, Percent, Network] as const;

export const VERIFY_ICONS = [BadgeCheck, GitBranch] as const;

export const DOCS_ERROR_ICONS = [Wallet, Gauge, KeyRound] as const;

export function iconForHref(href: string): LucideIcon {
  if (HREF_ICONS[href]) return HREF_ICONS[href];
  const path = href.split("?")[0];
  if (HREF_ICONS[path]) return HREF_ICONS[path];
  if (path.startsWith("/models/")) return Boxes;
  if (path.startsWith("/app/settings")) return Settings;
  if (path.startsWith("/admin/")) return LayoutDashboard;
  if (path.startsWith("/channel/")) return Store;
  if (path.startsWith("/partner/")) return Network;
  if (path.startsWith("/app/") || path.startsWith("/console/")) return LayoutDashboard;
  if (path.startsWith("/docs/")) return BookOpen;
  if (path.startsWith("/blog/")) return Newspaper;
  return Circle;
}

export function iconForKind(kind: string): LucideIcon {
  return KIND_ICONS[kind] ?? Layers;
}

export function iconForPublicPage(id: string): LucideIcon {
  return PUBLIC_PAGE_ICONS[id] ?? Circle;
}

export function iconForMegaMenu(id: string): LucideIcon {
  return MEGA_MENU_ICONS[id] ?? Library;
}

export function iconForMegaLink(link: MegaLink): LucideIcon {
  if (link.literal && MEGA_LITERAL_ICONS[link.literal]) {
    return MEGA_LITERAL_ICONS[link.literal];
  }
  if (link.labelKey === "cline") return Puzzle;
  if (link.labelKey === "openaiSdk") return FileCode;
  if (link.labelKey === "vibe") return Terminal;
  if (link.labelKey === "textModels") return MessageSquareText;
  if (link.labelKey === "imageModels") return Image;
  if (link.labelKey === "videoModels") return Video;
  return iconForHref(link.href);
}

export function iconForTool(name: string): LucideIcon {
  return TOOL_ICONS[name] ?? Terminal;
}
