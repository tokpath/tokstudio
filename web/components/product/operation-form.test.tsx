// @vitest-environment jsdom
import {render,fireEvent,screen,waitFor,cleanup} from "@testing-library/react";
import {QueryClient,QueryClientProvider} from "@tanstack/react-query";
import {afterEach,expect,it,vi} from "vitest";
import {OperationForm} from "./operation-form";
import type {Operation,Entity,JSONValue} from "@/lib/contracts";
afterEach(()=>{cleanup();vi.unstubAllGlobals()});
function show(operation:Operation,row?:Entity){const cache=new QueryClient({defaultOptions:{queries:{retry:false}}});return render(<QueryClientProvider client={cache}><OperationForm operatorID={operation.path.startsWith("/api/v1/me/")?undefined:"operator-one"} operation={operation} row={row}/></QueryClientProvider>)}
it("selects named staff permissions and preserves existing extension grants",async()=>{
  const fetch=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify({id:"membership",status:"saved"}),{status:200}));vi.stubGlobal("fetch",fetch);
  show({name:"membership",label:"保存成员权限",method:"POST",path:"/api/v1/operators/operator-one/staff/{id}/membership",fields:[{name:"grants",label:"自定义权限",kind:"text",options:[{value:"catalog.manage",label:"维护平台模型与路由"},{value:"payments.manage",label:"管理收款"}]}]},{id:"membership",values:{grants:"catalog.manage,extension.inspect"}});
  expect((screen.getByRole("checkbox",{name:"维护平台模型与路由"})as HTMLInputElement).checked).toBe(true);
  expect((screen.getByRole("checkbox",{name:"现有扩展权限 · extension.inspect"})as HTMLInputElement).checked).toBe(true);
  fireEvent.click(screen.getByRole("checkbox",{name:"管理收款"}));fireEvent.click(screen.getByRole("checkbox",{name:"维护平台模型与路由"}));fireEvent.click(screen.getByRole("button",{name:"保存成员权限",exact:true}));
  await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(1));expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({grants:"extension.inspect,payments.manage"});
});
it("retains hidden concurrency version and edits actual arrays without showing internal CAS fields",async()=>{const fetch=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify({id:"membership",status:"saved"}),{status:200}));vi.stubGlobal("fetch",fetch);show({name:"membership",label:"保存成员",method:"POST",path:"/api/v1/operators/operator-one/staff/{id}/membership",include_version:true,fields:[{name:"version",label:"内部版本",kind:"hidden",required:true},{name:"grants",label:"明确权限",kind:"csv",required:true}]},{id:"staff-one",version:7,values:{grants:["reports.view","catalog.manage"]}});expect(screen.queryByText("内部版本")).toBeNull();expect((screen.getByLabelText("明确权限",{exact:false}) as HTMLInputElement).value).toBe("reports.view,catalog.manage");fireEvent.click(screen.getByRole("button",{name:"保存成员",exact:true}));await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(1));expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({version:7,grants:["reports.view","catalog.manage"]});await waitFor(()=>expect((screen.getByRole("button",{name:"本次操作已完成，关闭后查看最新记录"}) as HTMLButtonElement).disabled).toBe(true));});
it("only sends materials belonging to the selected payment driver",async()=>{const fetch=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify({id:"merchant-one",status:"pending_configuration"}),{status:200}));vi.stubGlobal("fetch",fetch);show({name:"configure",label:"保存材料",method:"POST",path:"/api/v1/operators/operator-one/payment-methods",fields:[{name:"driver",label:"通道",kind:"select",required:true,default_value:"stripe",options:[{value:"stripe",label:"Stripe"},{value:"wechat",label:"微信"}]},{name:"credentials.secret_key",label:"Stripe密钥",kind:"password",visible_when:{field:"driver",values:["stripe"]}},{name:"credentials.api_v3_key",label:"微信加密密钥",kind:"password",visible_when:{field:"driver",values:["wechat"]}}]});fireEvent.change(screen.getByLabelText("Stripe密钥"),{target:{value:"synthetic-stripe-secret"}});fireEvent.change(screen.getByLabelText("通道",{exact:false}),{target:{value:"wechat"}});expect(screen.queryByLabelText("Stripe密钥")).toBeNull();fireEvent.change(screen.getByLabelText("微信加密密钥"),{target:{value:"synthetic-wechat-key"}});fireEvent.click(screen.getByRole("button",{name:"保存材料",exact:true}));await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(1));expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({driver:"wechat",credentials:{api_v3_key:"synthetic-wechat-key"}});});

it("serializes a hidden destination consent as a boolean",async()=>{const fetch=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify({id:"proposal",status:"awaiting_finance_verification"}),{status:200}));vi.stubGlobal("fetch",fetch);show({name:"destination",label:"确认剩余款目的地",method:"POST",path:"/api/v1/me/refunds/legs/leg/responsible-destination",fields:[{name:"responsible_execution",label:"本人同意",kind:"hidden",default_value:true}]});fireEvent.click(screen.getByRole("button",{name:"确认剩余款目的地",exact:true}));await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(1));expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({responsible_execution:true});});

