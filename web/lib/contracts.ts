import { z } from "zod";

// Amounts cross the API as decimal strings. No asset or price decision may use
// JavaScript Number. Integer counters/limits and timestamps have separate types.
export const Decimal = z.string().regex(/^(0|[1-9]\d*)(\.\d{1,24})?$/, "请输入非负十进制金额，最多 24 位小数");
export type DecimalString = z.infer<typeof Decimal>;
export const SignedDecimal = z.string().regex(/^-?(0|[1-9]\d*)(\.\d{1,24})?$/);
const ID = z.string().min(1);
export const ThemeSchema = z.object({
  primary_color: z.string().regex(/^#[\da-fA-F]{6}$/).nullable().optional(),
  accent_color: z.string().regex(/^#[\da-fA-F]{6}$/).nullable().optional(),
  background_color: z.string().regex(/^#[\da-fA-F]{6}$/).nullable().optional(),
  text_color: z.string().regex(/^#[\da-fA-F]{6}$/).nullable().optional(),
  support_url: z.string().nullable().optional(),
  logo_url: z.string().nullable().optional(),
});
export type Theme = z.infer<typeof ThemeSchema>;
export const MonetarySchema = z.object({ currency: z.string(), atg_per_unit: Decimal, status: z.enum(["unconfigured", "scheduled", "locked"]), effective_at: z.string().optional(), version: z.number().int(), configured_by: z.string().optional() });
export type Monetary = z.infer<typeof MonetarySchema>;
export const defaultMonetary: Monetary = { currency: "USD", atg_per_unit: "1", status: "unconfigured", version: 0 };
export const MonetarySnapshotSchema = z.object({ currency: z.string(), atg_per_unit: Decimal, version: z.number().int() });
export const BrandSchema = z.object({ id: ID, name: z.string(), api_base_url: z.string(), theme: ThemeSchema });
export type Brand = z.infer<typeof BrandSchema>;
export const OperatorSchema = z.object({ id: ID, name: z.string(), kind: z.enum(["platform", "oem", "channel"]), brand_id: ID, contact_email: z.string().optional(), legal_name: z.string().optional(), theme: ThemeSchema.optional() });
export type Operator = z.infer<typeof OperatorSchema>;
export const AvailabilitySchema = z.object({ capability: z.string(), enabled: z.boolean(), reason: z.string().optional(), next_action: z.string().optional() });
export const PaymentMethodSchema = z.object({ settlement_currency: z.string().optional(), settlement_supported: z.boolean().optional(), unsupported_reason: z.string().optional(), requires_settlement_confirmation: z.boolean().optional(), id: ID, name: z.string(), kind: z.enum(["online", "offline", "stablecoin"]), status: z.enum(["ready", "pending_configuration", "disabled"]), instructions: z.string().optional(), currency: z.string().optional(), network: z.string().optional() });
export const PublicContextSchema = z.object({
  monetary: MonetarySchema.default(defaultMonetary),
  brand: BrandSchema, operator: OperatorSchema, theme: ThemeSchema,
  invitation: z.object({ code: z.string(), status: z.enum(["valid", "invalid", "disabled", "wrong_brand"]), type: z.enum(["user", "channel"]).optional(), operator: OperatorSchema.optional(), inviter_display_name: z.string().optional(), message: z.string().optional() }).optional(),
  availability: z.array(AvailabilitySchema), payment_methods: z.array(PaymentMethodSchema),
  google_sign_in: z.boolean().optional(),
});
export type PublicContext = z.infer<typeof PublicContextSchema>;
const PersonSchema = z.object({ id: ID, email: z.string(), display_name: z.string() });
export const SessionSchema = z.discriminatedUnion("kind", [
  z.object({ kind: z.literal("anonymous") }),
  z.object({ kind: z.literal("customer"), customer: PersonSchema, brand: BrandSchema, operator: OperatorSchema, csrf_token: z.string().optional() }),
  z.object({ kind: z.literal("staff"), requires_password_change:z.boolean().optional(), staff: PersonSchema, workspaces: z.array(z.object({ operator: OperatorSchema, permissions: z.array(z.string()), can_manage_monetary: z.boolean().default(false) })), active_operator_id: z.string().optional(), csrf_token: z.string().optional() }),
]);
export const LoginResultSchema = z.union([SessionSchema, z.object({kind:z.literal("workspace_choice"),choices:z.array(z.enum(["customer","staff"]))})]);
export type Session = z.infer<typeof SessionSchema>;
export const APIErrorSchema = z.object({ error: z.object({ code: z.string(), message: z.string(), request_id: z.string(), retryable: z.boolean(), next_action: z.string().optional() }) });
export type APIErrorBody = z.infer<typeof APIErrorSchema>;
export const CommandResultSchema = z.object({ id: ID, status: z.string(), version: z.number().int().optional(), message: z.string().optional(),access_url:z.string().optional(),email:z.string().optional(),requires_password_change:z.boolean().optional(),order:z.lazy(()=>OrderSchema).optional() });
export type CommandResult = z.infer<typeof CommandResultSchema>;
export const RegistrationResultSchema = z.object({ registration_id: ID, verification_required: z.boolean(), message: z.string(), operator: OperatorSchema.optional() });
export const BalanceSchema = z.object({ available: Decimal, held: Decimal });
export const CreditsSchema = z.object({
  customer_atg: BalanceSchema.extend({ purchased_remaining: Decimal, gift_remaining: Decimal, expired: Decimal.optional() }),
  money_balance: BalanceSchema,
  commission_money: BalanceSchema.extend({ frozen: Decimal, recovery_due: Decimal,source_restricted:Decimal.optional() }),
  qualification: z.object({ confirmed_money: Decimal, pending_money: Decimal,pending_correction_money:Decimal.optional(), threshold_money: Decimal, qualified_at: z.string().nullable(), function_status: z.string() }),
});
export type Credits = z.infer<typeof CreditsSchema>;
export const PlanSchema = z.object({ id: ID, name: z.string(), price_money: Decimal, purchase_atg: Decimal, gift_atg: Decimal, validity: z.enum(["unlimited", "month", "quarter", "year"]), auto_renew_available: z.boolean(), status: z.string(), description: z.string().optional(), terms_summary:z.string().optional(), quote_id:ID.optional(), version: z.number().int() });
export type Plan = z.infer<typeof PlanSchema>;
const PublicModelPrice=z.object({component:z.string(),label:z.string().optional(),per:z.string().optional(),amount_atg:Decimal,unit:z.string()});
export const ModelSchema = z.object({ id: ID, name: z.string(), capabilities: z.array(z.string()), status: z.string(), prices: z.array(PublicModelPrice), upcoming_changes:z.array(z.object({id:z.string(),kind:z.string(),effective_at:z.string(),status:z.string(),prices:z.array(z.object({component:z.string(),amount_atg:Decimal,unit:z.string(),per:z.string()})).optional()})).nullable().optional(), unavailable_reason: z.string().optional() });
export type Model = z.infer<typeof ModelSchema>;
export const OrderSchema = z.object({ payment_state: z.string().optional(), payment_notice: z.string().optional(), monetary_snapshot: MonetarySnapshotSchema.optional(), method_change_allowed:z.boolean().optional(), cancel_allowed: z.boolean().optional(), id: ID, status: z.string(), kind: z.string(), operator_name: z.string(), amount_money: Decimal, paid_money: Decimal, delivered_at: z.string().nullable().optional(), created_at: z.string(), expires_at: z.string().nullable().optional(), plan_name: z.string().optional(), purchase_atg: Decimal.optional(), gift_atg: Decimal.optional(), payment_action: z.object({ payment_quote: z.object({ id: z.string(), source:z.string(), source_url:z.string(), rate:Decimal, rate_date:z.string(), quoted_at:z.string(), expires_at:z.string(), pay_money:z.object({currency:z.string(),amount:Decimal}),settlement_money:z.object({currency:z.string(),amount:Decimal}) }).nullable().optional(), settlement_money: z.object({ currency: z.string(), amount: Decimal }).optional(), kind: z.enum(["redirect", "stripe", "offline", "stablecoin", "form"]), url: z.string().optional(), client_secret: z.string().optional(), publishable_key: z.string().optional(), instructions: z.string().optional(), address: z.string().optional(), network: z.string().optional(), currency:z.string().optional(),token_contract:z.string().optional(),wallet_receipt_path:z.string().optional(), amount: z.string().optional(), fields: z.record(z.string()).optional() }).nullable().optional() });
export type Order = z.infer<typeof OrderSchema>;
export const QuoteSchema = z.object({ id: ID, plan_id: ID, plan_name: z.string(), operator_name: z.string(), amount_money: Decimal, purchase_atg: Decimal, plan_gift_atg: Decimal, operator_gift_atg: Decimal, kol_gift_atg: Decimal, total_atg: Decimal, validity: z.string(), terms_version: z.string(), terms_summary:z.string().optional(), expires_at: z.string(), warnings: z.array(z.string()) });
export type Quote = z.infer<typeof QuoteSchema>;
export const SubscriptionSchema = z.object({ id: ID, name: z.string(), status: z.string(), purchase_remaining_atg: Decimal, gift_remaining_atg: Decimal, effective_at: z.string(), expires_at: z.string().nullable(), auto_renew: z.boolean(), auto_renew_available: z.boolean().default(true), renewal_unavailable_reason:z.string().optional(), next_renewal_at:z.string().optional(), allow_commission: z.boolean(), requires_confirmation: z.boolean(), requires_current_plan:z.boolean().optional(), changed_terms: z.array(z.string()).optional(), terms_summary:z.string().optional(), renewal_quote:z.object({quote_id:ID,plan_id:ID,name:z.string(),price_money:Decimal,purchase_atg:Decimal,gift_atg:Decimal,validity:z.string(),terms_summary:z.string()}).optional(), version: z.number().int() });
export const KeySchema = z.object({ id: ID, name: z.string(), prefix: z.string(), status: z.string(), model_ids: z.array(z.string()).nullable(), budget: z.object({ limit_atg: Decimal.nullable(), period: z.enum(["lifetime", "month", "quarter", "year"]), timezone: z.string(), spent_atg: Decimal, held_atg: Decimal, overage_atg: Decimal, window_start: z.string().nullable(), window_end: z.string().nullable() }), rate_limit_rpm: z.number().int().nullable(), concurrency_limit: z.number().int().nullable(), version: z.number().int(), created_at: z.string() });
export const KeyRevealSchema = z.object({ secret: z.string().min(1) });
export type APIKey = z.infer<typeof KeySchema>;
export const KeySecretSchema = z.object({ key: KeySchema, secret: z.string().min(1) });
export const RequestSchema = z.object({ id: ID, model_id: ID, key_name: z.string(), status: z.string(), charged_atg: Decimal, corrected_atg:Decimal.optional(),corrected_charged_atg:Decimal.optional(),net_actual_atg:Decimal.optional(),net_charged_atg:Decimal.optional(), held_atg: Decimal, created_at: z.string(), result_expires_at: z.string().nullable().optional(), result_url: z.string().nullable().optional(), reason: z.string().optional(), actual_atg: Decimal.optional(), merchant_shortfall_atg: Decimal.optional(), components: z.array(z.object({ component: z.string(), label:z.string().optional(), quantity: z.string(), unit: z.string(), price_atg: Decimal.optional(), price_description: z.string().optional(), amount_atg: SignedDecimal })).optional(), allocations: z.array(z.object({ source_name: z.string(), kind: z.string(), amount_atg: Decimal, paid_basis_money: Decimal, sources: z.array(z.object({kind:z.string(),amount_atg:Decimal})).optional() })).optional() });
export const InviteCodeSchema = z.string().regex(/^[0-9A-Z]{6}$/);
export const ReferralSchema = z.object({ invite_code: InviteCodeSchema, invite_url: z.string(), direct_count: z.number().int(), indirect_count: z.number().int(), qualified: z.boolean(), function_status: z.string(), confirmed_money: Decimal, pending_money: Decimal, threshold_money: Decimal, direct_rate: Decimal, indirect_rate: Decimal });
export const ChannelInvitationSchema = z.object({ invite_code: InviteCodeSchema, invite_url: z.string(), brand_name: z.string(), enabled: z.boolean(), registration_open: z.boolean() });
export const CommissionSchema = z.object({ id: ID, source_id: ID, type: z.string(), hop: z.number().int(), status: z.string(), amount_money: SignedDecimal, basis_money: Decimal, created_at: z.string(), confirms_at: z.string().nullable(), description: z.string().optional() });
export const NotificationSchema = z.object({ id: ID, title: z.string(), body: z.string(), category: z.string(), necessary: z.boolean(), system_generated: z.boolean().optional(), read_at: z.string().nullable(), created_at: z.string(), action_url: z.string().optional(), delivery_status: z.string().optional() });
export const UnreadCountSchema = z.object({ count: z.number().int().nonnegative() });
export const SettlementSchema = z.object({ id: ID, status: z.string(), amount_money: Decimal, fee_money: Decimal.optional(), net_money: Decimal.optional(), reason: z.string().optional(), created_at: z.string(), paid_at: z.string().nullable().optional(), reference: z.string().optional(), download_url: z.string().optional() });
export const ActivitySchema = z.object({ id: ID, name: z.string(), plan_id: ID, status: z.string(), benefit_points: Decimal, starts_at: z.string(), ends_at: z.string(), per_customer_limit: z.number().int(), total_limit: z.number().int().nullable(), version: z.number().int() });

export function collection<T extends z.ZodTypeAny>(item: T) { return z.object({ items: z.array(item), next_cursor: z.string().nullable().optional() }); }
export type JSONValue = string | number | boolean | null | JSONValue[] | { [key: string]: JSONValue };
export const JSONValueSchema: z.ZodType<JSONValue> = z.lazy(() => z.union([z.string(), z.number(), z.boolean(), z.null(), z.array(JSONValueSchema), z.record(JSONValueSchema)]));
export const EntitySchema = z.object({ presentation: z.object({ title: z.string().optional(), title_kind:z.string().optional(), subtitle: z.string().optional(), labels: z.record(z.string()) }).optional(), id: ID, status: z.string().optional(), version: z.number().int().optional(), values: z.record(JSONValueSchema), allowed_actions: z.array(z.string()).optional() });
export type Entity = z.infer<typeof EntitySchema>;
export const FieldSchema = z.object({ name: z.string(), label: z.string(), kind: z.enum(["text", "email", "password", "decimal", "integer", "boolean", "select", "datetime", "textarea", "json", "file", "csv", "hidden"]), lookup_url:z.string().optional(),visible_when:z.object({field:z.string(),values:z.array(z.string()),not:z.boolean().optional(),fallback_fields:z.array(z.string()).optional()}).optional(), required: z.boolean().optional(), help: z.string().optional(), options: z.array(z.object({ value: z.string(), label: z.string() })).optional(), default_value: JSONValueSchema.optional() });
export type CommandField = z.infer<typeof FieldSchema>;
export const OperationSchema = z.object({ name: z.string(), label: z.string(), method: z.enum(["POST", "PATCH", "DELETE"]), path: z.string(), description: z.string().optional(), confirmation: z.string().optional(), fields: z.array(FieldSchema), permission: z.string().optional(), include_version: z.boolean().optional() });
export type Operation = z.infer<typeof OperationSchema>;
export const ResourcePageSchema = z.object({ views:z.array(z.object({value:z.string(),label:z.string()})).optional(),
  title: z.string(), description: z.string(), items: z.array(EntitySchema), search_supported: z.boolean().optional(), status_supported: z.boolean().optional(),
  columns: z.array(z.object({ key: z.string(), label: z.string(), help: z.string().optional(), kind: z.enum(["text", "money", "atg", "decimal", "datetime", "status", "boolean", "link", "details"]) })),
  operations: z.array(OperationSchema), row_operations: z.array(OperationSchema),
  availability: z.array(AvailabilitySchema).optional(), next_cursor: z.string().nullable().optional(),
  summary: z.array(z.object({ label: z.string(), value: z.string(), unit: z.string().optional(), detail: z.string().optional() })).optional(),
  payout_methods: z.array(z.object({ id: z.string(), name: z.string(), kind: z.string(), kind_label: z.string(), summary: z.string(), flat_fee_money: Decimal, fee_rate: Decimal, minimum_money: Decimal, fields: z.array(z.object({ name: z.string(), label: z.string(), help: z.string(), kind: z.enum(["text", "select"]), options: z.array(z.object({ value: z.string(), label: z.string() })).optional() })) })).optional(),
  currency_digits: z.number().int().nonnegative().optional(),
});
export type ResourcePage = z.infer<typeof ResourcePageSchema>;

export type LoginInput = { kind?: "customer" | "staff"; email: string; password: string };
export type RegistrationInput = { email: string; password: string; display_name: string; invite_code?: string; next?: string };
export type KeyInput = { name: string; model_ids: string[] | null; budget: { limit_atg: DecimalString | null; period: "lifetime" | "month" | "quarter" | "year" }; rate_limit_rpm: number | null; concurrency_limit: number | null };
export type OrderInput = { quote_id: string; terms_version: string; funding: { money_balance: DecimalString; commission_money: DecimalString; external_method_id?: string }; auto_renew: boolean; allow_commission: boolean };

export const OfferSchema = z.object({ id: ID, name: z.string(), kind: z.enum(["operator", "kol"]), description: z.string(), allow_combination: z.boolean(),compatible_campaign_ids:z.array(z.string()).optional() });
export const PlaygroundResultSchema = z.object({ request_id: ID, status: z.string(), text: z.string().optional(),images:z.array(z.string()).optional(),media_kind:z.enum(["image","audio","video"]).optional(), result_url: z.string().nullable().optional(), result_expires_at: z.string().nullable().optional(), charged_atg: Decimal, reason: z.string().optional() });

export const NotificationPreferencesSchema = z.object({ marketing: z.boolean(), usage_email: z.boolean() });

export const TakeoverSnapshotSchema = z.object({
  customers: z.record(z.string(), z.number().int()),
  assets: z.array(z.object({account_id: ID, customer_id: ID, purpose:z.string(), unit:z.string(), balance:SignedDecimal,held:Decimal,version:z.number().int()})),
  coverage:z.record(z.string(),z.unknown()), procurement:z.record(z.string(),z.unknown()),lots:z.array(z.record(z.string(),z.unknown())),
  service_atg:Decimal,service_money:Decimal,cash_money:Decimal,offset_atg:Decimal,offset_money:Decimal,residual_refund_money:Decimal,settlement_due_money:SignedDecimal,rounding_evidence:z.record(z.string(),z.string()),
});
export const TakeoverSchema=z.object({
 id:ID,channel_id:ID,brand_operator_id:ID,state:z.string(),method:z.string(),service_money_per_atg:Decimal,procurement_money_per_atg:Decimal,extra_money:SignedDecimal,contract_reference:z.string(),evidence:z.string(),requested_by:ID,approved_by:ID.nullable().optional(),
 snapshot:TakeoverSnapshotSchema.nullable(),snapshot_fingerprint:z.string(),service_money:Decimal,cash_money:Decimal,procurement_offset_atg:Decimal,procurement_offset_money:Decimal,remaining_procurement_refund_money:Decimal,settlement_due_money:SignedDecimal,settled_money:SignedDecimal,created_at:z.string(),completed_at:z.string().nullable().optional(),
});
export type Takeover=z.infer<typeof TakeoverSchema>;
export const TakeoversSchema=z.object({items:z.array(TakeoverSchema),channels:z.array(z.object({id:ID,name:z.string(),legal_name:z.string(),lifecycle:z.string()}))});
export const FinancialReportSchema=z.object({operator_id:z.string(),from:z.string(),to:z.string(),captured_at:z.string(),status:z.string(),settlement_currency:z.string(),
 original_currency_totals:z.array(z.object({currency:z.string(),income:SignedDecimal,expense:SignedDecimal,receipt_fees:SignedDecimal,net_receipts:SignedDecimal,pending_receipt_fees:z.number().int(),supplier_reported_cost:SignedDecimal,supplier_reference_cost:SignedDecimal,supplier_actual_cost:SignedDecimal})).default([]),
 sales:z.object({completed_sales:z.number().int(),sales_money:SignedDecimal,wallet_top_ups:z.number().int(),wallet_top_up_money:SignedDecimal,pending_deliveries:z.number().int()}),
 procurement:z.object({purchase_orders:z.number().int(),purchases_money:SignedDecimal,wholesale_orders:z.number().int(),wholesale_sales_money:SignedDecimal}),
 usage:z.object({completed_requests:z.number().int(),charged_atg:SignedDecimal,correction_atg:SignedDecimal,shortfall_atg:SignedDecimal}),
 revenue:z.object({paid_invocation_money:SignedDecimal,expired_unused_paid_money:SignedDecimal,wholesale_consumed_money:SignedDecimal,withdrawal_fees_retained_money:SignedDecimal,total_money:SignedDecimal}),
 costs:z.object({fulfillment_money:SignedDecimal,retail_gift_subset_money:SignedDecimal,commission_money:SignedDecimal,payment_fees_money:SignedDecimal,exception_loss_money:SignedDecimal,total_money:SignedDecimal,pending_fees:z.number().int(),pending_supplier_costs:z.number().int()}),
 cash:z.object({recorded_in_money:SignedDecimal,recorded_out_money:SignedDecimal,procurement_payments_money:SignedDecimal,commission_payouts_money:SignedDecimal,takeover_in_money:SignedDecimal,takeover_out_money:SignedDecimal,provider_receivable_money:SignedDecimal}),
 liabilities:z.object({wholesale_prepayment_money:SignedDecimal,customer_wallet_money:SignedDecimal,outstanding_grant_paid_money:SignedDecimal,outstanding_grant_atg:SignedDecimal,commission_money:SignedDecimal,unfulfilled_order_funds_money:SignedDecimal,supplier_payable_money:SignedDecimal}),
 payments:z.object({pending_receipt_facts:z.number().int(),awaiting_settlement:z.number().int().optional(),original_money:z.array(z.object({currency:z.string(),amount:Decimal,facts:z.number().int()}))}).optional(),
 refunds:z.object({pending_fees:z.number().int(),pending_executions:z.number().int(),outstanding_money:SignedDecimal,mismatch_count:z.number().int()}),
 takeover_flows:z.array(z.object({purpose:z.string(),kind:z.string(),unit:z.string(),increase:SignedDecimal,decrease:SignedDecimal,net:SignedDecimal})).nullable(),takeover_balances:z.array(z.object({purpose:z.string(),unit:z.string(),balance:SignedDecimal})).nullable(),notes:z.array(z.string()).nullable(),
});
