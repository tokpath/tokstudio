/** @vitest-environment jsdom */
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,fireEvent,render,screen,within} from "@testing-library/react";
import {withZh} from "@/lib/test-i18n";
import {OEMPurchaseReversal} from "./oem-purchase-reversal";
vi.mock("@/components/rbac/viewer-context",()=>({useViewer:()=>({userId:"finance"})}));
afterEach(()=>{cleanup();sessionStorage.clear();vi.unstubAllGlobals()});
const original={id:"purchase-one",oem_channel_org_id:"oem-one",quota_amount_minor:900000000,sale_amount_minor:1000000000,status:"completed"};
const json=(body:unknown,status=200)=>({ok:status<400,status,json:async()=>body});
it("preserves the original reversal through unknown response and reload, and finishes only after its exact record is confirmed",async()=>{
 const sent:string[]=[];let completed=false;const changed=vi.fn();vi.stubGlobal("fetch",vi.fn(async(_url,init)=>{if(init?.method==="POST"){sent.push(init.body);throw new Error("lost response")};const saved=JSON.parse(sessionStorage.getItem("oem-purchase-reverse:finance:oem-one:purchase-one")!);return json({item:{status:completed?"reversed":"completed",reversal_operation_id:saved.id,reversal_reason:saved.payload.reason}})}));
 const view=render(withZh(<OEMPurchaseReversal original={original} onChanged={changed}/>));fireEvent.click(screen.getByRole("button",{name:"撤销误录"}));fireEvent.change(screen.getByLabelText("撤销原因"),{target:{value:"wrong OEM"}});fireEvent.click(screen.getByRole("button",{name:"撤销误录"}));const dialog=await screen.findByRole("dialog");expect(dialog.textContent).toContain("$900.00 USD");expect(dialog.textContent).toContain("$1000.00 USD");fireEvent.click(within(dialog).getByRole("button",{name:"确认",exact:true}));await screen.findByRole("alert");view.unmount();render(withZh(<OEMPurchaseReversal original={original} onChanged={changed}/>));await screen.findByRole("button",{name:"重试原撤销"});fireEvent.click(screen.getByRole("button",{name:"核对原交易"}));expect(changed).not.toHaveBeenCalled();completed=true;fireEvent.click(screen.getByRole("button",{name:"核对原交易"}));await screen.findByText(/误录已撤销/);expect(changed).toHaveBeenCalledOnce();expect(sent).toHaveLength(1);expect(sessionStorage.getItem("oem-purchase-reverse:finance:oem-one:purchase-one")).toBeNull();
});
it("leaves the original sale in place when the locked transaction rejects insufficient available quota",async()=>{
 const changed=vi.fn();vi.stubGlobal("fetch",vi.fn(async()=>json({error:{code:"quota_not_recoverable",message:"原额度不足，未撤销"}},409)));render(withZh(<OEMPurchaseReversal original={original} onChanged={changed}/>));fireEvent.click(screen.getByRole("button",{name:"撤销误录"}));fireEvent.change(screen.getByLabelText("撤销原因"),{target:{value:"wrong amount"}});fireEvent.click(screen.getByRole("button",{name:"撤销误录"}));fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button",{name:"确认",exact:true}));await screen.findAllByText("原额度不足，未撤销");expect(changed).not.toHaveBeenCalled();expect(sessionStorage.getItem("oem-purchase-reverse:finance:oem-one:purchase-one")).toBeNull();
});