function mount(operation:Operation,props:Partial<React.ComponentProps<typeof OperationForm>>={}){const cache=new QueryClient({defaultOptions:{queries:{retry:false}}});return render(<QueryClientProvider client={cache}><OperationForm operatorID={operation.path.startsWith("/api/v1/me/")?undefined:"operator-one"} operation={operation} {...props}/></QueryClientProvider>)}
function priceOperation(configuration:JSONValue):Operation{return {name:"price",label:"发布价格版本",method:"POST",path:"/api/v1/operators/operator-one/pricing",fields:[{name:"configuration",label:"配置 JSON",kind:"json",required:true,default_value:configuration}]};}
function priceConfig():JSONValue{return {action:"price",evidence:"verified-notice",price:{id:"price-new",model_id:"model-one",scope:{kind:"retail",seller_id:"operator-one"},layer:"regular",effective_at:"2026-12-01T00:00:00Z",rule:{mode:"manual",components:[{key:"input_tokens",unit:"token",per:"1000000",rate:"1.000000000000000001",tiers:[]}]},published:true},provider_extension:{exact:"0.000000000000000000000001"}};}
function referenceFetch(){return vi.fn(async(input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify(String(input).includes("/references/")?{items:[{value:"model-one",label:"业务模型一"}]}:{id:"saved",status:"published"}),{status:200}));}

it("edits component and progressive tier prices as exact strings while retaining extension fields",async()=>{
  const fetch=referenceFetch();vi.stubGlobal("fetch",fetch);mount(priceOperation(priceConfig()));
  fireEvent.change(screen.getByLabelText(/^基础单价/),{target:{value:"0.000000000000000000000001"}});
  fireEvent.click(screen.getByRole("button",{name:"添加阶梯",exact:true}));
  fireEvent.change(screen.getByLabelText(/^起始数量/),{target:{value:"0"}});
  fireEvent.change(screen.getByLabelText(/^本阶梯单价/),{target:{value:"0.123456789012345678901234"}});
  fireEvent.click(screen.getByRole("button",{name:"发布价格版本",exact:true}));
  await waitFor(()=>expect(fetch.mock.calls.some(call=>call[1]?.method==="POST")).toBe(true));
  const call=fetch.mock.calls.find(call=>call[1]?.method==="POST")!;
  const payload=JSON.parse(String(call[1]?.body));
  expect(payload.configuration.price.rule.components[0]).toEqual({key:"input_tokens",unit:"token",per:"1000000",rate:"0.000000000000000000000001",tiers:[{from:"0",rate:"0.123456789012345678901234"}]});
  expect(payload.configuration.provider_extension).toEqual({exact:"0.000000000000000000000001"});
});

it("locates an invalid tier without submitting or rounding its amount",async()=>{
  const fetch=referenceFetch();vi.stubGlobal("fetch",fetch);mount(priceOperation(priceConfig()));
  fireEvent.click(screen.getByRole("button",{name:"添加阶梯",exact:true}));
  fireEvent.change(screen.getByLabelText(/^起始数量/),{target:{value:"0"}});
  const rate=screen.getByLabelText(/^本阶梯单价/);
  fireEvent.change(rate,{target:{value:"0.1234567890123456789012345"}});
  fireEvent.click(screen.getByRole("button",{name:"发布价格版本",exact:true}));
  expect(screen.getByRole("alert").textContent).toContain("最多 24 位小数");
  expect(document.activeElement).toBe(rate);
  expect(fetch.mock.calls.filter(call=>call[1]?.method==="POST")).toHaveLength(0);
});

it("locks a contextual order and retains the precise amount through submission",async()=>{
  const fetch=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify({id:"allocation",status:"allocated"}),{status:200}));vi.stubGlobal("fetch",fetch);
  mount({name:"allocate",label:"关联款项",method:"POST",path:"/api/v1/operators/operator-one/payments/receipt/allocate",fields:[{name:"order_id",label:"业务订单",kind:"text",required:true},{name:"amount_usd",label:"分配金额 USD",kind:"decimal",required:true}]},{initialValues:{order_id:"exact-order",amount_usd:"1.000000000000000001"},lockedFields:["order_id"]});
  expect((screen.getByLabelText("业务订单",{exact:false})as HTMLInputElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole("button",{name:"关联款项",exact:true}));
  await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(1));
  expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({order_id:"exact-order",amount_usd:"1.000000000000000001"});
});

