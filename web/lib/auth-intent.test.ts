import { describe, expect, it } from "vitest";
import { authIntent, AUTH_INTENT_KEY, loginIntentHref, readAuthIntent, storeAuthIntent } from "./auth-intent";
import { oauthFailureHref } from "./google-oauth";
function storage() {
  const data = new Map<string,string>();
  return { getItem: (key: string) => data.get(key) ?? null, setItem: (key: string,value: string) => { data.set(key,value); }, removeItem: (key: string) => { data.delete(key); } };
}
describe("authentication task context", () => {
  it("keeps purchase/model intent and invitation through OAuth failure and retry", () => {
    const next = "/app/plans?plan=oem-plan&next=%2Fmodels%2Fmodel%3Ftab%3Dagent";
    const intent = authIntent(new URLSearchParams({ next, promo: " thu123 " }));
    const memory = storage();
    storeAuthIntent(memory,intent,1000);
    const restored = readAuthIntent(memory,false,2000);
    const failed = oauthFailureHref("Google 登录失败","access_denied",restored);
    expect(authIntent(new URL(failed,"https://oem.example").searchParams)).toEqual({next,promotionCode:"THU123"});
    expect(readAuthIntent(memory,true,2000)).toEqual(restored);
    expect(readAuthIntent(memory,false,2000)).toEqual({next:"",promotionCode:""});
  });
  it("clears prior context when starting a new login and expires stale invites", () => {
    const memory=storage();
    storeAuthIntent(memory,{next:"/app/wallet",promotionCode:"OLD"},1000);
    storeAuthIntent(memory,{next:"",promotionCode:""},2000);
    expect(readAuthIntent(memory,false,2001).promotionCode).toBe("");
    storeAuthIntent(memory,{next:"/app",promotionCode:"OLD"},1000);
    expect(readAuthIntent(memory,false,1000+16*60*1000).next).toBe("");
    expect(memory.getItem(AUTH_INTENT_KEY)).toBeNull();
  });
  it("rejects tampered storage and only creates site-local login destinations", () => {
    const memory=storage();
    memory.setItem(AUTH_INTENT_KEY,"not-json");
    expect(readAuthIntent(memory).next).toBe("");
    expect(loginIntentHref("//evil.test","bad/code")).toBe("/login?next=%2Fapp");
  });
});
