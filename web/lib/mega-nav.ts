/** Public navigation advertises maintained, available tasks. */
export type MegaLink = { href: string; labelKey?: string; hintKey?: string; literal?: string };
export type MegaColumn = { titleKey: string; links: MegaLink[] };
export type MegaMenu = { id: string; labelKey: string; href?: string; columns: MegaColumn[] };
export const MEGA_MENUS: MegaMenu[] = [
  { id:"models", labelKey:"models", href:"/models", columns:[{titleKey:"browse",links:[
    {href:"/models",labelKey:"allModels"}, {href:"/models?kind=text",labelKey:"textModels"},
    {href:"/models?kind=image",labelKey:"imageModels"}, {href:"/models?kind=video",labelKey:"videoModels"},
    {href:"/pricing",labelKey:"pricing"},
  ]}] },
  { id:"docs", labelKey:"docs", href:"/docs", columns:[{titleKey:"start",links:[
    {href:"/docs",labelKey:"devDocs"}, {href:"/docs/integrations",labelKey:"openaiSdk"},
    {href:"/docs/develop",labelKey:"errors"},
  ]}] },
  { id:"resources",labelKey:"resources",columns:[{titleKey:"product",links:[
    {href:"/promo",labelKey:"promo"}, {href:"/enterprise",labelKey:"enterpriseSvc"},
    {href:"/trust",labelKey:"trustCenter"}, {href:"/privacy",labelKey:"privacy"}, {href:"/terms",labelKey:"terms"},
  ]}] },
];
export const TOP_LINKS: {href:string;labelKey:string}[] = [];