it("prevents dismissal of an unknown result and retries the exact original command",async()=>{
  const close=vi.fn(),blocked=vi.fn();
  const fetch=vi.fn().mockRejectedValueOnce(new TypeError("connection lost")).mockImplementation(async()=>new Response(JSON.stringify({id:"withdrawal",status:"requested"}),{status:200}));vi.stubGlobal("fetch",fetch);
  mount({name:"withdraw",label:"申请提现",method:"POST",path:"/api/v1/operators/operator-one/withdrawals",fields:[{name:"amount_usd",label:"申请金额 USD",kind:"decimal",required:true}]},{onClose:close,onBlockedChange:blocked});
  const amount=screen.getByLabelText("申请金额 USD",{exact:false});
  fireEvent.change(amount,{target:{value:"0.000000000000000001"}});
  fireEvent.click(screen.getByRole("button",{name:"申请提现",exact:true}));
  await screen.findByText("本次办理结果待核实");
  expect((amount as HTMLInputElement).disabled).toBe(true);
  expect((screen.getByRole("button",{name:"关闭",exact:true})as HTMLButtonElement).disabled).toBe(true);
  expect(blocked).toHaveBeenLastCalledWith(true);
  fireEvent.click(screen.getByRole("button",{name:"关闭",exact:true}));expect(close).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button",{name:"重试本次操作",exact:true}));
  await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(2));
  expect(fetch.mock.calls[1][1].body).toBe(fetch.mock.calls[0][1].body);
  expect(fetch.mock.calls[1][1].headers.get("Idempotency-Key")).toBe(fetch.mock.calls[0][1].headers.get("Idempotency-Key"));
  await screen.findByRole("button",{name:"返回记录",exact:true});expect(blocked).toHaveBeenLastCalledWith(false);
});

it("selects route credentials without replacing the route identity or concurrency version",async()=>{
  const fetch=vi.fn(async(input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify(String(input).includes("/references/credentials")?{items:[{value:"new-credential",label:"已保存供应商凭据",defaults:{id:"credential-record",provider_id:"new-provider",expected_version:99}}]}:String(input).includes("/references/models")?{items:[{value:"model-one",label:"业务模型一"}]}:{id:"route-original",status:"saved"}),{status:200}));vi.stubGlobal("fetch",fetch);
  const operation:Operation={name:"save",label:"保存路由",method:"POST",path:"/api/v1/operators/operator-one/routes",fields:[{name:"configuration",label:"配置 JSON",kind:"json",required:true}]};
  mount(operation,{row:{id:"route-original",version:7,values:{id:"route-original",public_model_id:"model-one",identity:"verified-revision",provider_id:"old-provider",provider:"openai",model:"upstream-model",credential_id:"old-credential",enabled:false,healthy:false,priority:2,capabilities:[]}}});
  const credential=screen.getByRole("combobox",{name:/已保存的供应商凭据引用/});
  await waitFor(()=>expect((credential as HTMLSelectElement).disabled).toBe(false));
  fireEvent.change(credential,{target:{value:"new-credential"}});
  fireEvent.click(screen.getByRole("button",{name:"保存路由",exact:true}));
  await waitFor(()=>expect(fetch.mock.calls.some(call=>call[1]?.method==="POST")).toBe(true));
  const payload=JSON.parse(String(fetch.mock.calls.find(call=>call[1]?.method==="POST")?.[1]?.body));
  expect(payload.configuration.expected_version).toBe(7);
  expect(payload.configuration.upstream).toMatchObject({id:"route-original",credential_id:"new-credential",provider_id:"new-provider",identity:"verified-revision",priority:2});
});

it("keeps reference defaults, the hidden revision and exact amounts together",async()=>{
  const fetch=vi.fn(async(input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify(String(input).includes("/references/")?{items:[{value:"plan-one",label:"专业套餐",defaults:{expected_version:12,price_usd:"1.123456789012345678"}}]}:{id:"plan-one",status:"published"}),{status:200}));vi.stubGlobal("fetch",fetch);
  mount({name:"publish",label:"保存套餐",method:"POST",path:"/api/v1/operators/operator-one/plans",fields:[{name:"package_id",label:"选择套餐",kind:"text",lookup_url:"/api/v1/operators/operator-one/references/plans"},{name:"price_usd",label:"售价 USD",kind:"decimal",required:true},{name:"expected_version",label:"版本",kind:"hidden",default_value:0}]});
  const select=screen.getByRole("combobox",{name:"选择套餐"});await waitFor(()=>expect((select as HTMLSelectElement).disabled).toBe(false));
  fireEvent.change(select,{target:{value:"plan-one"}});
  expect((screen.getByLabelText("售价 USD",{exact:false}) as HTMLInputElement).value).toBe("1.123456789012345678");
  expect(screen.queryByLabelText("版本")).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"保存套餐",exact:true}));
  await waitFor(()=>expect(fetch.mock.calls.some(call=>call[1]?.method==="POST")).toBe(true));
  expect(JSON.parse(String(fetch.mock.calls.find(call=>call[1]?.method==="POST")?.[1]?.body))).toEqual({package_id:"plan-one",price_usd:"1.123456789012345678",expected_version:12});
});

