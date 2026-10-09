/** @vitest-environment jsdom */
import {cleanup,render,screen,waitFor} from '@testing-library/react';
import {afterEach,expect,it,vi} from 'vitest';
import {withZh} from '@/lib/test-i18n';
import {ChargeRefundPanel} from './charge-refund-panel';
vi.mock('next/navigation',()=>({useSearchParams:()=>new URLSearchParams('request_id=req_original&return_to=%2Fadmin%2Fusage%2Frequests%2Freq_original%3Freturn_to%3D%252Fadmin%252Fusage%253Ftab%253Drequests')}));
afterEach(()=>{cleanup();vi.unstubAllGlobals()});
it('loads the original contextual charge and returns to its complete request chain without writing',async()=>{
 const fetcher=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>({ok:true,json:async()=>({item:{request_id:'req_original',user_id:'u',model_id:'Echo',state:'committed',amount_minor:1500000,wallet_minor:1000000,entitlement_minor:500000,gift_minor:0},user:{email:'alice@example.test',display_name:''}})}));vi.stubGlobal('fetch',fetcher);render(withZh(<ChargeRefundPanel onRefund={vi.fn()}/>));
 await screen.findByText(/原消费：\$1.50 USD/);expect(screen.getByLabelText('消费请求编号')).toHaveProperty('value','req_original');expect(screen.getByRole('link',{name:'返回原请求'}).getAttribute('href')).toBe('/admin/usage/requests/req_original?return_to=%2Fadmin%2Fusage%3Ftab%3Drequests');expect(fetcher.mock.calls[0][0]).toContain('request_id=req_original');expect(fetcher.mock.calls.some(([,init])=>init?.method==='POST')).toBe(false);
});
it('shows preview failure without claiming an amount or enabling reversal',async()=>{
 vi.stubGlobal('fetch',vi.fn(async()=>({ok:false,json:async()=>({error:{message:'原账单不可读'}})})));render(withZh(<ChargeRefundPanel onRefund={vi.fn()}/>));await waitFor(()=>expect(screen.getByRole('alert').textContent).toContain('原账单不可读'));expect(screen.queryByRole('button',{name:'核对并退消费账单'})).toBeNull();expect(screen.queryByText(/原消费：/)).toBeNull();
});
