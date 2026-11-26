#!/usr/bin/env node
// Build the public contract from the same Zod output schemas used by the browser.
// No server, database, secret, or generated customer data is required.
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),{createRequire}=require('node:module');
const root=path.resolve(__dirname,'..'),req=createRequire(path.join(root,'web/package.json')),ts=req('typescript');
const source=fs.readFileSync(path.join(root,'web/lib/contracts.ts'),'utf8');const compiled=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText;const out={exports:{}};vm.runInNewContext(compiled,{module:out,exports:out.exports,require:req});
const stringFormats = new Map([['email', 'email'], ['uuid', 'uuid'], ['datetime', 'date-time'], ['url', 'uri']]);
function schema(zodSchema, depth = 0) {
  // Keep recursive JSON values inline at the same depth as the reviewed contract.
  if (depth > 18) return {};
  const definition = zodSchema.def;
  const child = item => schema(item, depth + 1);
  switch (definition.type) {
    case 'string': {
      const result = { type: 'string' };
      const checks = [definition, ...(definition.checks ?? []).map(check => check._zod.def)];
      for (const check of checks) {
        if (check.format === 'regex') result.pattern = check.pattern.source;
        if (check.check === 'min_length') result.minLength = check.minimum;
        if (check.check === 'max_length') result.maxLength = check.maximum;
        const format = stringFormats.get(check.format);
        if (format) result.format = format;
      }
      return result;
    }
    case 'number':
      return { type: zodSchema.format?.includes('int') ? 'integer' : 'number' };
    case 'boolean':
    case 'null':
      return { type: definition.type };
    case 'literal': {
      if (definition.values.length !== 1) throw new Error('Expected a single OpenAPI literal value');
      const [value] = definition.values;
      return { const: value, type: typeof value };
    }
    case 'enum':
      return { type: 'string', enum: zodSchema.options };
    case 'array':
      return { type: 'array', items: child(definition.element) };
    case 'object': {
      const fields = Object.entries(definition.shape);
      const properties = Object.fromEntries(fields.map(([key, field]) => [key, child(field)]));
      const required = fields.filter(([, field]) => !field.safeParse(undefined).success).map(([key]) => key);
      return { type: 'object', properties, ...(required.length ? { required } : {}), additionalProperties: definition.catchall?.def.type !== 'never' };
    }
    case 'optional':
    case 'default':
    case 'prefault':
      return child(definition.innerType);
    case 'nullable':
      return { anyOf: [child(definition.innerType), { type: 'null' }] };
    case 'union': {
      const options = definition.options.map(child);
      if (definition.discriminator) return { oneOf: options, discriminator: { propertyName: definition.discriminator } };
      return { anyOf: options };
    }
    case 'record':
      return { type: 'object', additionalProperties: child(definition.valueType) };
    case 'lazy':
      return child(definition.getter());
    default:
      throw new Error(`Unsupported Zod schema type for OpenAPI: ${definition.type}`);
  }
}
const schemas={};for(const[k,v]of Object.entries(out.exports))if(k.endsWith('Schema')||k==='Decimal'||k==='SignedDecimal')schemas[k.replace(/Schema$/,'')]=schema(v);
const ref=n=>({$ref:'#/components/schemas/'+n}),str={type:'string'},bool={type:'boolean'},uuid={type:'string',format:'uuid'},integer={type:'integer',minimum:0},decimal=ref('LedgerDecimal'),priceDecimal=ref('Decimal'),obj=(p,r=Object.keys(p))=>({type:'object',properties:p,required:r,additionalProperties:false}),array=s=>({type:'array',items:s}),collection=n=>obj({items:array(ref(n)),next_cursor:{type:['string','null']}},['items']);
schemas.LedgerDecimal={type:'string',pattern:'^(0|[1-9][0-9]{0,19})(\\.[0-9]{1,18})?$',description:'Exact unsigned NUMERIC(38,18) ledger amount; never round incoming values.'};
schemas.SignedLedgerDecimal={type:'string',pattern:'^-?(0|[1-9][0-9]{0,19})(\\.[0-9]{1,18})?$',description:'Exact signed NUMERIC(38,18) ledger quantity.'};
schemas.NewPassword={type:'string',minLength:10,maxLength:30,pattern:'^[!-~]+$',allOf:[{anyOf:[
 {pattern:'(?=.*[0-9])(?=.*[A-Z])'},{pattern:'(?=.*[0-9])(?=.*[a-z])'},{pattern:'(?=.*[0-9])(?=.*[^0-9A-Za-z])'},
 {pattern:'(?=.*[A-Z])(?=.*[a-z])'},{pattern:'(?=.*[A-Z])(?=.*[^0-9A-Za-z])'},{pattern:'(?=.*[a-z])(?=.*[^0-9A-Za-z])'}
]}],writeOnly:true,description:'10–30 visible ASCII characters from at least two categories: digits, uppercase letters, lowercase letters, and symbols.'};
Object.assign(schemas,{LoginInput:obj({kind:{enum:['customer','staff']},email:{type:'string',format:'email'},password:str},['email','password']),RegistrationInput:obj({email:{type:'string',format:'email'},password:ref('NewPassword'),display_name:str,invite_code:{type:'string',pattern:'^([0-9A-Za-z]{6})?$',description:'Six digits/Latin letters; case-insensitive. Empty or omitted means brand-direct registration.'},next:{type:'string',maxLength:2048,description:'Optional continuation to /workspace/models, /workspace/plans or /workspace/playground. The server retains only allowed model/plan/kol_activity parameters in the verification email; invitation ownership is separate.'}},['email','password','display_name']),KeyInput:obj({name:str,model_ids:{anyOf:[array(str),{type:'null'}]},budget:obj({limit_atg:{anyOf:[decimal,{type:'null'}]},period:{enum:['lifetime','month','quarter','year']},timezone:str},['limit_atg','period']),rate_limit_rpm:{type:['integer','null'],minimum:1},concurrency_limit:{type:['integer','null'],minimum:1},version:integer},['name','model_ids','budget','rate_limit_rpm','concurrency_limit']),OrderInput:obj({quote_id:uuid,terms_version:str,funding:obj({money_balance:decimal,commission_money:decimal,external_method_id:uuid,fee_rule_id:uuid},['money_balance','commission_money']),auto_renew:bool,allow_commission:bool}),QuoteInput:obj({plan_id:uuid,operator_activity_id:uuid,kol_activity_id:uuid},['plan_id']),TopUpInput:obj({amount_money:decimal,external_method_id:uuid,fee_rule_id:uuid},["amount_money","external_method_id"]),ReferencePage:obj({items:array(obj({value:str,label:str,defaults:{type:'object',additionalProperties:true}},['value','label'])),next_cursor:str},['items']),InvoiceInput:obj({issuer_id:uuid,lines:array(obj({order_id:uuid})),details:obj({title:str,tax_id:str,email:{type:'string',format:'email'},address:str},['title','tax_id','email'])},['lines','details']),Destination:obj({network:{enum:['bank']},asset:str,address:str},['network','asset','address']),DestinationProposal:obj({destination:ref('Destination'),authorization_reference:str,responsible_execution:bool},['destination','authorization_reference']),PlaygroundInput:obj({model:str,capability:{enum:['chat','responses','embedding','image','speech','transcription','video']},input:str,voice:str,response_format:str,seconds:str,parameters:{type:'object',additionalProperties:{anyOf:[str,{type:'number'},bool]}}},['model','capability']),VersionInput:obj({version:integer}),NotificationPreferencesInput:obj({marketing:bool,usage_email:bool})});
const paths={};const commonError={'400':'Invalid input or missing idempotency key','401':'Authentication required','403':'Wrong tenant, permission, origin, CSRF or frozen action','404':'Not found in authenticated scope','409':'Business state, configuration, version, or unknown outcome requires review','410':'Temporary result expired or unavailable','429':'Request budget, rate, or concurrency limit','503':'Dependency or verified route unavailable'};
const json=s=>({'application/json':{schema:typeof s==='string'?ref(s):s}}),query=(name,description)=>({name,in:'query',required:false,description,schema:str});
function add(method,url,summary,response,body,security='browser',extra={}){
 const [pathname,search='']=url.split('?');
 if(!pathname.startsWith('/v1/')||url.includes('#')||url.split('?').length>2)throw new Error('Invalid API operation URL '+url);
 const writes=!['get','head'].includes(method),parameters=[];
 for(const m of pathname.matchAll(/\{([^}]+)\}/g))parameters.push({name:m[1],in:'path',required:true,schema:str});
 for(const[name,value]of new URLSearchParams(search))parameters.push({name,in:'query',required:false,description:name==='target_operator_id'?'Exact selected operator target. Omission selects the current operator only where its domain authority permits; it never grants membership or access to the target workspace.':'Workflow query parameter.',schema:name.endsWith('_id')?uuid:/^\{[^}]+\}$/.test(value)?str:{type:'string',enum:[value]}});
 if(writes&&security!=='webhook'){parameters.push({$ref:'#/components/parameters/IdempotencyKey'});if(security==='browser')parameters.push({$ref:'#/components/parameters/CSRF'})}
 const operation={operationId:method+'_'+pathname.replace(/[^a-zA-Z0-9]+/g,'_'),summary,tags:[security==='model'?'Model API':pathname.includes('/operators/')?'Operator browser API':pathname.includes('/auth/')?'Authentication':'Customer browser API'],security:security==='public'?[]:security==='model'?[{ModelKey:[]}]:security==='webhook'?[]:[{BrowserSession:[]}],parameters,responses:{'200':{description:'Successful response',content:json(response||'CommandResult')},...Object.fromEntries(Object.entries(commonError).map(([status,description])=>[status,{description,content:json('APIError')}]))},...extra};
 if(body)operation.requestBody={required:true,content:json(body)};paths[pathname]??={};paths[pathname][method]=operation;return operation;
}
// One global monetary foundation, independent of each operator's business switch.
const timestamp={type:'string',format:'date-time'};
schemas.Monetary=obj({currency:{type:'string',pattern:'^[A-Z]{3}$',not:{enum:['XXX','XTS']},description:'Global settlement ISO 4217 currency; XXX and XTS are rejected.'},atg_per_unit:{...decimal,description:'ATG granted for one settlement-currency unit. Strictly positive; not a foreign-exchange rate.'},status:{enum:['unconfigured','scheduled','locked']},effective_at:timestamp,version:{type:'integer',minimum:1},configured_by:str,confirmed_at:timestamp},['currency','atg_per_unit','status','version']);
schemas.MonetaryConfigurationInput=obj({currency:{type:'string',pattern:'^[A-Z]{3}$',not:{enum:['XXX','XTS']},description:'Selected ISO 4217 settlement currency, excluding XXX and XTS.'},atg_per_unit:{allOf:[decimal,{not:{pattern:'^0(?:\\.0+)?$'}}],description:'Strictly positive ATG per one settlement-currency unit.'},effective_at:{...timestamp,description:'Must be strictly later than the database current time. The settings lock permanently when this time arrives.', 'x-after-database-time':true},expected_version:integer,confirmed:{type:'boolean',const:true,description:'Explicit confirmation of the selected currency, ATG ratio, and irreversible lock at the effective time.'}});
schemas.MonetaryCancelInput=obj({expected_version:integer});
add('get','/v1/monetary/settings','Read the global settlement currency, ATG ratio and effective status','Monetary',null,'public',{tags:['Monetary foundation'],description:'Public readiness facts. Before effectiveness, customer transactions and grants remain unavailable. Each operator business switch is separate; stopping business never unlocks effective monetary settings.'});
const monetaryOwner={tags:['Monetary foundation'],'x-required-role':'owner','x-required-role-status':'active','x-authority-scope':'installation.platform_operator_id'};
const configureMonetary=add('post','/v1/monetary/settings','Confirm and schedule the global monetary foundation','CommandResult','MonetaryConfigurationInput','browser',{...monetaryOwner,description:'Requires an active real owner membership on the installed platform operator, verified from the authenticated staff session. Broad permissions, inherited authority or a supplied actor ID are insufficient. confirmed must be true and expected_version must match the current row. effective_at must be in the future according to the database clock. Only unconfigured settings can be scheduled; an existing schedule must first be cancelled. At effectiveness the currency and ATG ratio become permanently immutable, even when every business switch is off. No exchange rate is fetched, stored or inferred.', 'x-permanently-locked-on-effect':true});
configureMonetary.responses['409'].description='Current version mismatch, an existing uncancelled schedule, or permanently locked effective settings.';
const cancelMonetary=add('post','/v1/monetary/cancel','Cancel a scheduled monetary foundation before it takes effect','CommandResult','MonetaryCancelInput','browser',{...monetaryOwner,description:'Requires the same active real platform owner membership and current expected_version. Cancellation succeeds only while a schedule exists and the database time is strictly before effective_at. The currency and ratio become configurable again after cancellation; cancellation after effectiveness is permanently forbidden.', 'x-required-monetary-status':'scheduled'});
cancelMonetary.responses['409'].description='Current version mismatch, no pending schedule, or permanently locked effective settings.';
add('get','/v1/public/context','Resolve brand, actual merchant, inherited theme and business availability','PublicContext',null,'public',{parameters:[query('invite_code','Optional explicit invitation; invalid invitation is not silently discarded.')]});
for(const[r,n]of[['models','Model'],['plans','Plan']])add('get','/v1/public/'+r,'Public '+r+' in resolved merchant context',collection(n),null,'public');
const publicLogo=add('get','/v1/branding/assets/{id}/content','Read the current brand or channel logo without exposing private object storage',{},null,'public');
publicLogo.parameters.push({name:'operator_id',in:'query',required:false,schema:uuid,description:'The actual channel operator within the resolved brand; omission selects the brand owner.'});
publicLogo.responses['200'].content=Object.fromEntries(['image/png','image/jpeg','image/webp'].map(type=>[type,{schema:{type:'string',contentEncoding:'binary'}}]));
const ownLogo=add('get','/v1/operators/{operatorId}/branding/assets/{id}/content','Read the published logo for an authenticated member of this workspace',{},null,'browser',{description:'Requires an active staff membership in the exact workspace, including read-only staff. Does not grant appearance editing or access to another operator. Supports OEM and channel headers on a shared login hostname.'});ownLogo.responses['200'].content=publicLogo.responses['200'].content;
add('post','/v1/auth/login','Detect the authenticated account; choose a workspace only if both credentials match','LoginResult','LoginInput','public');add('get','/v1/auth/me','Current independent customer or staff permission context','Session',null,'public');add('post','/v1/auth/staff-password','Change staff password and revoke other sessions','CommandResult',obj({current_password:str,new_password:ref('NewPassword')}));add('post','/v1/auth/language','Remember the signed-in recipient’s UI language for system emails, without changing notification consent','CommandResult',obj({operator_id:uuid,language:{enum:['zh','en','ja','ru','hi','ms','th']}},['operator_id','language']));add('post','/v1/auth/logout','Revoke current browser session','CommandResult',obj({}));add('post','/v1/auth/registrations','Begin registration and enqueue actual verification email','RegistrationResult','RegistrationInput','public');add('post','/v1/auth/verify','Verify registration or email change token','CommandResult',obj({token:str}),'public');
add('post','/v1/auth/password-resets','Queue password reset without disclosing account existence','CommandResult',obj({email:{type:'string',format:'email'}}),'public',{description:'Customer recovery uses the account and mail service of the requested brand. Staff recovery is available through the official platform brand.'});
add('post','/v1/auth/password-resets/complete','Complete a password reset within the issuing brand','CommandResult',obj({token:str,password:ref('NewPassword')}),'public');add('post','/v1/auth/staff-invitations/accept','Accept actual staff invitation with password or verified existing identity','CommandResult',obj({token:str,password:{type:'string',writeOnly:true,description:'Current password for an existing staff account. For a new account, use the NewPassword schema.'},display_name:str},['token','password']),'public');
add('get','/v1/me/credits','Exact available, held, purchased, gifted, wallet and commission balances','Credits');for(const[r,n]of[['models','Model'],['plans/catalog','Plan'],['plans','Subscription'],['keys','Key'],['orders','Order'],['requests','Request'],['commissions','Commission'],['notifications','Notification'],['refunds','Settlement']]){const op=add('get','/v1/me/'+r,'Authenticated own '+r,collection(n));if(r==='plans')op.parameters.push(query('cursor','Opaque cursor returned by next_cursor.'));if(['orders','requests','commissions','notifications'].includes(r))op.parameters.push(query('cursor','Opaque cursor returned by next_cursor; do not decode or construct.'),query('q','Server-side bounded search.'))}
add('get','/v1/me/referrals','Own invitation and two-hop qualification facts','Referral');add('post','/v1/me/keys','Create API key; full secret is displayed once','KeySecret','KeyInput');add('get','/v1/me/keys/{id}/secret','Read the stored API key so its owner can copy it again','KeyReveal');add('patch','/v1/me/keys/{id}','Change API key configuration; current budget usage is retained','Key','KeyInput');for(const action of['enable','disable','reset'])add('post',`/v1/me/keys/{id}/${action}`,action+' API key; already admitted requests keep settlement',action==='reset'?'KeySecret':'CommandResult','VersionInput');add('delete','/v1/me/keys/{id}','Delete API key without deleting financial history','CommandResult','VersionInput');
add('post','/v1/me/orders/quotes','Preview exact price and benefits without persisting activity capacity','Quote','QuoteInput');add('post','/v1/me/orders','Create order against quote; reuse same idempotency key on uncertain result','Order','OrderInput');add('get','/v1/me/orders/{id}','Own order, receipt and payment instructions','Order');add('post','/v1/me/orders/{id}/reconcile','Query original external payment; never create another payment','CommandResult',obj({}));add('post','/v1/me/orders/{id}/cancel','Cancel an order before any payment is recorded and release its holds','Order',obj({}));add('post','/v1/me/orders/{id}/payment-method','Replace an unpaid checkout method and open the new payment instructions','Order',obj({external_method_id:uuid,fee_rule_id:uuid},['external_method_id']));add('post','/v1/me/topups','Create wallet top-up and original checkout','CommandResult','TopUpInput');add('get','/v1/me/plans/{id}/offers','Eligible activities and explicit compatible campaign IDs',collection('Offer'));add('patch','/v1/me/plans/{id}/renewal','Explicit renewal authorization; takeover requires current merchant plan','CommandResult',obj({auto_renew:bool,allow_commission:bool,version:integer,confirmed_changed_terms:bool,plan_id:uuid,quote_id:uuid},['auto_renew','allow_commission','version']));
add('get','/v1/me/requests/{id}','Own usage, exact billing components and source allocation','Request');add('post','/v1/me/requests/{id}/cancel','Request cancellation; does not promise upstream refund','CommandResult',obj({}));const result=add('get','/v1/me/requests/{id}/result','Read own private temporary result; expired result returns410',{});result.responses['200'].content={'application/json':{schema:{}},'application/octet-stream':{schema:{type:'string',contentEncoding:'binary'}}};add('post','/v1/me/playground','Execute through own private playground key and shared credits','PlaygroundResult','PlaygroundInput');
add('get','/v1/me/notifications/unread-count','Count unread notifications addressed to the signed-in customer','UnreadCount');add('get','/v1/operators/{operatorId}/notifications/unread-count','Count unread notifications addressed to the signed-in staff member','UnreadCount');
const notificationDetails=add('get','/v1/operators/{operatorId}/notifications/notices','Cursor-paginated personal notices or authorized event recipients','ResourcePage');notificationDetails.parameters.push(query('group','personal, or a permitted notification event key'),query('cursor','Opaque detail cursor'),query('q','Search within this authorized group'),query('status','Exact delivery state'));
add('get','/v1/me/notifications/preferences','Read current notification preferences','NotificationPreferencesInput');add('post','/v1/me/notifications/preferences','Update voluntary marketing and usage email preferences','CommandResult','NotificationPreferencesInput');add('post','/v1/me/notifications/read-all','Mark every unread notice addressed to the signed-in customer as read','CommandResult',obj({}));add('post','/v1/me/notifications/{id}/read','Persist actual read evidence','CommandResult',obj({}));add('post','/v1/me/refunds','Rejected. Payment-balance cash-out is a withdrawal, not a source-linked refund','CommandResult',obj({amount_money:decimal,reason:str}));add('post','/v1/me/refunds/{id}/cancel','Cancel only a not yet executed original refund','CommandResult',obj({}));add('get','/v1/me/refunds/{id}/destinations','Own immutable refund leg destinations',collection('RefundDestination'));add('post','/v1/me/refunds/legs/{id}/destination','Customer explicitly proposes exact external refund destination','CommandResult','DestinationProposal');add('post','/v1/me/invoices','Invoice completed purchases once at their full paid amounts','CommandResult','InvoiceInput');add('post','/v1/me/invoices/{id}/replacement','Request replacement for the same orders on the voided original invoice','CommandResult',obj({details:schemas.InvoiceInput.properties.details},['details']));add('get','/v1/me/invoices/{id}/files','Read own application details, purchase orders and available private invoice files','InvoiceFiles');const file=add('get','/v1/me/invoices/{id}/files/{fileID}','Download authorized private invoice file',{});file.responses['200'].content={'application/octet-stream':{schema:{type:'string',contentEncoding:'binary'}}};add('post','/v1/me/withdrawals','Submit one commission withdrawal after the customer compares fees and provides the selected method recipient materials','CommandResult',obj({method_id:uuid,policy_id:uuid,amount_money:decimal,recipient:{type:'object',additionalProperties:{type:'string'},description:'Field names required by the selected payout method, such as payee name, account, bank or email.'}},['method_id','policy_id','amount_money','recipient']));add('post','/v1/me/withdrawals/{id}/cancel','Cancel eligible unexecuted withdrawal','CommandResult',obj({}));add('post','/v1/me/balance-withdrawals','Submit one payment-balance withdrawal. The fee is fixed at submission and paid by the customer; the payout is not split across top-up receipts','CommandResult',obj({method_id:uuid,policy_id:uuid,amount_money:decimal,recipient:{type:'object',additionalProperties:{type:'string'},description:'Field names required by the selected payout method.'}},['method_id','policy_id','amount_money','recipient']));add('post','/v1/me/balance-withdrawals/{id}/cancel','Cancel an unreviewed payment-balance withdrawal and release its hold','CommandResult',obj({}));add('post','/v1/me/account/email-changes','Begin verified email change','CommandResult',obj({current_password:str,new_email:{type:'string',format:'email'}}));add('get','/v1/me/account/closure','Read all real closure requirements',{});add('post','/v1/me/account/lifecycle/{action}','stop/resume/begin-close/complete-close with explicit customer evidence','CommandResult',obj({version:integer,evidence:str,waiver_evidence:str,waive_balance_money:{...decimal,description:'Optional exact payment wallet balance explicitly relinquished by its owner on complete-close. All other obligations must be closed.'}},['version']));add('post','/v1/me/account/waive-credits','Explicit ATG waiver only during closing; settlement-currency and commission balances retained','CommandResult',obj({waiver_evidence:str,version:integer}));
const workspace=add('get','/v1/me/{resource}/workspace','Typed operational workspace for topups, refunds, invoices, activities, withdrawals, account and credit-sources','ResourcePage');workspace.parameters.push(query('cursor','Opaque cursor from next_cursor'),query('q','Server-side search'));
const referenceKinds=['branding-brands','models','scope-subjects','customers','plans','orders','receipts','credentials','supplier-models','pricing-models','cost-suppliers','cost-models','merchants','supplier-methods','buyers','suppliers','purchase-quotes','campaigns'];
for(const base of['/v1/me','/v1/operators/{operatorId}'])add('get',base+'/references/{reference}','Permission-scoped name/ID selector; bounded server search and cursor','ReferencePage',null,'browser',{parameters:[...(base.includes('operatorId')?[{name:'operatorId',in:'path',required:true,schema:str}]:[]),{name:'reference',in:'path',required:true,schema:{type:'string',enum:referenceKinds}},query('q','Search by actual name or ID; at most 200 characters'),query('cursor','Cursor returned by next_cursor'),query('public_model_id','For supplier-models: offer models supporting at least one public operation'),query('provider_id','For cost-models: limit choices to this supplier'),...(base.includes('operatorId')?[{name:'buyer_id',in:'query',required:false,schema:uuid,description:'For models only: an exact direct child operator. Adds its actual authorization_version; without this query, authorization_version is omitted.'}]:[])]});
schemas.ModelReferenceDefaults=obj({public_model_id:str,identity:str,listing_version:integer,authorization_version:integer,routing_version:integer,route_strategy:{enum:['priority','lowest_latency','lowest_cost']},route_bindings:array(ref('RouteBindingInput'))},['public_model_id','identity','listing_version']);
schemas.ModelReferencePage=obj({items:array(obj({value:str,label:str,defaults:ref('ModelReferenceDefaults')})),next_cursor:str},['items']);
const modelReferences=add('get','/v1/operators/{operatorId}/references/models','Verified model identities and actual listing or direct-buyer authorization revisions','ModelReferencePage');
modelReferences.parameters.push(query('q','Search model name or public ID; at most 200 characters'),query('cursor','Cursor returned by next_cursor'),{name:'buyer_id',in:'query',required:false,schema:uuid,description:'An exact direct child operator. Returns authorization_version for this seller/buyer/model; missing records return version 0. Every choice always returns the current operator listing_version.'});
modelReferences.parameters.push({name:'purpose',in:'query',required:false,schema:{enum:['routing']},description:'Platform catalog managers only: include current route bindings, order, strategy and routing_version. This is the route policy CAS, not the public model version.'});
modelReferences['x-required-permissions-any']=['catalog.manage','sales.manage'];
const brandingReferences=add('get','/v1/operators/{operatorId}/references/branding-brands','Only brand operators this staff member may configure through this parent workspace','ReferencePage');
brandingReferences.description='Includes the current brand operator and permitted direct OEM children; channels are excluded. Each result value is the target operator ID, never a brand ID. Appearance authority does not grant customer or financial access.';
brandingReferences.parameters.push(query('q','Search actual target name or ID; at most 200 characters'),query('cursor','Cursor returned by next_cursor'));brandingReferences['x-required-permission']='organization.read';
schemas.OperatorContext=obj({subject:ref('Entity'),subject_operations:array(ref('Operation')),sections:array(obj({resource:str,title:str,page:ref('ResourcePage'),filters:obj({statuses:array(str),units:array(str)},[])})),next_actions:array(str)},['subject','sections','next_actions']);
for(const kind of ['customer','order']){const op=add('get',`/v1/operators/{operatorId}/${kind}-context/{id}`,'Exact permission-scoped '+kind+' workspace','OperatorContext');op.parameters.push(query('section','One returned section to paginate'+(kind==='customer'?' or filter. Required with cursor or filters.':'')),query('cursor','Previous next_cursor for this section with the same filters'));if(kind==='customer')op.parameters.push(query('q','Case-insensitive literal search of visible section fields, up to 200 characters'),query('status','Status value offered by the section'),query('unit','Asset unit offered by the assets section'),{...query('from','Inclusive lower bound on the section date'),schema:{type:'string',format:'date-time'}},{...query('to','Exclusive upper bound on the section date'),schema:{type:'string',format:'date-time'}});}
add('post','/v1/operators/{operatorId}/order-context/{id}/allocate','Allocate an existing verified receipt to this exact operator order','CommandResult',obj({receipt_id:uuid}));
const contractFile=add('get','/v1/operators/{operatorId}/contracts/{id}/content','Download a private cooperation contract visible to this supplier or associated buyer',{});contractFile.responses['200'].content={'application/octet-stream':{schema:{type:'string',contentEncoding:'binary'}}};
const resources=['receipt-valuations','gifts','overview','customers','plans','orders','payments','payment-methods','payment-quotes','pricing','marketing','commissions','ledger','operators','contracts','procurement','procurement-invoices','coverage','alerts','takeovers','exits','branding','domains','channel-themes','staff','restrictions','notifications','business-status','refunds','withdrawals','disputes','invoices','reconciliation','reports','exports','providers','catalog','routes','requests','jobs','mail','bifrost','supplier-costs'];
for(const resource of resources){const op=add('get',`/v1/operators/{operatorId}/${resource}`,'Read exact operator '+resource+' workspace','ResourcePage');op.description='Requires authenticated staff membership and the domain permission for this exact operator. Allowed operations, explicit typed fields and row actions are returned by the workspace; no generic writable table API exists.';op.parameters.push(query('cursor','Opaque page cursor'),query('q','Server-side search only when the response declares search_supported; matching occurs within the authorized scope before pagination.'),query('status','Exact business status only when the response declares status_supported; applied in the authorized scope before pagination.'),query('view','Only workspace-provided view values are accepted.'),...(resource==='procurement'?[query('section','orders, quotes or automatic; filters are applied before pagination.')]:[]),query('record','Exact refund case context when following an authorized business record.'))}
paths['/v1/operators/{operatorId}/payment-quotes'].get.description='Platform only. Configure exchange rate sources used for payment quotes; OEM and channel workspaces have no access.';
paths['/v1/operators/{operatorId}/mail'].get.description='Platform or OEM only. Channels automatically use their brand’s email service and have no mail workspace.';
const invitation=add('get','/v1/operators/{operatorId}/customer-invitation','Read this channel\'s permanent customer invite code and its own verified brand registration URL','ChannelInvitation');invitation['x-required-permission']='customers.read';
add('get','/v1/operators/{operatorId}/reports/financial','Repeatable-read exact financial report; 1–366 days','FinancialReport',null,'browser',{parameters:[{name:'operatorId',in:'path',required:true,schema:str},{name:'from',in:'query',required:true,schema:{type:'string',format:'date-time'}},{name:'to',in:'query',required:true,schema:{type:'string',format:'date-time'}}]});
const exportFile=add('get','/v1/operators/{operatorId}/exports/{id}/download','Download own private expiring CSV; financial files additionally require reports.view',{});exportFile.responses['200'].content={'text/csv':{schema:{type:'string'}}};exportFile['x-required-permission']='reports.export';
for (const [path, summary, parameters] of [
  ['/v1/me/orders/{id}/voucher/{kind}', 'Download an order or verified receipt voucher for the signed-in customer', [{name:'id',in:'path',required:true,schema:uuid},{name:'kind',in:'path',required:true,schema:{enum:['order','receipt']}}]],
  ['/v1/me/refunds/{id}/voucher', 'Download a completed refund voucher for the signed-in customer', [{name:'id',in:'path',required:true,schema:uuid}]],
  ['/v1/me/statement', 'Download the signed-in customer’s ledger statement for up to 366 days', [query('from','Inclusive RFC 3339 timestamp'),query('to','Exclusive RFC 3339 timestamp')]],
]) {
  const voucher = add('get', path, summary, {}, null, 'browser', {parameters});
  voucher.responses['200'].content = {'text/csv':{schema:{type:'string'}}};
}
const modelBodies={chat:{model:str,messages:array({type:'object',additionalProperties:true}),stream:bool,max_completion_tokens:integer,tools:array({type:'object',additionalProperties:true})},responses:{model:str,input:{},stream:bool,max_output_tokens:integer},embeddings:{model:str,input:{anyOf:[str,array(str)]},encoding_format:{enum:['float','base64']}},image:{model:str,prompt:str,n:{type:'integer',minimum:1,maximum:10},size:str,quality:str,response_format:str,stream:bool,partial_images:integer},speech:{model:str,input:str,voice:str,response_format:str,stream:bool},transcription:{model:str,file:{type:'string',contentEncoding:'binary'},language:str,response_format:str},video:{model:str,prompt:str,seconds:str,size:str}};
for(const[url,kind,required]of[['chat/completions','chat',['model','messages']],['responses','responses',['model','input']],['embeddings','embeddings',['model','input']],['images/generations','image',['model','prompt']],['audio/speech','speech',['model','input','voice']],['audio/transcriptions','transcription',['model','file']],['videos','video',['model','prompt']]]){const s=obj(modelBodies[kind],required);s.additionalProperties=true;const op=add('post','/v1/'+url,'Execute verified '+kind+' capability',{},s,'model');op.description='OpenAI-shaped call for a capability this public model offers. Customer parameters are forwarded unchanged. Brand-bound Bearer API Key. Optional stable Idempotency-Key up to150 characters: retrying identical content reuses one admitted request and charge; different content conflicts. No key means a new request. Billing requires verified usage. Stream errors after headers are emitted as SSE error events. Temporary result retention is bounded.';op.parameters=op.parameters.map(x=>x.$ref?.endsWith('IdempotencyKey')?{name:'Idempotency-Key',in:'header',required:false,schema:{type:'string',maxLength:150}}:x);op.responses['200'].headers={'X-TokenHub-Request-ID':{description:'Platform request ID used by the customer usage and billing records.',schema:uuid}};op.responses['200'].content={'application/json':{schema:{}},'text/event-stream':{schema:{type:'string'},example:'data: {"choices":[{"delta":{"content":"..."}}]}\n\ndata: [DONE]\n\n'}};if(['speech','video'].includes(kind))op.responses['200'].content['application/octet-stream']={schema:{type:'string',contentEncoding:'binary'}};if(kind==='video')op.responses['202']={description:'One durable asynchronous task, never fabricated completed usage',content:json(obj({id:str,object:{const:'video'},status:str,model:str,created_at:integer,delivered_at:{anyOf:[timestamp,{type:'null'}]}}))};if(['transcription','video'].includes(kind))op.requestBody.content['multipart/form-data']={schema:s}}
const chat = paths['/v1/chat/completions'].post;
chat.description += ' A public model that offers Responses instead of Chat Completions can accept supported Chat requests through an eligible Codex or Grok subscription pool. Restrictions on the upstream fields still apply.';
const messageContent = {type:'object',additionalProperties:true};
const messages = add('post','/v1/messages','Execute an Anthropic Messages request through a subscription pool',{},obj({
 model:str,messages:array(messageContent),max_tokens:{type:'integer',minimum:1},
 system:{anyOf:[str,array(messageContent)]},stream:bool,tools:array(messageContent),tool_choice:messageContent,
 thinking:messageContent,output_config:messageContent,output_format:messageContent,temperature:{type:'number'},top_p:{type:'number'},
},['model','messages','max_tokens']),'model');
messages.description = 'Available for public Responses models routed through an eligible Codex or Grok subscription pool. The response uses the Anthropic Messages format and billing uses verified upstream Responses usage. The request must fit the upstream connection parameters. Codex can accept max_tokens only when it covers the verified model output limit. Reuse an optional Idempotency-Key to replay the same request without another charge.';
messages.security = [{ModelKey:[]},{MessagesKey:[]}];
const modelIdempotency = {name:'Idempotency-Key',in:'header',required:false,schema:{type:'string',maxLength:150}};
messages.parameters = [modelIdempotency];
messages.responses['200'].headers = chat.responses['200'].headers;
messages.responses['200'].content = {
 ...json({}),
 'text/event-stream':{schema:str,example:'event: message_stop\ndata: {"type":"message_stop"}\n\n'},
};
const messagesError = obj({type:{const:'error'},error:obj({type:str,message:str}),request_id:str});
for (const [status,response] of Object.entries(messages.responses)) {
 if (status !== '200') response.content = json(messagesError);
}
const imageReference=obj({image:{type:'string',contentEncoding:'base64'},url:{type:'string',format:'uri'},image_url:{type:'string',format:'uri'}},[]);
imageReference.oneOf=['image','url','image_url'].map(key=>({required:[key]}));
const imageEdit={...obj({...modelBodies.image,image:imageReference,images:{type:'array',items:imageReference,minItems:1,maxItems:10},mask:{type:'string',contentEncoding:'base64'}},['model','prompt']),additionalProperties:true,oneOf:[{required:['image']},{required:['images']}]};
const imageUpload={...obj({...modelBodies.image,image:{type:'string',contentEncoding:'binary'},'image[]':array({type:'string',contentEncoding:'binary'}),mask:{type:'string',contentEncoding:'binary'}},['model','prompt']),additionalProperties:true,oneOf:[{required:['image']},{required:['image[]']}]};
const imageTask=obj({id:uuid,object:{const:'image.task'},status:str,model:str,created_at:integer,delivered_at:{anyOf:[timestamp,{type:'null'}]},expires_at:integer,result:obj({data:array(obj({b64_json:{type:'string',contentEncoding:'base64'}},['b64_json']))},['data']),reason:str},['id','object','status','model','created_at','delivered_at']);
const imageHeaders={'X-TokenHub-Request-ID':{description:'Platform request ID for usage records and result retrieval.',schema:uuid}};
for(const [endpoint, edit, async] of [['edits',true,false],['generations/async',false,true],['edits/async',true,true]]){
 const body=edit?imageEdit:{...obj(modelBodies.image,['model','prompt']),additionalProperties:true};
 const op=add('post','/v1/images/'+endpoint,async?'Accept one durable image task':'Edit an image',{},body,'model');
 op.parameters=op.parameters.map(p=>p.$ref?.endsWith('IdempotencyKey')?{name:'Idempotency-Key',in:'header',required:false,schema:{type:'string',maxLength:150}}:p);
 if(edit)op.requestBody.content['multipart/form-data']={schema:imageUpload};
 if(async){
  op.description='Accepts one image request without streaming. HTTP 202 records delivery and returns a task ID. The platform verifies usage and settles in the background. The same key and idempotency key can be used to retrieve the original task.';
  delete op.responses['200'];
  op.responses['202']={description:'Image task accepted',headers:{...imageHeaders,Location:{schema:str},'Retry-After':{schema:integer}},content:json(imageTask)};
 }else{
  op.responses['200'].headers=imageHeaders;
  op.responses['200'].content={...json({}),'text/event-stream':{schema:str}};
 }
}
const imageStatus=add('get','/v1/images/tasks/{id}','Retrieve temporary results for an image request',imageTask,null,'model');
imageStatus.description='Use the original API Key and the task ID or X-TokenHub-Request-ID. Completed results contain private base64 images for the retention window. Polling does not rerun or charge the request.';
imageStatus.responses['410']={description:'Temporary result expired',content:json('APIError')};
imageStatus.responses['503']={description:'Result temporarily unavailable inside the retention window',content:json('APIError')};
const responsesSocket = add('get','/v1/responses','Open a Responses WebSocket connection',{},null,'model');
responsesSocket.description = 'Send flat JSON response.create events. Each inference turn checks budget, reserves credits and settles verified Responses usage. Closing the socket does not cancel accepted turns. The latest completed response in a stream can supply context on this connection. Use the same event_id or idempotency_key and payload to retrieve a retained terminal response after reconnecting.';
delete responsesSocket.responses['200'];
responsesSocket.responses['101'] = {description:'WebSocket connection accepted'};
responsesSocket.responses['426'] = {description:'WebSocket upgrade required',content:json('APIError')};
responsesSocket['x-websocket-client-message'] = {
 ...obj({type:{const:'response.create'},model:str,input:{},previous_response_id:{type:['string','null']},
  idempotency_key:{...str,maxLength:120},event_id:{...str,maxLength:120},stream_id:{...str,maxLength:120},generate:bool},
 ['type','model']),additionalProperties:true,
};