it("uploads only the file visible for the selected business path",async()=>{
  const fetch=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify({id:"evidence",status:"saved"}),{status:200}));vi.stubGlobal("fetch",fetch);
  mount({name:"proof",label:"保存凭证",method:"POST",path:"/api/v1/operators/operator-one/evidence",fields:[{name:"kind",label:"凭证种类",kind:"select",default_value:"bank",options:[{value:"bank",label:"银行"},{value:"chain",label:"链上"}]},{name:"bank_file",label:"银行凭证文件",kind:"file",visible_when:{field:"kind",values:["bank"]}},{name:"chain_file",label:"链上凭证文件",kind:"file",visible_when:{field:"kind",values:["chain"]}}]});
  fireEvent.change(screen.getByLabelText("银行凭证文件"),{target:{files:[new File(["bank"],"bank.txt")]}});
  fireEvent.change(screen.getByLabelText("凭证种类"),{target:{value:"chain"}});
  fireEvent.change(screen.getByLabelText("链上凭证文件"),{target:{files:[new File(["chain"],"chain.txt")]}});
  fireEvent.click(screen.getByRole("button",{name:"保存凭证",exact:true}));await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(1));
  const body=fetch.mock.calls[0]?.[1]?.body as FormData;
  expect(body.get("bank_file")).toBeNull();expect((body.get("chain_file") as File).name).toBe("chain.txt");expect(body.get("kind")).toBe("chain");
});

it("selects a verified public model for a route and applies only its matching identity",async()=>{
  const fetch=vi.fn(async(input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify(String(input).includes("/references/models")?{items:[{value:"model-two",label:"业务模型二 · 已核实版本",defaults:{public_model_id:"model-two",identity:"revision-two",id:"unexpected-id",expected_version:99,provider_id:"unexpected-provider"}}]}:String(input).includes("/references/")?{items:[]}:{id:"route-original",status:"saved"}),{status:200}));vi.stubGlobal("fetch",fetch);
  mount({name:"save",label:"保存路由",method:"POST",path:"/api/v1/operators/operator-one/routes",fields:[{name:"configuration",label:"配置 JSON",kind:"json",required:true}]},{row:{id:"route-original",version:7,values:{id:"route-original",public_model_id:"model-one",identity:"revision-one",provider_id:"provider-original",provider:"openai",model:"upstream-model",credential_id:"credential-original",enabled:false,healthy:false,priority:-2,capabilities:[],provider_extension:{revision:"untouched"}}}});
  const model=screen.getByRole("combobox",{name:/对应公开模型/});await waitFor(()=>expect((model as HTMLSelectElement).disabled).toBe(false));
  fireEvent.change(model,{target:{value:"model-two"}});
  expect((screen.getByLabelText(/^与公开模型一致的身份与版本/)as HTMLInputElement).value).toBe("revision-two");
  fireEvent.click(screen.getByRole("button",{name:"保存路由",exact:true}));
  await waitFor(()=>expect(fetch.mock.calls.some(call=>call[1]?.method==="POST")).toBe(true));
  const payload=JSON.parse(String(fetch.mock.calls.find(call=>call[1]?.method==="POST")?.[1]?.body));
  expect(payload.configuration.expected_version).toBe(7);
  expect(payload.configuration.upstream).toMatchObject({id:"route-original",public_model_id:"model-two",identity:"revision-two",provider_id:"provider-original",priority:-2,provider_extension:{revision:"untouched"}});
});

it("offers model references for listing without copying route defaults into the authorization",async()=>{
  const fetch=vi.fn(async(input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify(String(input).includes("/references/models")?{items:[{value:"model-two",label:"业务模型二",defaults:{public_model_id:"model-two",identity:"revision-two",listing_version:0}}]}:{id:"model-two",status:"configured"}),{status:200}));vi.stubGlobal("fetch",fetch);
  mount({name:"listing",label:"授权或上架模型",method:"POST",path:"/api/v1/operators/operator-one/pricing",fields:[{name:"configuration",label:"配置 JSON",kind:"json",required:true}]});
  const model=screen.getByRole("combobox",{name:"公开模型"});await waitFor(()=>expect((model as HTMLSelectElement).disabled).toBe(false));
  fireEvent.change(model,{target:{value:"model-two"}});
  fireEvent.click(screen.getByRole("button",{name:"授权或上架模型",exact:true}));
  await waitFor(()=>expect(fetch.mock.calls.some(call=>call[1]?.method==="POST")).toBe(true));
  expect(JSON.parse(String(fetch.mock.calls.find(call=>call[1]?.method==="POST")?.[1]?.body)).configuration).toEqual({action:"listing",model_id:"model-two",enabled:true,expected_version:0});
});

