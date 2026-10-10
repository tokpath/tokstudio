/** @vitest-environment jsdom */
import {act} from "react";
import {renderToString} from "react-dom/server";
import {hydrateRoot, type Root} from "react-dom/client";
import {afterEach, describe, expect, it, vi} from "vitest";
import {ModelUsagePanel} from "@/components/model-usage-panel";
import {instructionsHref} from "./model-instructions-context";
import {withZh} from "./test-i18n";

const model={id:"tokenhub/echo-1",display_name:"Echo",vendor:"tokenhub",kind:"text"};
const hydrationMessage=/hydration|hydrated|did not match|server rendered|server-rendered/i;
let root:Root|undefined;
afterEach(async()=>{if(root)await act(()=>root!.unmount());root=undefined;document.body.innerHTML="";vi.restoreAllMocks();vi.unstubAllGlobals();});

function returnTargets(container:HTMLElement) {
 const links=Array.from(container.querySelectorAll("a"));
 const wallet=new URL(links.find(link=>link.textContent==="账户余额 / 充值")!.getAttribute("href")!,"https://brand.example");
 const login=new URL(links.find(link=>link.textContent==="创建 Key")!.getAttribute("href")!,"https://brand.example");
 const create=new URL(login.searchParams.get("next")!,"https://brand.example");
 return {wallet:wallet.searchParams.get("next")!,create:create.searchParams.get("return_to")!};
}

describe("model instructions first render",()=>{
 it.each(["/docs","/docs/integrations","/app/docs","/models/tokenhub/echo-1"])("hydrates %s with the server return target and no mismatched href",async pathname=>{
  const initialHref=instructionsHref(pathname,{key_id:"key-a",tool:"aider",protocol:"/v1/responses",language:"python_sdk",promo:"invite",return_to:"/models?q=echo"},model.id,"protocol");
  window.history.replaceState(null,"",initialHref);
  const clientWindow=window;
  vi.stubGlobal("fetch",vi.fn(()=>new Promise(()=>{})));
  const errors:unknown[][]=[];
  vi.spyOn(console,"error").mockImplementation((...args)=>{errors.push(args);});
  const ui=withZh(<ModelUsagePanel model={model} keyID="key-a" initialHref={initialHref}/>);
  // Exercise the real server branch; jsdom otherwise has window during renderToString.
  vi.stubGlobal("window",undefined);
  let html:string;
  try {html=renderToString(ui);} finally {vi.stubGlobal("window",clientWindow);}
  const container=document.createElement("div");document.body.append(container);container.innerHTML=html!;
  const serverTargets=returnTargets(container);
  for(const target of Object.values(serverTargets)) {
   const back=new URL(target,"https://brand.example");
   expect(back.pathname).toBe(pathname);expect(back.searchParams.get("model")).toBe(model.id);
   expect(back.searchParams.get("key_id")).toBe("key-a");expect(back.searchParams.get("tool")).toBe("aider");
   expect(back.searchParams.get("protocol")).toBe("/v1/responses");expect(back.searchParams.get("language")).toBe("python_sdk");
   expect(back.searchParams.get("promo")).toBe("invite");expect(back.searchParams.get("return_to")).toBe("/models?q=echo");
  }
  await act(async()=>{root=hydrateRoot(container,ui,{onRecoverableError:error=>errors.push([error])});});
  expect(returnTargets(container)).toEqual(serverTargets);
  expect(errors.flat().map(String).filter(message=>hydrationMessage.test(message))).toEqual([]);
 });
});
