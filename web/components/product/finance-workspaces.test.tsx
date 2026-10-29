// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { FinanceReceiptsScreen, PaymentMethodsScreen } from "./finance-workspaces";
import { FinanceEvidenceLinks, FinanceEvidenceUpload } from "./finance-evidence";

vi.mock("./operator-resources",()=>({ResourceScreen:({resource}:{resource:string})=><div data-testid="resource">{resource}</div>}));
afterEach(()=>{cleanup();vi.unstubAllGlobals();window.history.replaceState({},"","/");});

it("keeps pending receipts in the receipt workspace and resets unrelated filters when switching",async()=>{
  window.history.replaceState({},"","/manage/operator/payments?section=pending&q=old-search");
  render(<FinanceReceiptsScreen operatorID="operator"/>);
  await waitFor(()=>expect(screen.getByTestId("resource").textContent).toBe("receipt-valuations"));
  fireEvent.click(screen.getByRole("button",{name:"已入账收款"}));
  expect(screen.getByTestId("resource").textContent).toBe("payments");
  expect(window.location.search).toBe("");
  fireEvent.click(screen.getByRole("button",{name:"待核实收款"}));
  expect(window.location.search).toBe("?section=pending");
});

it("offers payment channels without an exchange-rate configuration entry",()=>{
  render(<PaymentMethodsScreen operatorID="operator"/>);
  expect(screen.getAllByTestId("resource").map(node=>node.textContent)).toEqual(["payment-methods"]);
  expect(screen.queryByText("外币收款报价依据")).toBeNull();
  expect(screen.queryByText("valuations")).toBeNull();
});

it("retries an uncertain private attachment with its original file and key",async()=>{
  const attach=vi.fn(),pending=vi.fn();
  const response=vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({error:{code:"temporary_failure",message:"retry",retryable:true}}),{status:503})).mockResolvedValueOnce(new Response(JSON.stringify({id:"0b15a9e1-7a65-4f36-826f-067a8a481135",status:"ready"}),{status:200}));
  vi.stubGlobal("fetch",response);
  const cache=new QueryClient({defaultOptions:{queries:{retry:false}}});
  render(<QueryClientProvider client={cache}><FinanceEvidenceUpload operatorID="operator" disabled={false} attach={attach} onPendingChange={pending}/></QueryClientProvider>);
  fireEvent.click(screen.getByText("附加付款凭据（可选）"));
  const file=new File(["synthetic evidence"],"bank.png",{type:"image/png"});
  fireEvent.change(screen.getByLabelText("选择付款凭据"),{target:{files:[file]}});
  fireEvent.click(screen.getByRole("button",{name:"上传并关联凭据"}));
  await screen.findByRole("button",{name:"重试原附件上传"});
  fireEvent.click(screen.getByRole("button",{name:"重试原附件上传"}));
  await waitFor(()=>expect(attach).toHaveBeenCalledWith("tokenhub-evidence:0b15a9e1-7a65-4f36-826f-067a8a481135"));
  expect(response).toHaveBeenCalledTimes(2);
  const first=response.mock.calls[0][1],second=response.mock.calls[1][1];
  expect(first.headers.get("Idempotency-Key")).toBe(second.headers.get("Idempotency-Key"));
  expect(first.body).toBe(second.body);
  expect(first.body.get("file")).toBe(file);
  expect(pending).toHaveBeenCalledWith(true);
});

it("exposes attachments through the current operator's authorized download endpoint",()=>{
  render(<FinanceEvidenceLinks operatorID="operator" value={{evidence:"bank statement\ntokenhub-evidence:0b15a9e1-7a65-4f36-826f-067a8a481135"}}/>);
  expect(screen.getByRole("link",{name:"查看付款凭据 1"}).getAttribute("href")).toBe("/api/v1/operators/operator/evidence/0b15a9e1-7a65-4f36-826f-067a8a481135/content");
});