it("reads authorization revisions for the selected buyer and clears stale model revisions when the buyer changes",async()=>{
  const fetch=vi.fn(async(input:RequestInfo|URL,_init?:RequestInit)=>{
    const url=String(input);let result:unknown={id:"scheduled",status:"pending"};
    if(url.includes("/references/buyers"))result={items:[{value:"buyer-one",label:"直属下级一"},{value:"buyer-two",label:"直属下级二"}]};
    else if(url.includes("/references/models"))result={items:[{value:"model-one",label:"业务模型一",defaults:{public_model_id:"model-one",identity:"verified-revision",listing_version:4,authorization_version:url.includes("buyer-two")?9:3}}]};
    return new Response(JSON.stringify(result),{status:200});
  });vi.stubGlobal("fetch",fetch);
  mount({name:"listing",label:"授权或上架模型",method:"POST",path:"/api/v1/operators/operator-one/pricing",fields:[{name:"configuration",label:"配置 JSON",kind:"json",required:true,default_value:{action:"authorization",effective_at:"2026-12-01T00:00:00Z",evidence:"verified-notice"}}]});
  const model=screen.getByRole("combobox",{name:"公开模型"});expect((model as HTMLSelectElement).disabled).toBe(true);
  expect(fetch.mock.calls.some(call=>String(call[0]).includes("/references/models"))).toBe(false);
  const buyer=screen.getByRole("combobox",{name:"被授权的直属下级"});await waitFor(()=>expect((buyer as HTMLSelectElement).disabled).toBe(false));
  fireEvent.change(buyer,{target:{value:"buyer-one"}});await waitFor(()=>expect((model as HTMLSelectElement).disabled).toBe(false));
  fireEvent.change(model,{target:{value:"model-one"}});
  const version=screen.getByLabelText(/^当前上架 \/ 授权版本/);expect((version as HTMLInputElement).disabled).toBe(true);expect((version as HTMLInputElement).value).toBe("3");
  fireEvent.change(buyer,{target:{value:"buyer-two"}});expect((model as HTMLSelectElement).value).toBe("");expect((version as HTMLInputElement).value).toBe("");
  await waitFor(()=>expect((model as HTMLSelectElement).disabled).toBe(false));fireEvent.change(model,{target:{value:"model-one"}});expect((version as HTMLInputElement).value).toBe("9");
  fireEvent.click(screen.getByRole("button",{name:"授权或上架模型",exact:true}));await waitFor(()=>expect(fetch.mock.calls.some(call=>call[1]?.method==="POST")).toBe(true));
  expect(JSON.parse(String(fetch.mock.calls.find(call=>call[1]?.method==="POST")?.[1]?.body)).configuration).toMatchObject({action:"authorization",buyer_id:"buyer-two",model_id:"model-one",expected_version:9});
  expect(fetch.mock.calls.some(call=>String(call[0]).includes("buyer_id=buyer-one"))).toBe(true);expect(fetch.mock.calls.some(call=>String(call[0]).includes("buyer_id=buyer-two"))).toBe(true);
});

it("blocks listing when its reference response does not supply a verified revision",async()=>{
  const fetch=referenceFetch();vi.stubGlobal("fetch",fetch);
  mount({name:"listing",label:"授权或上架模型",method:"POST",path:"/api/v1/operators/operator-one/pricing",fields:[{name:"configuration",label:"配置 JSON",kind:"json",required:true}]});
  const model=screen.getByRole("combobox",{name:"公开模型"});await waitFor(()=>expect((model as HTMLSelectElement).disabled).toBe(false));fireEvent.change(model,{target:{value:"model-one"}});
  fireEvent.click(screen.getByRole("button",{name:"授权或上架模型",exact:true}));expect(fetch.mock.calls.filter(call=>call[1]?.method==="POST")).toHaveLength(0);
  expect(screen.getByRole("alert").textContent).toContain("自动带入");
});

it("keeps the default follow wildcard available beside model references",async()=>{
  const fetch=referenceFetch();vi.stubGlobal("fetch",fetch);mount(priceOperation(priceConfig()));
  const model=screen.getByRole("combobox",{name:/公开模型（自动跟随/});await waitFor(()=>expect((model as HTMLSelectElement).disabled).toBe(false));
  expect(screen.getByRole("option",{name:"全部模型的默认跟随规则（*）"})).toBeTruthy();
  fireEvent.change(model,{target:{value:"*"}});
  const configuration=JSON.parse((screen.getByLabelText("完整技术配置 JSON")as HTMLTextAreaElement).value);
  expect(configuration.price.model_id).toBe("*");
  expect(configuration.provider_extension.exact).toBe("0.000000000000000000000001");
});

it("never prefills secrets from records and omits untouched optional secrets instead of clearing them",async()=>{
  const fetch=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify({id:"merchant",status:"saved"}),{status:200}));vi.stubGlobal("fetch",fetch);
  mount({name:"configure",label:"保存商户",method:"POST",path:"/api/v1/operators/operator-one/payment-methods",fields:[{name:"credentials.currency",label:"币种",kind:"text"},{name:"credentials.secret_key",label:"新的密钥",kind:"password"},{name:"credentials.app_private_key",label:"新的私钥",kind:"textarea"}]},{row:{id:"merchant",values:{credentials:{currency:"USD",secret_key:"must-not-echo",app_private_key:"must-not-echo-private"}}}});
  expect((screen.getByLabelText("新的密钥")as HTMLInputElement).value).toBe("");
  expect((screen.getByLabelText("新的私钥")as HTMLTextAreaElement).value).toBe("");
  expect(screen.queryByText("must-not-echo")).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"保存商户",exact:true}));await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(1));
  expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({credentials:{currency:"USD"}});
});

