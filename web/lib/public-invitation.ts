import { promotionCode } from "./auth-intent";
import { safeNextPath } from "./login-next";
type InvitationStorage=Pick<Storage,"getItem"|"setItem"|"removeItem">;
export const PUBLIC_INVITATION_KEY="tokenhub_public_invitation_v1";
const AGE=30*60*1000;
export function capturePublicInvitation(storage:InvitationStorage|null,search:Pick<URLSearchParams,"get"|"has">,brandId:string,now=Date.now()){
  if(!storage || (!search.has("promotion_code") && !search.has("promo")))return;
  const code=promotionCode(search.get("promotion_code") || search.get("promo"));
  try{if(code)storage.setItem(PUBLIC_INVITATION_KEY,JSON.stringify({code,brandId,at:now}));else storage.removeItem(PUBLIC_INVITATION_KEY);}catch{/* private mode */}
}
export function readPublicInvitation(storage:InvitationStorage|null,brandId:string,now=Date.now()):string{
  if(!storage)return "";
  try{const raw=storage.getItem(PUBLIC_INVITATION_KEY);if(!raw)return "";const value=JSON.parse(raw);if(value.brandId!==brandId || typeof value.at!=="number" || value.at>now || now-value.at>AGE)return "";return promotionCode(value.code);}catch{return "";}
}
export function publicInvitationHref(href:string,rawCode?:string):string {
  const code=promotionCode(rawCode);
  if(!code || !safeNextPath(href))return href;
  const url=new URL(href,"https://invitation.invalid");url.searchParams.set("promotion_code",code);return url.pathname+url.search+url.hash;
}
