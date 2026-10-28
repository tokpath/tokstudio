import { afterEach, expect, it, vi } from "vitest";
import { dateRange, localDay, monthRange, rangeDays } from "./report-period";

afterEach(()=>vi.useRealTimers());
it("includes the end day in local time and caps today at the query time",()=>{
  const now=new Date(2026,9,28,14,30);
  expect(dateRange("2026-10-01","2026-10-28",now)).toEqual({from:new Date(2026,9,1).toISOString(),to:now.toISOString()});
  const past=dateRange("2026-09-01","2026-09-30",now);
  expect(past.to).toBe(new Date(2026,9,1).toISOString());
  expect(rangeDays(past)).toEqual({from:"2026-09-01",to:"2026-09-30"});
  expect(()=>dateRange("2026-02-30","2026-03-01",now)).toThrow();
  expect(()=>dateRange("2026-10-29","2026-10-30",now)).toThrow();
  expect(()=>dateRange("2024-01-01","2026-01-01",now)).toThrow();
});
it("defaults to this month without UTC date drift",()=>{
  vi.useFakeTimers();vi.setSystemTime(new Date(2026,9,28,2));
  const range=monthRange();expect(localDay(new Date(range.from))).toBe("2026-10-01");expect(rangeDays(range).to).toBe("2026-10-28");
});