it("requires a new review after changed input and permits correction after a known rejection",async()=>{
  const fetch=vi.fn().mockImplementationOnce(async()=>new Response(JSON.stringify({error:{code:"invalid_amount",message:"核实金额后重试",request_id:"known-rejection",retryable:false}}),{status:400})).mockImplementation(async()=>new Response(JSON.stringify({id:"withdrawal",status:"requested"}),{status:200}));vi.stubGlobal("fetch",fetch);
  mount({name:"withdraw",label:"申请提现",method:"POST",path:"/api/v1/operators/operator-one/withdrawals",confirmation:"确认已核实金额和收款资料",fields:[{name:"amount_usd",label:"申请金额 USD",kind:"decimal",required:true}]},{onClose:vi.fn()});
  const amount=screen.getByLabelText("申请金额 USD",{exact:false}),confirmation=screen.getByRole("checkbox",{name:"确认已核实金额和收款资料"});
  fireEvent.change(amount,{target:{value:"1"}});fireEvent.click(confirmation);fireEvent.click(screen.getByRole("button",{name:"申请提现",exact:true}));
  await screen.findByText("核实金额后重试");
  expect((amount as HTMLInputElement).disabled).toBe(false);expect((screen.getByRole("button",{name:"关闭",exact:true})as HTMLButtonElement).disabled).toBe(false);
  fireEvent.change(amount,{target:{value:"1.000000000000000001"}});expect((confirmation as HTMLInputElement).checked).toBe(false);
  fireEvent.click(screen.getByRole("button",{name:"申请提现",exact:true}));expect(fetch).toHaveBeenCalledTimes(1);
  fireEvent.click(confirmation);fireEvent.click(screen.getByRole("button",{name:"申请提现",exact:true}));await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(2));
  expect(JSON.parse(fetch.mock.calls[1][1].body)).toEqual({amount_usd:"1.000000000000000001"});
  expect(fetch.mock.calls[1][1].headers.get("Idempotency-Key")).not.toBe(fetch.mock.calls[0][1].headers.get("Idempotency-Key"));
});

it.each(["{malformed",JSON.stringify({unexpected:"response"})])("locks an unverified response and preserves the original operation",async response=>{
  const fetch=vi.fn(async()=>new Response(response,{status:200}));vi.stubGlobal("fetch",fetch);
  mount({name:"allocate",label:"关联款项",method:"POST",path:"/api/v1/operators/operator-one/payments/receipt/allocate",fields:[{name:"amount_usd",label:"分配金额 USD",kind:"decimal",default_value:"0.000000000000000001"}]},{onClose:vi.fn()});
  fireEvent.click(screen.getByRole("button",{name:"关联款项",exact:true}));await screen.findByText("本次办理结果待核实");
  expect((screen.getByRole("button",{name:"关闭",exact:true})as HTMLButtonElement).disabled).toBe(true);
  expect((screen.getByLabelText("分配金额 USD")as HTMLInputElement).disabled).toBe(true);
  expect(screen.queryByText("关联款项已受理")).toBeNull();
});

it("validates percentage fractions without converting or rounding their decimal strings",async()=>{
  const fetch=vi.fn(async()=>new Response(JSON.stringify({id:"policy",status:"saved"}),{status:200}));vi.stubGlobal("fetch",fetch);
  mount({name:"policy",label:"保存比例",method:"POST",path:"/api/v1/operators/operator-one/commissions",fields:[{name:"direct_rate",label:"直接比例（0到1）",kind:"decimal",required:true}]});
  const rate=screen.getByRole("textbox",{name:/^直接比例（0到1）/});fireEvent.change(rate,{target:{value:"5"}});fireEvent.click(screen.getByRole("button",{name:"保存比例",exact:true}));expect(fetch).not.toHaveBeenCalled();
  expect(screen.getByRole("alert").textContent).toContain("5% 填写 0.05");
  fireEvent.change(rate,{target:{value:"0.050000000000000001"}});fireEvent.click(screen.getByRole("button",{name:"保存比例",exact:true}));await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(1));
  expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({direct_rate:"0.050000000000000001"});
});

