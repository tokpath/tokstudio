import { safeReturnHref } from "./return-context";
export type Customer={id:string;email:string;display_name:string;status:string;channel_org_id:string;channel_code:string;brand_id:string;brand_name:string;source_code?:string;acquisition_role_id?:string;professional_role_id?:string;roles?:string[];created_at:string};
export type CustomerScope={id:string;code:string;brand_id:string;brand_name:string;can_create_professional?:boolean};
export function customerReturnHref(value:string|null|undefined,fallback:string,scope:string):string{
 const href=safeReturnHref(value,fallback);const params=new URL(href,"https://customer.invalid").searchParams;
 return params.has("viewer_scope") && params.get("viewer_scope")!==scope ? fallback:href;
}