const batchPath = '/v1/images/batches';
const batchStatus = obj({
 id:uuid,object:{const:'image.batch'},model:str,task_name:str,
 status:{enum:['queued','in_progress','completed','partial_failed','cancelled','failed']},
 created_at:integer,delivered_at:timestamp,item_count:integer,success_count:integer,fail_count:integer,cancelled_count:integer,
 estimate_atg:decimal,charged_atg:decimal,
});
const batchItem = obj({custom_id:str,request_id:uuid,status:str,reason:str,cancel_requested:bool,charged_atg:decimal});
const batchImageReference = obj({
 file_uri:{...str,format:'uri'},data:{...str,contentEncoding:'base64'},mime_type:str,
},[]);
batchImageReference.oneOf = [{required:['data']},{required:['file_uri']}];
const submitBatch = add('post',batchPath,'Accept an image batch and reserve all its items',{},obj({
 model:str,task_name:{...str,maxLength:80},size:str,quality:str,
 items:{...array(obj({
  custom_id:{...str,pattern:'^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,79}$'},prompt:str,
  output_count:{type:'integer',minimum:1,maximum:10,default:1},
  reference_images:{...array(batchImageReference),maxItems:10},
 },['custom_id','prompt'])),minItems:1,maxItems:20},
},['model','items']),'model');
submitBatch.parameters = [modelIdempotency];
submitBatch.description = 'Acceptance freezes each item price and atomically reserves the whole batch. HTTP 202 records delivery. Items with reference images use image editing. Individual requests settle with verified usage and results are private to the submitting API key.';
delete submitBatch.responses['200'];
submitBatch.responses['202'] = {description:'Batch accepted',headers:{Location:{schema:str},'Retry-After':{schema:integer}},content:json(batchStatus)};
const batches = add('get',batchPath,'List this API key’s image batches',obj({
 object:{const:'list'},data:array(batchStatus),has_more:bool,next_cursor:str,
}),null,'model');
batches.parameters.push(query('cursor','The next cursor from the previous page'),{name:'limit',in:'query',schema:{type:'integer',minimum:1,maximum:50,default:20}});
add('get',batchPath+'/{id}','Read the status, estimate and settled charge of an image batch',batchStatus,null,'model');
add('get',batchPath+'/{id}/items','List original requests and charges inside an image batch',obj({
 object:{const:'list'},data:array(batchItem),has_more:{const:false},
}),null,'model');
const cancelBatch = add('post',batchPath+'/{id}/cancel','Request cancellation of unfinished image items',batchStatus,null,'model');
cancelBatch.parameters = cancelBatch.parameters.filter(p=>!p.$ref);
const batchContent = add('get',batchPath+'/{id}/items/{custom_id}/content','Download one private image from a batch item',{},null,'model');
batchContent.parameters.push({name:'image_index',in:'query',schema:{...integer,default:0}});
const binaryFile = {schema:{type:'string',contentEncoding:'binary'}};
batchContent.responses['200'].content = Object.fromEntries(['image/png','image/jpeg','image/webp','image/gif'].map(type=>[type,binaryFile]));
batchContent.responses['410'] = {description:'The item result expired',content:json('APIError')};
const batchZip = add('get',batchPath+'/{id}/download','Download available batch images and a manifest as a ZIP',{},null,'model');
batchZip.responses['200'].content = {'application/zip':binaryFile};
batchZip.responses['410'] = {description:'The available results have expired',content:json('APIError')};