it("types nested concurrency versions while preserving opaque execution keys and explicit nullable policy values",async()=>{
  const fetch=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify({id:"policy",status:"saved"}),{status:200}));vi.stubGlobal("fetch",fetch);
  mount({name:"policy",label:"保存嵌套政策",method:"POST",path:"/api/v1/operators/operator-one/refunds",fields:[{name:"configuration.expected_version",label:"真实政策版本",kind:"hidden",default_value:12},{name:"configuration.expected_execution_key",label:"原执行键",kind:"hidden",default_value:"000123-opaque"},{name:"configuration.second_review_above",label:"独立复核阈值",kind:"decimal"},{name:"configuration.fee_rate",label:"手续费比例",kind:"decimal",required:true,default_value:"2"}]});
  fireEvent.click(screen.getByRole("button",{name:"保存嵌套政策",exact:true}));expect(fetch).not.toHaveBeenCalled();expect(screen.getByRole("alert").textContent).toContain("0 到 1");
  fireEvent.change(screen.getByLabelText("手续费比例",{exact:false}),{target:{value:"0.000000000000000001"}});fireEvent.click(screen.getByRole("button",{name:"保存嵌套政策",exact:true}));await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(1));
  expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({configuration:{expected_version:12,expected_execution_key:"000123-opaque",second_review_above:null,fee_rate:"0.000000000000000001"}});
});

it("accepts the ledger's exact eighteenth decimal place and rejects a nineteenth before submitting",async()=>{
  const fetch=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify({id:"receipt",status:"saved"}),{status:200}));vi.stubGlobal("fetch",fetch);
  mount({name:"receipt",label:"登记精确金额",method:"POST",path:"/api/v1/operators/operator-one/payments",fields:[{name:"amount_usd",label:"金额 USD",kind:"decimal",required:true}]});
  const amount=screen.getByLabelText("金额 USD",{exact:false});fireEvent.change(amount,{target:{value:"0.0000000000000000001"}});fireEvent.click(screen.getByRole("button",{name:"登记精确金额",exact:true}));
  expect(screen.getByRole("alert").textContent).toContain("最多 18 位小数");expect(fetch).not.toHaveBeenCalled();
  fireEvent.change(amount,{target:{value:"0.000000000000000001"}});fireEvent.click(screen.getByRole("button",{name:"登记精确金额",exact:true}));await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(1));
  expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({amount_usd:"0.000000000000000001"});
});

it("permits signed ledger thresholds for actual risk metrics",async()=>{
  const fetch=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify({id:"rule",status:"saved"}),{status:200}));vi.stubGlobal("fetch",fetch);
  mount({name:"risk_rule",label:"保存风险阈值",method:"POST",path:"/api/v1/operators/operator-one/risk",fields:[{name:"warning",label:"预警阈值",kind:"decimal",required:true,default_value:"-1.000000000000000001"},{name:"critical",label:"严重阈值",kind:"decimal",required:true,default_value:"-2.000000000000000001"}]});
  fireEvent.click(screen.getByRole("button",{name:"保存风险阈值",exact:true}));await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(1));
  expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({warning:"-1.000000000000000001",critical:"-2.000000000000000001"});
});

it("preserves a procurement authorization identity and CAS while linked quotes, buyers and merchants change",async()=>{
  const fetch=vi.fn(async(input:RequestInfo|URL,_init?:RequestInit)=>{
    const path=String(input);const items=path.includes("purchase-quotes")?[{value:"quote-new",label:"当前可用报价",defaults:{quote_id:"quote-new",supplier_id:"supplier-new",id:"quote-record",expected_version:90,version:91,enabled:true}}]:path.includes("merchants")?[{value:"merchant-new",label:"已保存商户",defaults:{id:"merchant-record",expected_version:99,enabled:true}}]:[{value:"buyer-new",label:"直属下级"}];
    return new Response(JSON.stringify(path.includes("/references/")?{items}:{id:"authorization-original",status:"saved"}),{status:200});
  });vi.stubGlobal("fetch",fetch);
  mount({name:"authorization",label:"保存采购授权",method:"POST",path:"/api/v1/operators/operator-one/procurement",fields:[{name:"id",label:"授权编号",kind:"hidden",required:true},{name:"quote_id",label:"采购报价",kind:"text",required:true,lookup_url:"/api/v1/operators/operator-one/references/purchase-quotes"},{name:"supplier_id",label:"直接上级",kind:"text",required:true},{name:"merchant_id",label:"付款商户",kind:"text",lookup_url:"/api/v1/operators/operator-one/references/merchants"},{name:"buyer_id",label:"直属下级",kind:"text",lookup_url:"/api/v1/operators/operator-one/references/buyers"},{name:"enabled",label:"启用授权",kind:"boolean"},{name:"expected_version",label:"授权版本",kind:"hidden",default_value:0}]},{row:{id:"authorization-original",version:7,values:{id:"authorization-original",quote_id:"quote-old",supplier_id:"supplier-old",merchant_id:"merchant-old",enabled:false}}});
  for(const [label,value]of [["采购报价","quote-new"],["付款商户","merchant-new"],["直属下级","buyer-new"]]){const select=screen.getByRole("combobox",{name:label});await waitFor(()=>expect((select as HTMLSelectElement).disabled).toBe(false));fireEvent.change(select,{target:{value}});}
  expect((screen.getByLabelText("直接上级",{exact:false})as HTMLInputElement).value).toBe("supplier-new");
  fireEvent.click(screen.getByRole("button",{name:"保存采购授权",exact:true}));await waitFor(()=>expect(fetch.mock.calls.some(call=>call[1]?.method==="POST")).toBe(true));
  expect(JSON.parse(String(fetch.mock.calls.find(call=>call[1]?.method==="POST")?.[1]?.body))).toEqual({id:"authorization-original",quote_id:"quote-new",supplier_id:"supplier-new",merchant_id:"merchant-new",buyer_id:"buyer-new",enabled:false,expected_version:7});
});

