/** @vitest-environment jsdom */
import {cleanup, render, screen} from "@testing-library/react";
import {afterEach, beforeEach, describe, expect, it, vi} from "vitest";
const {catalog}=vi.hoisted(()=>({catalog:vi.fn()}));
vi.mock("next/headers",()=>({headers:async()=>new Headers({"x-tokenhub-host":"oem.example"})}));
vi.mock("next-intl/server",()=>({getTranslations:async()=>(key:string)=>key}));
vi.mock("@/lib/catalog",()=>({loadCatalogPage:catalog}));
vi.mock("@/components/model-usage-panel",()=>({ModelUsagePanel:({model,keyID,initialTab,initialHref}:{model:{id:string};keyID:string;initialTab:string;initialHref:string})=><div data-testid="instructions" data-model={model.id} data-key={keyID} data-tab={initialTab} data-href={initialHref}/> }));
import {PublicModelInstructions} from "@/components/public-model-instructions";
import DocsPage from "@/app/docs/page";
import IntegrationsPage from "@/app/docs/integrations/page";

describe("public instructions entries",()=>{
 beforeEach(()=>catalog.mockReset());afterEach(cleanup);
 it("loads the exact model within the current brand and preserves its Key",async()=>{
  catalog.mockResolvedValue({ok:true,items:[{id:"vendor/model-250",display_name:"Model 250"}]});
  const entry=await DocsPage({searchParams:Promise.resolve({model:"vendor/model-250",key_id:"key-a",tab:"agent",tool:"aider"})});
  expect(entry.type).toBe(PublicModelInstructions);render(await PublicModelInstructions(entry.props));
  expect(catalog).toHaveBeenCalledWith("oem.example",{id:"vendor/model-250"});
  expect(screen.getByTestId("instructions").dataset).toMatchObject({model:"vendor/model-250",key:"key-a",tab:"agent"});
  const href=new URL(screen.getByTestId("instructions").dataset.href!,"https://oem.example");
  expect(href.pathname).toBe("/docs");expect(href.searchParams.get("tool")).toBe("aider");expect(href.searchParams.get("key_id")).toBe("key-a");
 });
 it("keeps protocol selection at the integrations entry",async()=>{
  catalog.mockResolvedValue({ok:true,items:[{id:"vendor/text",display_name:"Text"}]});
  const entry=await IntegrationsPage({searchParams:Promise.resolve({model:"vendor/text",key_id:"key-a",tab:"protocol"})});
  expect(entry.type).toBe(PublicModelInstructions);render(await PublicModelInstructions(entry.props));
  expect(screen.getByTestId("instructions").dataset).toMatchObject({key:"key-a",tab:"protocol"});
  expect(new URL(screen.getByTestId("instructions").dataset.href!,"https://oem.example").pathname).toBe("/docs/integrations");
 });
 it("keeps tool, language, invitation and safe return context when choosing a model",async()=>{
  catalog.mockResolvedValue({ok:true,items:[{id:"vendor/text",display_name:"Text"}]});
  render(await PublicModelInstructions({tab:"agent",query:{tool:"aider",language:"node_sdk",promo:"invite",return_to:"/models?q=text"}}));
  const href=screen.getByRole("link",{name:/Text/}).getAttribute("href")!;
  expect(href).toContain("/docs?");expect(href).toContain("tool=aider");expect(href).toContain("promo=invite");expect(href).toContain("language=node_sdk");expect(href).toContain("return_to=%2Fmodels%3Fq%3Dtext");
 });
});
