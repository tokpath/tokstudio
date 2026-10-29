// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { ProcurementCards } from "./procurement-cards";

afterEach(cleanup);
it("keeps stock purchase visible with its preparation notice beside the action", () => {
  render(<ProcurementCards rows={[{id:"quote",status:"quote",values:{record_kind:"quote",gift_tiers:[{from_money:"0",gift_percent:"0"}],purchase_available:true,purchase_notice:"可先采购备货；当前暂不能调用模型。"}}]} title={()=>"OEM stock"} actions={()=><button>按金额采购</button>} />);
  expect(screen.getByRole("status").textContent).toContain("可先采购备货");
  expect((screen.getByRole("button",{name:"按金额采购"}) as HTMLButtonElement).disabled).toBe(false);
});
it("explains an unavailable quote at the disabled purchase button", () => {
  render(<ProcurementCards rows={[{id:"quote",status:"disabled",values:{record_kind:"quote",gift_tiers:[{from_money:"0",gift_percent:"0"}],purchase_available:false,purchase_block_reason:"该报价已停用，请联系供货方确认。"}}]} title={()=>"Stopped quote"} actions={()=>null} />);
  expect((screen.getByRole("button",{name:"按金额采购"}) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByText("该报价已停用，请联系供货方确认。")).toBeTruthy();
});