it.each(["/api/v1/operators/operator-one/marketing", "/api/v1/me/activities"])("uses campaign CAS only for the campaign selector and keeps linked plan gifts out of %s",async path=>{
  const fetch=vi.fn(async(input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify(String(input).includes("/references/campaigns")?{items:[{value:"campaign-selected",label:"本经营活动",defaults:{campaign_id:"campaign-selected",gift_atg:"3.000000000000000001",expected_version:11}}]}:String(input).includes("/references/plans")?{items:[{value:"plan-new",label:"适用套餐",defaults:{package_id:"plan-new",gift_atg:"500",expected_version:99}}]}:{id:"campaign-selected",status:"published"}),{status:200}));vi.stubGlobal("fetch",fetch);
  mount({name:"publish",label:"保存活动授权",method:"POST",path,fields:[{name:"campaign_id",label:"经营活动",kind:"text",lookup_url:"/api/v1/operators/operator-one/references/campaigns"},{name:"package_id",label:"适用套餐",kind:"text",required:true,lookup_url:"/api/v1/operators/operator-one/references/plans"},{name:"gift_atg",label:"活动赠送",kind:"decimal",required:true},{name:"expected_version",label:"活动版本",kind:"hidden",default_value:0}]},{row:{id:"campaign-old",version:7,values:{campaign_id:"campaign-old",package_id:"plan-old",gift_atg:"2"}}});
  for(const [label,value]of [["经营活动","campaign-selected"],["适用套餐","plan-new"]]){const select=screen.getByRole("combobox",{name:label});await waitFor(()=>expect((select as HTMLSelectElement).disabled).toBe(false));fireEvent.change(select,{target:{value}});}
  fireEvent.click(screen.getByRole("button",{name:"保存活动授权",exact:true}));await waitFor(()=>expect(fetch.mock.calls.some(call=>call[1]?.method==="POST")).toBe(true));
  expect(JSON.parse(String(fetch.mock.calls.find(call=>call[1]?.method==="POST")?.[1]?.body))).toEqual({campaign_id:"campaign-selected",package_id:"plan-new",gift_atg:"3.000000000000000001",expected_version:11});
});

it("copies only explicit order and receipt dependencies without unrelated defaults",async()=>{
  const fetch=vi.fn(async(input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify(String(input).includes("/references/orders")?{items:[{value:"order-one",label:"真实订单",defaults:{order_id:"order-one",customer_id:"customer-one",gift_atg:"900",expected_version:99}}]}:String(input).includes("/references/receipts")?{items:[{value:"receipt-one",label:"已核实收款",defaults:{receipt_id:"receipt-one",amount_usd:"4.123456789012345678",currency:"USD",issuer_id:"issuer-one",gift_atg:"800",expected_version:88}}]}:{id:"refund-one",status:"saved"}),{status:200}));vi.stubGlobal("fetch",fetch);
  mount({name:"allocate",label:"关联收款",method:"POST",path:"/api/v1/operators/operator-one/refunds",fields:[{name:"order_id",label:"原订单",kind:"text",required:true,lookup_url:"/api/v1/operators/operator-one/references/orders"},{name:"customer_id",label:"原客户",kind:"text",required:true},{name:"receipt_id",label:"原收款",kind:"text",required:true,lookup_url:"/api/v1/operators/operator-one/references/receipts"},{name:"amount_usd",label:"可分配USD",kind:"decimal",required:true},{name:"currency",label:"原币种",kind:"text",required:true},{name:"issuer_id",label:"原开票方",kind:"text",required:true},{name:"gift_atg",label:"本次赠送",kind:"decimal",required:true,default_value:"2"},{name:"expected_version",label:"退款版本",kind:"hidden",default_value:7}]});
  for(const [label,value]of [["原订单","order-one"],["原收款","receipt-one"]]){const select=screen.getByRole("combobox",{name:label});await waitFor(()=>expect((select as HTMLSelectElement).disabled).toBe(false));fireEvent.change(select,{target:{value}});}
  fireEvent.click(screen.getByRole("button",{name:"关联收款",exact:true}));await waitFor(()=>expect(fetch.mock.calls.some(call=>call[1]?.method==="POST")).toBe(true));
  expect(JSON.parse(String(fetch.mock.calls.find(call=>call[1]?.method==="POST")?.[1]?.body))).toEqual({order_id:"order-one",customer_id:"customer-one",receipt_id:"receipt-one",amount_usd:"4.123456789012345678",currency:"USD",issuer_id:"issuer-one",gift_atg:"2",expected_version:7});
});
