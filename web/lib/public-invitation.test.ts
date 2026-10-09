import { expect,it } from "vitest";
import { capturePublicInvitation, publicInvitationHref, readPublicInvitation } from "./public-invitation";
it("preserves a normalized invitation through public model/docs browsing and isolates brand and expiry",()=>{
  const values=new Map<string,string>();const storage={getItem:(key:string)=>values.get(key) ?? null,setItem:(key:string,value:string)=>{values.set(key,value);},removeItem:(key:string)=>{values.delete(key);}};
  capturePublicInvitation(storage,new URLSearchParams("promo=thu123"),"brand-a",1000);
  const model=publicInvitationHref("/models/echo?tab=agent&next=%2Fapp%2Fkeys",readPublicInvitation(storage,"brand-a",2000));
  expect(model).toBe("/models/echo?tab=agent&next=%2Fapp%2Fkeys&promotion_code=THU123");
  capturePublicInvitation(storage,new URL(model,"https://brand.example").searchParams,"brand-a",2000);
  expect(readPublicInvitation(storage,"brand-a",3000)).toBe("THU123");
  expect(readPublicInvitation(storage,"brand-b",3000)).toBe("");
  expect(readPublicInvitation(storage,"brand-a",2000+31*60*1000)).toBe("");
  capturePublicInvitation(storage,new URLSearchParams("promotion_code=bad/code"),"brand-a",3000);
  expect(readPublicInvitation(storage,"brand-a",3001)).toBe("");
  expect(publicInvitationHref("//foreign.test","THU123")).toBe("//foreign.test");
});
