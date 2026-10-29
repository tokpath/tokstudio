import { expect, test } from "@playwright/test";
import { chooseControl } from "./choose-control";
const base="/api/v1/operators/operator-one";
const tiers=[{from_money:"0",gift_percent:"0"},{from_money:"1000",gift_percent:"1"},{from_money:"2000",gift_percent:"2"}];
const purchase={name:"purchase",label:"按金额采购",method:"POST",path:`${base}/procurement/{id}/purchase`,fields:[{name:"amount_money",label:"本次采购结算币种",kind:"decimal",required:true}]};
const quote={name:"quote",label:"向直属采购方发布供货报价",method:"POST",path:`${base}/procurement`,fields:[{name:"action",label:"办理类型",kind:"hidden",default_value:"quote"},{name:"buyer_id",label:"直属采购方",kind:"text",lookup_url:`${base}/references/buyers`,required:true},{name:"gift_tiers",label:"阶梯赠送",kind:"json",required:true,default_value:[{from_money:"0",gift_percent:"0"}]}]};
function resource(items:unknown[]=[],extra:Record<string,unknown>={}) {return {title:"采购、供货与自动补货",description:"每单一笔足额付款，核实后自动交付。",columns:[],operations:[],row_operations:[],items,...extra};}
test("procurement has real filters, exact purchase previews and a named tier editor",async({page})=>{
 const reads:URL[]=[],writes:{path:string;body:any}[]=[];
 await page.route("**/api/v1/**",async route=>{
  const req=route.request(),url=new URL(req.url()),path=url.pathname;
  let out:any=resource();
  const operator={id:"operator-one",name:"山海经营方",legal_name:"山海服务主体",kind:"oem",brand_id:"brand-one"};
  if(path==="/api/v1/public/context")out={brand:{id:"brand-one",name:"山海",api_base_url:"http://127.0.0.1/v1",theme:{}},operator,theme:{},monetary:{currency:"CNY",atg_per_unit:"2.5",status:"locked",version:1},availability:[],payment_methods:[]};
  else if(path==="/api/v1/auth/me")out={kind:"staff",staff:{id:"staff-one",email:"admin@tokenhub.local",display_name:"负责人"},workspaces:[{operator,permissions:["*"]}],active_operator_id:operator.id,csrf_token:"csrf_test"};
  else if(req.method()==="POST") {writes.push({path,body:req.postDataJSON()});out={id:"created-record",status:"published"};}
  else if(path===`${base}/references/buyers`)out={items:[{value:"buyer-one",label:"直属渠道",defaults:{buyer_id:"buyer-one"}}]};
  else if(path===`${base}/procurement`){
   reads.push(url);const supplier=url.searchParams.get("view")==="supplier";
   const isQuotes=url.searchParams.get("section")==="quotes";
   out=resource(isQuotes?[{id:"quote-one",status:"quote",values:{record_kind:"quote",name:"日常阶梯报价",gift_tiers:tiers},allowed_actions:supplier?[]:["purchase"]}]:[],{search_supported:true,status_supported:true,views:[{value:"buyer",label:"我方采购"},{value:"supplier",label:"我方供货"}],columns:[{key:"gift_tiers",label:"分段赠送规则",kind:"details"}],operations:supplier?[quote]:[],row_operations:supplier?[]:[purchase]});
  }
  await route.fulfill({status:200,contentType:"application/json",body:JSON.stringify(out)});
 });
 await page.goto("/manage/operator-one/procurement");
 await expect(page.getByRole("textbox",{name:"搜索记录"})).toBeVisible();
 await expect(page.getByRole("button",{name:"保存常用筛选"})).toHaveCount(0);
 await page.getByRole("tab",{name:"采购报价",exact:true}).click();
 await expect(page.getByText("日常阶梯报价",{exact:true})).toBeVisible();
 await expect.poll(()=>reads.at(-1)?.searchParams.get("section")).toBe("quotes");
 await page.getByRole("textbox",{name:"搜索记录"}).fill("日常");await page.getByRole("button",{name:"查询",exact:true}).click();
 await expect.poll(()=>reads.at(-1)?.searchParams.get("q")).toBe("日常");
 await page.getByRole("button",{name:"按金额采购",exact:true}).click();
 let panel=page.getByRole("dialog").last();
 await panel.getByLabel("本次采购CNY",{exact:false}).fill("2500");
 await expect(panel.getByText("购买 6250 ATG + 赠送 50 ATG = 合计 6300 ATG")).toBeVisible();
 await panel.getByRole("button",{name:"按金额采购",exact:true}).click();
 await expect.poll(()=>writes.length).toBe(1);expect(writes[0]).toEqual({path:`${base}/procurement/quote-one/purchase`,body:{amount_money:"2500"}});
 await panel.getByRole("button",{name:"关闭办理面板",exact:true}).click();
 await page.getByRole("button",{name:"我方供货",exact:true}).click();
 await page.getByRole("tab",{name:"供货报价",exact:true}).click();
 await page.getByRole("button",{name:"向直属采购方发布供货报价",exact:true}).click();
 panel=page.getByRole("dialog").last();
 await chooseControl(panel.getByRole("combobox",{name:"直属采购方",exact:true}),"buyer-one");
 await panel.getByRole("button",{name:"添加赠送阶梯",exact:true}).click();
 await panel.getByLabel("本段赠送 %").nth(1).fill("1");
 await panel.getByRole("button",{name:"添加赠送阶梯",exact:true}).click();
 await panel.getByLabel("本段赠送 %").nth(2).fill("2");
 await expect(panel.getByText("购买 5000 ATG + 赠送 25 ATG = 合计 5025 ATG")).toBeVisible();
 await panel.getByRole("button",{name:"向直属采购方发布供货报价",exact:true}).click();
 await expect.poll(()=>writes.length).toBe(2);expect(writes[1].body).toEqual({action:"quote",buyer_id:"buyer-one",gift_tiers:tiers});
});