add('get','/v1/models','List public model IDs permitted for this brand-bound API key',obj({object:{const:'list'},data:array(obj({id:str,object:{const:'model'},owned_by:str}))}),null,'model');
add('get',batchPath+'/models','List callable public image models permitted for this API key',obj({object:{const:'list'},data:array(obj({id:str,object:{const:'model'},owned_by:str}))}),null,'model');
const videoSubmit = paths['/v1/videos'].post;
const videoGenerate = add('post','/v1/videos/generations','Submit a video generation task',{},videoSubmit.requestBody.content['application/json'].schema,'model');
videoGenerate.parameters = videoSubmit.parameters;
videoGenerate.requestBody = videoSubmit.requestBody;
videoGenerate.responses = videoSubmit.responses;
add('post','/v1/me/refunds/legs/{id}/responsible-destination','Customer explicitly authorizes the remaining refund through the current responsible operator','CommandResult','DestinationProposal');
add('get','/v1/videos/{id}','Query own original asynchronous task without resubmission',obj({id:str,object:{const:'video'},status:str,model:str,created_at:integer,delivered_at:{anyOf:[timestamp,{type:'null'}]},reason:str},['id','object','status','model','created_at']),null,'model');const download=add('get','/v1/videos/{id}/content','Download own completed private temporary video',{},null,'model');download.responses['200'].content={'application/octet-stream':{schema:{type:'string',contentEncoding:'binary'}}};add('delete','/v1/videos/{id}','Request cancellation of own original asynchronous task',obj({id:str,status:str,cancel_requested:bool}),null,'model');
const hook=add('post','/v1/payments/webhooks/{merchantId}','Persist and verify original signed provider callback before recoverable receipt handling',{},null,'webhook');hook.description='Provider-specific raw request body and headers. Stripe HMAC, Alipay RSA2, WeChat RSA/AES-GCM are verified by configured real driver. Duplicate stable external fact IDs are idempotent. This is not a browser command and does not accept browser CSRF or synthetic success flags.';hook.requestBody={required:true,content:{'application/json':{schema:{}},'application/x-www-form-urlencoded':{schema:{type:'object',additionalProperties:true}}}};
const kvoucherHook=add('post','/v1/payments/webhooks/kvoucher/{operatorId}','Verify a KVoucher merchant webhook and record the order hint',{},null,'webhook');kvoucherHook.description='One URL per operator. Signature is HMAC-SHA256 over the raw body using the hex SHA-256 of the webhook secret registered for that operator. Retrieving the reload code records the receipt; a Gkash return URL is not payment evidence. Each checkout freezes the actual MYR amount using the channel quote where available, otherwise the configured reference source. Verified receipts always retain the actual MYR amount. Missing, expired or mismatched quote relations require settlement confirmation and never imply full payment of the order.';kvoucherHook.requestBody={required:true,content:{'application/json':{schema:{}}}};
paths['/v1/operators/{operatorId}/takeovers'].get.responses['200'].content=json('Takeovers');
 paths['/v1/videos/{id}'].delete.parameters=paths['/v1/videos/{id}'].delete.parameters.filter(p=>!p.$ref);
 require(path.join(root,'backend/openapi/operator-inputs.cjs'))({root,schemas,paths,add,obj,str,bool,integer,uuid,decimal,priceDecimal,ref,array,json});
 const document={openapi:'3.1.0',info:{title:'TokenHub API',version:'0.1.0',description:'Actual new application contract. Direct API paths start /v1. Browser reverse proxy paths start /api/v1 and strip /api before Gin. Customer and staff use HttpOnly tokenhub_session; model API uses a brand-bound Bearer Key. All money/ATG amounts are exact decimal strings. MONEY denotes the independently configured global settlement currency; actual original-currency transfers and supplier reference, reported and statement costs remain separate. Per-payment currency quotes prefer the payment provider and fall back to the configured Frankfurter source. These quotes determine only the payable amount for that order; assets, supplier costs and cross-currency profit are never converted. Domain ownership, permission and business state are enforced server-side. This contract covers all model endpoints and core browser flows; operator commands use explicit request schemas derived from the actual typed workflows and reviewed technical DTOs.'},servers:[{url:'/',description:'API origin; browser proxy prefixes /api'}],paths,components:{securitySchemes:{BrowserSession:{type:'apiKey',in:'cookie',name:'tokenhub_session',description:'HttpOnly, SameSite=Lax, Secure in production. Staff and customer contexts have independent kinds and exact operator permissions.'},ModelKey:{type:'http',scheme:'bearer',bearerFormat:'TokenHub API Key'}},parameters:{IdempotencyKey:{name:'Idempotency-Key',in:'header',required:true,description:'A stable UUID generated for one browser action. Retry the same action with the same key; a new deliberate action uses a new UUID.',schema:uuid},CSRF:{name:'X-CSRF-Token',in:'header',required:true,description:'csrf_token from authenticated /v1/auth/me or login response; same-origin required for browser writes.',schema:str}},schemas}};
