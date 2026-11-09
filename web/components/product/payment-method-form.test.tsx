// @vitest-environment jsdom
import {act,cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import {QueryClient,QueryClientProvider} from "@tanstack/react-query";
import {afterEach,expect,it,vi} from "vitest";
import {allowNavigation} from "@/lib/navigation-guard";
import {PaymentMethodForm} from "./payment-method-form";
let pending=false;
const run=vi.fn(async(_body:unknown)=>({id:"method-one",status:"saved"}));
vi.mock("./monetary",()=>({useMonetary:()=>({currency:"USD"})}));
vi.mock("./shared",()=>({useCommand:()=>({run,busy:false,unresolved:pending}),ErrorPanel:()=>null,Notice:({children}:{children:React.ReactNode})=><div>{children}</div>}));
afterEach(()=>{cleanup();vi.clearAllMocks();pending=false});
const catalog=[{kind:"bank_transfer",label:"银行转账",payout:true,drivers:[{value:"manual",label:"线下办理",fields:[{name:"instructions",label:"付款说明",secret:false}]}]},{kind:"kvoucher",label:"KVoucher",payout:false,drivers:[{value:"kvoucher",label:"KVoucher",fields:[{name:"kvoucher_api_key",label:"KVoucher API Key",secret:true}]}]}];
function mount(){return render(<QueryClientProvider client={new QueryClient()}><PaymentMethodForm operatorID="op" operation={{name:"open_method",label:"开通支付方式",path:"/api/v1/operators/op/payment-methods",method:"POST",fields:[]}} catalog={catalog} onBlockedChange={()=>{}}/></QueryClientProvider>)}
it("opens a built-in offline method with automatic payout materials and one fee rule",async()=>{
 mount();fireEvent.change(screen.getByLabelText('支付方式'),{target:{value:'bank_transfer'}});
 expect(screen.queryByLabelText('支付方式名称')).toBeNull();expect(screen.queryByLabelText('提现收款材料')).toBeNull();
 expect((screen.getByLabelText('用于余额和佣金提现') as HTMLInputElement).checked).toBe(true);
 fireEvent.change(screen.getByLabelText('付款说明'),{target:{value:'At the counter'}});
 fireEvent.change(screen.getByLabelText('手续费规则'),{target:{value:'percent'}});
 fireEvent.change(screen.getByLabelText('手续费率 %'),{target:{value:'2'}});
 fireEvent.click(screen.getByRole('button',{name:'保存支付方式'}));
 await waitFor(()=>expect(run).toHaveBeenCalledOnce());
 expect(run.mock.calls[0][0]).toMatchObject({method_kind:'bank_transfer',driver:'manual',payout_enabled:true,fee_mode:'percent',fee_percent:'2',credentials:{currency:'USD',instructions:'At the counter'}});
});
it("changes connection fields and removes unsupported withdrawal use when choosing KVoucher",()=>{
 mount();fireEvent.change(screen.getByLabelText('支付方式'),{target:{value:'bank_transfer'}});
 fireEvent.change(screen.getByLabelText('支付方式'),{target:{value:'kvoucher'}});
 expect(screen.queryByLabelText('付款说明')).toBeNull();expect(screen.queryByLabelText('用于余额和佣金提现')).toBeNull();
 expect(screen.getByLabelText('KVoucher API Key').getAttribute('type')).toBe('password');
});

it("keeps unresolved settings protected from navigation and form edits",()=>{
 pending=true;mount();
 act(()=>{expect(allowNavigation()).toBe(false)});
 expect(screen.getByLabelText('支付方式').closest('fieldset')?.disabled).toBe(true);
 expect(screen.getByRole('button',{name:'重试本次操作'})).toBeTruthy();
});
