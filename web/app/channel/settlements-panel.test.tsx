// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,within,waitFor} from '@testing-library/react';
import {withZh} from '@/lib/test-i18n';
import {ChannelCommissionRecords} from './settlements-panel';
vi.mock('next/navigation',()=>({usePathname:()=>'/channel/settlements'}));
vi.mock('@/components/rbac/viewer-context',()=>({useViewer:()=>({userId:'channel-user',roles:['channel_admin'],signedIn:true,loading:false,channelType:'B'})}));
afterEach(()=>{cleanup();vi.unstubAllGlobals();window.history.replaceState(null,'','/');});
const json=(body:unknown,status=200)=>({ok:status<400,status,json:async()=>body});
it('opens an original own settlement outside the first page and retains the request return path',async()=>{
 window.history.replaceState(null,'','/channel/settlements?settlement_id=old-original&return_to=%2Fchannel%2Fusage%3Frequest_id%3Dr1&from=request');
 const calls:string[]=[];vi.stubGlobal('fetch',vi.fn(async(url:unknown)=>{calls.push(String(url));return String(url).endsWith('/settlements/old-original')?json({item:{id:'old-original',status:'paid',amount_minor:1250000,recipient:{email:'own@example.test'},payout_id:'payout-original',payout_occurred_at:'2026-10-10T00:00:00Z',payout_recorded_at:'2026-10-10T01:00:00Z',entries_snapshot_complete:true},entries:[{id:'entry-original',amount_minor:1250000,status:'paid',usage_event_id:'usage-original',policy_version:'policy-original'}]}):json({items:[],total:153,next_cursor:'page2'});}));
 render(withZh(<ChannelCommissionRecords settlements/>));expect(await screen.findByText('我的结算')).toBeTruthy();const detail=await screen.findByRole('dialog');await within(detail).findByText(/payout-original/);expect(detail.textContent).toContain('policy-original');expect(within(detail).queryByRole('checkbox')).toBeNull();expect(within(detail).queryByRole('button',{name:'核对并登记'})).toBeNull();expect(screen.getByText('返回原任务').closest('a')!.getAttribute('href')).toBe('/channel/usage?request_id=r1');expect(calls.some(url=>url.includes('limit=30'))).toBe(true);fireEvent.click(within(detail).getByRole('button',{name:'关闭'}));await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());const p=new URLSearchParams(window.location.search);expect(p.get('settlement_id')).toBeNull();expect(p.get('return_to')).toBe('/channel/usage?request_id=r1');expect(p.get('from')).toBe('request');
});
it('shows authorization failure rather than a fabricated empty detail and rejects a cross-surface return link',async()=>{
 window.history.replaceState(null,'','/channel/settlements?settlement_id=foreign&return_to=%2Fadmin%2Fusage');vi.stubGlobal('fetch',vi.fn(async(url:unknown)=>String(url).endsWith('/settlements/foreign')?json({error:{message:'原单不存在或无权读取'}},404):json({items:[],total:0})));render(withZh(<ChannelCommissionRecords settlements/>));await screen.findByText('原单不存在或无权读取');expect(screen.getByText('返回原任务').closest('a')!.getAttribute('href')).toBe('/channel/settlements');expect(within(screen.getByRole('dialog')).getByRole('button',{name:'重试'})).toBeTruthy();expect(screen.queryByText(/原结算金额:/)).toBeNull();
});