function validateRefs(value,location='$'){if(!value||typeof value!=='object')return;if(typeof value.$ref==='string'){if(!value.$ref.startsWith('#/'))throw new Error(`External reference at ${location}`);let current=document;for(const part of value.$ref.slice(2).split('/').map(v=>v.replace(/~1/g,'/').replace(/~0/g,'~'))){if(!current||!Object.prototype.hasOwnProperty.call(current,part))throw new Error(`Unresolved ${value.$ref} at ${location}`);current=current[part];}}for(const[k,v]of Object.entries(value))validateRefs(v,location+'.'+k)}
document.components.securitySchemes.MessagesKey = {type:'apiKey',in:'header',name:'X-Api-Key',description:'Brand-bound TokenHub API key for Anthropic Messages clients.'};
validateRefs(document);
for(const[url,methods]of Object.entries(paths)){
 if(/[?#]/.test(url))throw new Error('Query and fragment must not appear in OpenAPI path '+url);
 const names=[...url.matchAll(/\{([^}]+)\}/g)].map(match=>match[1]);
 for(const[method,operation]of Object.entries(methods)){
  const parameters=(operation.parameters||[]).filter(parameter=>!parameter.$ref),seen=new Set();
  for(const parameter of parameters){const key=parameter.in+':'+parameter.name;if(seen.has(key))throw new Error('Duplicate parameter '+key+' on '+method+' '+url);seen.add(key);if(parameter.in==='path'&&(!names.includes(parameter.name)||parameter.required!==true))throw new Error('Invalid path parameter '+parameter.name+' on '+method+' '+url)}
  for(const name of names)if(!parameters.some(parameter=>parameter.in==='path'&&parameter.name===name))throw new Error('Undocumented path parameter '+name+' on '+method+' '+url);
 }
}
const destination=path.join(root,'backend/openapi/tokenhub.json'),generated=JSON.stringify(document,null,2)+'\n';
if(process.argv.includes('--check')){if(!fs.existsSync(destination)||fs.readFileSync(destination,'utf8')!==generated){console.error('OpenAPI contract is missing or stale. Run node scripts/refactor-openapi.cjs and review the diff.');process.exitCode=1;}else console.log(`Verified ${Object.keys(paths).length} paths, ${Object.keys(schemas).length} schemas and all references`);}else{fs.writeFileSync(destination,generated);console.log(`Wrote ${Object.keys(paths).length} paths and ${Object.keys(schemas).length} schemas`);}
