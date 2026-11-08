import { Decimal } from "./contracts";

const scale=10n**18n;
function atoms(value:string):bigint|undefined { if(!Decimal.safeParse(value).success)return;const [a,b=""]=value.split(".");if(b.length>18)return;return BigInt(a+b.padEnd(18,"0")); }
function text(value:bigint):string {const raw=value.toString().padStart(19,"0"),fraction=raw.slice(-18).replace(/0+$/,"");return raw.slice(0,-18)+(fraction?"."+fraction:"");}
export function previewCollectionFee(amount:string,flat:string,rate:string,digits:number) {
 const a=atoms(amount),f=atoms(flat),r=atoms(rate);
 if(a===undefined||f===undefined||r===undefined||a<=0n||digits<0||digits>8)return;
 const denominator=10n**BigInt(36-digits),numerator=f*scale+a*r;
 const fee=(numerator*2n+denominator)/(denominator*2n)*10n**BigInt(18-digits);
 return {amount:text(a),fee:text(fee),total:text(a+fee)};
}
