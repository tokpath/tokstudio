export type PortalId = "user" | "channel" | "admin";

export type ConsoleHero = "user-overview" | "channel-home" | "admin-overview";

export type NavItem = {
  href: string;
  label: string;
  eyebrow: string;
  title: string;
  emptyTitle: string;
  emptyDetail: string;
  columns?: string[];
  hero?: ConsoleHero;
};

export type PortalConfig = {
  id: PortalId;
  name: string;
  base: string;
  items: NavItem[];
};

export const PUBLIC_NAV = [
  { href: "/", label: "状态" },
  { href: "/docs", label: "文档" },
  { href: "/models", label: "模型" },
  { href: "/pricing", label: "定价" },
] as const;

export const PORTALS: Record<PortalId, PortalConfig> = {
  user: {
    id: "user",
    name: "用户控制台",
    base: "/console/user",
    items: [
      {
        href: "/console/user",
        label: "总览",
        eyebrow: "Overview",
        title: "总览",
        emptyTitle: "还没有路由回单",
        emptyDetail: "接通网关后，这里会显示最近一次 public_model → provider → attempt。客户只收一笔。",
        hero: "user-overview",
      },
      {
        href: "/console/user/wallet",
        label: "余额/充值",
        eyebrow: "Wallet",
        title: "余额/充值",
        emptyTitle: "还没有钱包流水",
        emptyDetail: "预授权、结算和退款会以等宽金额出现在账本里。现在没有假余额。",
        columns: ["时间", "方向", "金额", "状态"],
      },
      {
        href: "/console/user/plans",
        label: "套餐",
        eyebrow: "Plans",
        title: "套餐",
        emptyTitle: "还没有套餐权益",
        emptyDetail: "套餐额度与现金余额分账。未发布的渠道套餐不会出现在这里。",
        columns: ["套餐", "权益", "额度", "状态"],
      },
      {
        href: "/console/user/keys",
        label: "API Key",
        eyebrow: "Keys",
        title: "API Key",
        emptyTitle: "还没有 API Key",
        emptyDetail: "创建后只显示前缀。完整 Key 默认掩码，复制和轮换会写审计。",
        columns: ["名称", "前缀", "RPM", "状态"],
      },
      {
        href: "/console/user/usage",
        label: "用量/账单",
        eyebrow: "Ledger",
        title: "用量/账单",
        emptyTitle: "还没有用量事件",
        emptyDetail: "客户收费、上游成本和佣金不会写在同一列。",
        columns: ["请求", "模型", "客户收费", "状态"],
      },
      {
        href: "/console/user/media",
        label: "媒体任务",
        eyebrow: "Media",
        title: "媒体任务",
        emptyTitle: "还没有媒体任务",
        emptyDetail: "queued / in_progress / completed / failed 会用字标，不用发光进度环。",
        columns: ["任务", "模型", "进度", "状态"],
      },
      {
        href: "/console/user/docs",
        label: "文档",
        eyebrow: "Docs",
        title: "接入文档",
        emptyTitle: "还没有专属示例",
        emptyDetail: "接通后这里带入当前品牌 Base URL 和模型白名单，不内嵌真实 Key。",
      },
      {
        href: "/console/user/settings",
        label: "设置",
        eyebrow: "Settings",
        title: "个人设置",
        emptyTitle: "还不能改渠道归属",
        emptyDetail: "注册后的渠道绑定由服务端固化，用户不能在设置里自行切换。",
      },
    ],
  },
  channel: {
    id: "channel",
    name: "渠道控制台",
    base: "/console/channel",
    items: [
      {
        href: "/console/channel",
        label: "本渠道用户",
        eyebrow: "Channel",
        title: "本渠道用户",
        emptyTitle: "还没有本渠道用户",
        emptyDetail: "只能看到授权层级内的用户。其他渠道数据不会出现。",
        hero: "channel-home",
        columns: ["用户", "绑定", "用量", "状态"],
      },
      {
        href: "/console/channel/plans",
        label: "套餐",
        eyebrow: "Plans",
        title: "渠道套餐",
        emptyTitle: "还没有渠道套餐",
        emptyDetail: "渠道套餐受平台底价和额度上限约束，异常套餐会进审核。",
        columns: ["套餐", "底价约束", "额度上限", "状态"],
      },
      {
        href: "/console/channel/promo",
        label: "推广",
        eyebrow: "Promo",
        title: "推广链接",
        emptyTitle: "还没有推广链接",
        emptyDetail: "归因在注册成功时固化。这里不提供改归属。",
        columns: ["链接", "归因", "注册", "状态"],
      },
      {
        href: "/console/channel/quota",
        label: "额度",
        eyebrow: "Quota",
        title: "渠道额度",
        emptyTitle: "还没有渠道额度",
        emptyDetail: "额度按批发价累计消耗，不是充值金额。",
        columns: ["额度", "已用", "剩余", "状态"],
      },
      {
        href: "/console/channel/usage",
        label: "用量",
        eyebrow: "Usage",
        title: "渠道用量",
        emptyTitle: "还没有渠道用量",
        emptyDetail: "不展示 prompt/completion 原文。",
        columns: ["用户", "模型", "用量", "状态"],
      },
      {
        href: "/console/channel/commissions",
        label: "佣金/结算",
        eyebrow: "Hold",
        title: "佣金/结算",
        emptyTitle: "还没有冻结佣金",
        emptyDetail: "佣金按实际消耗冻结，退款会冲正。手机上先保证这张表。",
        columns: ["冻结", "可结算", "冲正", "状态"],
      },
    ],
  },
  admin: {
    id: "admin",
    name: "平台管理台",
    base: "/console/admin",
    items: [
      {
        href: "/console/admin",
        label: "总览",
        eyebrow: "Ops",
        title: "总览",
        emptyTitle: "还没有运营指标",
        emptyDetail: "待对账、毛利、佣金负债和 Provider 健康会分卡展示，不用大英雄区。",
        hero: "admin-overview",
      },
      {
        href: "/console/admin/providers",
        label: "提供商",
        eyebrow: "Providers",
        title: "提供商",
        emptyTitle: "还没有提供商",
        emptyDetail: "列表只显示脱敏指纹，不回显上游完整 Key。",
        columns: ["提供商", "指纹", "健康", "状态"],
      },
      {
        href: "/console/admin/models",
        label: "模型",
        eyebrow: "Models",
        title: "模型目录",
        emptyTitle: "还没有上架模型",
        emptyDetail: "公开模型与上游映射分开。同步结果先进入 draft。",
        columns: ["public_model_id", "上游映射", "能力", "状态"],
      },
      {
        href: "/console/admin/routes",
        label: "路由组",
        eyebrow: "Routes",
        title: "路由组",
        emptyTitle: "还没有路由组",
        emptyDetail: "fallback 不重复收取客户费用；每次 attempt 单独留痕。",
        columns: ["路由组", "fallback", "attempt", "状态"],
      },
      {
        href: "/console/admin/prices",
        label: "价格",
        eyebrow: "Price book",
        title: "价格版本",
        emptyTitle: "还没有价格版本",
        emptyDetail: "新价格只影响新请求。这里不改写历史账单。",
        columns: ["版本", "生效", "范围", "状态"],
      },
      {
        href: "/console/admin/users",
        label: "用户",
        eyebrow: "Users",
        title: "用户",
        emptyTitle: "还没有用户",
        emptyDetail: "普通用户是唯一终端用户类型。管理角色是附加授权。",
        columns: ["用户", "授权", "渠道", "状态"],
      },
      {
        href: "/console/admin/wallets",
        label: "余额",
        eyebrow: "Wallets",
        title: "余额/充值",
        emptyTitle: "还没有充值订单",
        emptyDetail: "退款、加款必须二次确认并写审计。",
        columns: ["订单", "金额", "方式", "状态"],
      },
      {
        href: "/console/admin/usage",
        label: "用量",
        eyebrow: "Usage",
        title: "用量/账单",
        emptyTitle: "还没有待对账",
        emptyDetail: "usage 缺失进入 HOLD / RECONCILE，不用灰字「未知」。",
        columns: ["事件", "客户收费", "对账", "状态"],
      },
      {
        href: "/console/admin/channels",
        label: "渠道",
        eyebrow: "Channels",
        title: "渠道/代理商",
        emptyTitle: "还没有渠道",
        emptyDetail: "A/B 用平台品牌；C 只换 theme_json 允许的章和 Logo。",
        columns: ["渠道", "类型", "品牌", "状态"],
      },
      {
        href: "/console/admin/plans",
        label: "套餐审核",
        eyebrow: "Review",
        title: "套餐审核",
        emptyTitle: "没有待审核套餐",
        emptyDetail: "异常套餐进审核。平台可下架，但不能改已产生的账单。",
        columns: ["套餐", "渠道", "原因", "状态"],
      },
      {
        href: "/console/admin/commissions",
        label: "佣金策略",
        eyebrow: "Policy",
        title: "佣金策略",
        emptyTitle: "还没有佣金策略",
        emptyDetail: "佣金调整必须确认对话框，并说明会冲正哪些流水。",
        columns: ["策略", "上限", "生效", "状态"],
      },
      {
        href: "/console/admin/metrics",
        label: "指标",
        eyebrow: "Metrics",
        title: "指标",
        emptyTitle: "还没有时间序列",
        emptyDetail: "ECharts 用 brand 与 hairline。没有数据时不画假曲线。",
      },
      {
        href: "/console/admin/audit",
        label: "审计",
        eyebrow: "Audit",
        title: "审计日志",
        emptyTitle: "还没有审计记录",
        emptyDetail: "审计不可删除。密钥明文不会出现在这里。",
        columns: ["时间", "动作", "对象", "结果"],
      },
      {
        href: "/console/admin/settings",
        label: "设置",
        eyebrow: "Settings",
        title: "系统设置",
        emptyTitle: "品牌配置尚未接通",
        emptyDetail: "OEM 只能改 theme_json 白名单。纸/碳和语义色不能改。",
      },
    ],
  },
};

export const PORTAL_IDS = Object.keys(PORTALS) as PortalId[];

export function isPortalId(value: string): value is PortalId {
  return value === "user" || value === "channel" || value === "admin";
}

export function findPortalItem(portal: PortalId, slug: string[] | undefined): NavItem | undefined {
  const href = slug?.length ? `${PORTALS[portal].base}/${slug.join("/")}` : PORTALS[portal].base;
  return PORTALS[portal].items.find((item) => item.href === href);
}

export function listConsoleParams(): { portal: string; slug?: string[] }[] {
  return PORTAL_IDS.flatMap((portal) =>
    PORTALS[portal].items.map((item) => {
      const rest = item.href.slice(PORTALS[portal].base.length).replace(/^\//, "");
      return rest ? { portal, slug: rest.split("/") } : { portal };
    }),
  );
}
