// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {withZh} from '@/lib/test-i18n';
import ChannelLedger from './ledger-panel';
const viewer=vi.hoisted(()=>({userId:'oem-finance',roles:['oem_finance'],signedIn:true,loading:false,channelType:'C'}));
vi.mock('@/components/rbac/viewer-context',()=>({useViewer:()=>viewer}));
vi.mock('next/navigation',()=>({usePathname:()=>'/channel/ledger'}));
afterEach(()=>{cleanup();vi.unstubAllGlobals();window.history.replaceState(null,'','/');viewer.userId='oem-finance';});
const json=(body:unknown,status=200)=>({ok:status<400,status,json:async()=>body});
const context={owner_id:'oem-one',owner_name:'品牌一',channel_ids:['oem-one','child'],channel_codes:{child:'渠道一'}};
const allocation={id:'allocation-old',user_id:'customer',recipient:{email:'customer@example.test'},granted_minor:1000000,consumed_minor:123,remaining_minor:999877,status:'active',source_type:'payment',source_id:'order-original',created_at:'2026-10-10T00:00:00Z'};
it('reads all allocation pages, preserves attribution and return parameters, and links to real payment issuance',async()=>{
 window.history.replaceState(null,'','/channel/ledger?tab=issuance&allocation_q=customer&channel_id=child&next=%2Fchannel%2Fcommission&from=commissions');const calls:string[]=[];
 vi.stubGlobal('fetch',vi.fn(async(url:unknown)=>{calls.push(String(url));if(String(url).includes('/commission-context'))return json(context);if(String(url).includes('/allocations?'))return json({items:[allocation],total:153,next_cursor:String(url).includes('cursor=page2')?'':'page2'});return json({items:[],total:0});}));render(withZh(<ChannelLedger/>));await screen.findByText('customer@example.test');expect(screen.getByText(/共 153 条/)).toBeTruthy();fireEvent.click(screen.getByRole('button',{name:'下一页'}));await waitFor(()=>expect(calls.some(url=>url.includes('cursor=page2')&&url.includes('q=customer')&&url.includes('channel_id=child'))).toBe(true));const href=screen.getByRole('link',{name:'前往订单办理线下划拨'}).getAttribute('href')!;expect(href.startsWith('/channel/payments/orders?')).toBe(true);expect(new URL(href,'https://test.invalid').searchParams.get('next')).toContain('tab=issuance');expect(new URLSearchParams(window.location.search).get('from')).toBe('commissions');expect(calls.length).toBeLessThan(7);
});
it('does not keep the previous user book after viewer changes or accept a late response from it',async()=>{
 let resolve!:(value:unknown)=>void;let contextCalls=0;vi.stubGlobal('fetch',vi.fn(async(url:unknown)=>{if(String(url).includes('/commission-context'))return ++contextCalls===1?new Promise(r=>resolve=r):json({error:{message:'品牌权限已变更'}},403);return json({items:[],total:0});}));const view=render(withZh(<ChannelLedger/>));await waitFor(()=>expect(contextCalls).toBe(1));viewer.userId='other-user';view.rerender(withZh(<ChannelLedger/>));await screen.findByText('品牌权限已变更');resolve(json(context));await waitFor(()=>expect(screen.queryByText('品牌一',{exact:false})).toBeNull());expect(screen.getByRole('button',{name:'重试'})).toBeTruthy();expect(screen.queryByText('$0.00 USD')).toBeNull();
});
