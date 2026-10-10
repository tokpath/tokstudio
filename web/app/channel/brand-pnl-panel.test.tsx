// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {withZh} from '@/lib/test-i18n';
import {BrandPnLPanel} from './brand-pnl-panel';
vi.mock('next/navigation',()=>({usePathname:()=>'/channel/ledger'}));
afterEach(()=>{cleanup();vi.unstubAllGlobals();});
const json=(body:unknown,status=200)=>({ok:status<400,status,json:async()=>body});
const pnl={channel_org_id:'owner',pool_status:'available',recharge_minor:1000000,unconsumed_minor:100000,consumed_minor:123,marketing_minor:-3,supplier_minor:-20,pnl_minor:100,sell_minor:123,model_cost_minor:20,margin_minor:103,purchase_minor:1000000,business_type:"oem"};
it('shows loading before actual amounts, preserves micro-dollar consumption and marks a failed refresh',async()=>{
 let resolve!:(value:unknown)=>void;let n=0;vi.stubGlobal('fetch',vi.fn(()=>++n===1?new Promise(r=>resolve=r):Promise.reject(new Error('offline'))));render(withZh(<BrandPnLPanel scope="person:owner" ownerID="owner" path="/channel/pnl"/>));expect(screen.queryByText('$0.00 USD')).toBeNull();expect(screen.getByTestId('list-resource').getAttribute('data-list-phase')).toBe('loading');resolve(json({pnl}));await screen.findByText('$0.000123 USD');fireEvent.click(screen.getByRole('button',{name:'刷新'}));await screen.findByRole('alert');expect(screen.getByTestId('list-resource').getAttribute('data-list-phase')).toBe('stale');expect(screen.getByText('$0.000123 USD')).toBeTruthy();
});
it('isolates late brand responses and represents an unestablished pool without inventing a balance',async()=>{
 let resolve!:(value:unknown)=>void;vi.stubGlobal('fetch',vi.fn((url:unknown)=>String(url).endsWith('/one')?new Promise(r=>resolve=r):Promise.resolve(json({pnl:{...pnl,channel_org_id:'two',pool_status:'not_established',consumed_minor:2000000,sell_minor:2000000}}))));const view=render(withZh(<BrandPnLPanel key="one" scope="person:one" ownerID="one" path="/one"/>));view.rerender(withZh(<BrandPnLPanel key="two" scope="person:two" ownerID="two" path="/two"/>));await screen.findByText('$2.00 USD');resolve(json({pnl:{...pnl,channel_org_id:'one',consumed_minor:99000000,sell_minor:99000000}}));await waitFor(()=>expect(screen.queryByText('$99.00 USD')).toBeNull());expect(screen.getAllByText('服务池尚未建立')).toHaveLength(2);
});
