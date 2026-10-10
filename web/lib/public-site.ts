/** Maintained public tasks and their account continuations. */

export type PublicPageSpec = {
  href: string;
  ofox: string;
  label: string;
  purpose: string;
  auth?: boolean;
};

/** Actual task entry points; deprecated duplicate routes redirect to these. */
export const PUBLIC_PAGE_SPECS: PublicPageSpec[] = [
  {href:"/", ofox:"",label:"首页",purpose:"真实品牌能力与接入"},
  {href:"/models",ofox:"",label:"模型目录",purpose:"价格与使用说明"},
  {href:"/docs",ofox:"",label:"文档",purpose:"公开调用文档"},
  {href:"/docs/integrations",ofox:"",label:"接入说明",purpose:"支持的工具与协议"},
  {href:"/docs/develop",ofox:"",label:"开发指南",purpose:"错误与安全"},
  {href:"/pricing",ofox:"",label:"购买",purpose:"品牌真实套餐与充值"},
  {href:"/promo",ofox:"",label:"邀请",purpose:"同账户邀请与收益"},
  {href:"/enterprise",ofox:"",label:"OEM",purpose:"OEM 品牌经营"},
  {href:"/trust",ofox:"",label:"信任",purpose:"已实施的安全信息"},
  {href:"/trust/subprocessors",ofox:"",label:"子处理商",purpose:"第三方服务说明"},
  {href:"/terms",ofox:"",label:"服务条款",purpose:"法律条款"},
  {href:"/privacy",ofox:"",label:"隐私政策",purpose:"隐私说明"},
  {href:"/login",ofox:"",label:"登录/注册",purpose:"保留邀请与回程"},
  {href:"/app",ofox:"",label:"API 账户",purpose:"Key 优先",auth:true},
  {href:"/app/keys",ofox:"",label:"API Key",purpose:"使用范围与限制",auth:true},
  {href:"/app/wallet",ofox:"",label:"充值",purpose:"正式报价与结账",auth:true},
  {href:"/app/plans",ofox:"",label:"套餐",purpose:"权益与购买",auth:true},
  {href:"/app/referral",ofox:"",label:"邀请与收益",purpose:"邀请、进度、个人佣金",auth:true},
];

export const FOOTER_GROUPS = [
  {titleKey:"product",links:[{href:"/models",labelKey:"models"},{href:"/pricing",labelKey:"pricing"},{href:"/promo",labelKey:"promo"},{href:"/enterprise",labelKey:"enterprise"}]},
  {titleKey:"tools",links:[{href:"/docs",labelKey:"docs"},{href:"/docs/integrations",labelKey:"integrations"}]},
  {titleKey:"resources",links:[{href:"/trust",labelKey:"trust"},{href:"/trust/subprocessors",labelKey:"subprocessors"},{href:"/terms",labelKey:"terms"},{href:"/privacy",labelKey:"privacy"}]},
] as const;
