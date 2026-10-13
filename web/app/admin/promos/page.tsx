"use client";

import { networkUnavailable } from "@/lib/confirm";
import { defaultCopy, type CopyTranslator } from "@/lib/copy";
import { useTranslations } from "next-intl";
import { useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton, confirmFormSubmit } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Form } from "@/components/ui/form";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";

type Role = { id: string; channel_org_id: string; type: string; parent_id?: string; status: string };
type Promo = { id: string; code: string; channel_org_id: string; acquisition_role_id?: string; status: string };

function makeRoleSchema(tUI: CopyTranslator) { return z.object({
  channel_id: z.string().trim().min(1, tUI("consoleCopy.enterAChannelId")),
  role_type: z.string().trim().min(1, tUI("consoleCopy.enterARoleType")),
  parent_id: z.string().trim(),
}); }

function makePromoSchema(tUI: CopyTranslator) { return z.object({
  channel_id: z.string().trim().min(1, tUI("consoleCopy.enterAChannelId")),
  role_id: z.string().trim().min(1, tUI("consoleCopy.enterAPromotionRoleId")),
  code: z.string().trim(),
}); }

export default function AdminPromosPage() {
  const tUI = useTranslations();
  const confirmNetworkUnavailable = networkUnavailable(tUI);
  const roleSchema = makeRoleSchema(tUI);
  const promoSchema = makePromoSchema(tUI);

  const [message, setMessage] = useState(tUI("consoleCopy.promotionCodesFixAttributionAtRegistrationCreatingA"));
  const roleForm = useForm<z.infer<ReturnType<typeof makeRoleSchema>>>({
    resolver: zodResolver(roleSchema),
    defaultValues: { channel_id: "chn_reseller_b", role_type: "kol_l2", parent_id: "acr_b_kol1" },
  });
  const promoForm = useForm<z.infer<ReturnType<typeof makePromoSchema>>>({
    resolver: zodResolver(promoSchema),
    defaultValues: { channel_id: "chn_reseller_b", role_id: "acr_b_kol2", code: "" },
  });

  return (
    <AdminShell>
      <IfCan action="partners.write">
      <Form {...roleForm}>
        <form className="rounded-card border border-hairline bg-canvas-raised  p-6" onSubmit={(event) => event.preventDefault()}>
          <AdminH2 level={1} k="promoRoles" className="mb-4 text-lg font-semibold tracking-tight" />
          <p className="mb-3 text-sm text-ink-secondary">
            {tUI("consoleCopy.theHierarchyIsAgentKolL1KolL2")}</p>
          <div className="mb-3 flex flex-wrap items-end gap-2">
            <TextField control={roleForm.control} name="channel_id" label={tUI("consoleCopy.channelId238")} showLabel={false} className="w-48" />
            <TextField control={roleForm.control} name="role_type" label={tUI("consoleCopy.roleType")} showLabel={false} className="w-28" />
            <TextField control={roleForm.control} name="parent_id" label={tUI("consoleCopy.parentRole")} showLabel={false} className="w-48" />
            <ConfirmButton
              size="sm"
              title={tUI("consoleCopy.confirmPromotionRoleCreation")}
              description={tUI("consoleCopy.theHierarchyMustBeAgentKolL1Kol")}
              validate={() => roleForm.trigger()}
              onConfirm={confirmFormSubmit(roleForm.handleSubmit, async (values) => {
                try {
                const res = await fetch(`${apiBase}/admin/acquisition-roles`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({ channel_org_id: values.channel_id, type: values.role_type, parent_id: values.parent_id }),
                });
                const body = await res.json();
                if (!res.ok) {
                  setMessage(body.error?.message || tUI("consoleCopy.roleCreationFailed"));
                  return false;
                }
                if (body.item?.id) {
                  promoForm.setValue("role_id", body.item.id);
                  promoForm.setValue("channel_id", values.channel_id);
                }
                setMessage(tUI("consoleCopy.createdRole", { value0: String(body.item?.id), value1: String(body.item?.type) }));
                return true;
                } catch {
                  setMessage(confirmNetworkUnavailable);
                  return false;
                }
})}
            >
              {tUI("consoleCopy.createPromotionRole")}</ConfirmButton>
          </div>
          <p className="text-sm text-ink-secondary">{message}</p>
        </form>
      </Form>
      </IfCan>
      <IfCan action="promos.write">
      <Form {...promoForm}>
        <form className="rounded-card border border-hairline bg-canvas-raised  p-6" onSubmit={(event) => event.preventDefault()}>
          <AdminH2 k="promos" className="mb-4 text-lg font-semibold tracking-tight" />
          <p className="mb-3 text-sm text-ink-secondary">{tUI("consoleCopy.shareTheCodeOrCopyLoginPromoCode")}</p>
          <div className="mb-3 flex flex-wrap items-end gap-2">
            <TextField control={promoForm.control} name="role_id" label={tUI("consoleCopy.promotionRoleId")} showLabel={false} className="w-48" />
            <TextField control={promoForm.control} name="code" label={tUI("admin.promos")} placeholder="THB-SALE" showLabel={false} className="w-40" />
            <ConfirmButton
              size="sm"
              title={tUI("channelUi.confirmPromo")}
              description={tUI("consoleCopy.thisCodeFixesTheChannelAndRoleAt")}
              validate={() => promoForm.trigger()}
              onConfirm={confirmFormSubmit(promoForm.handleSubmit, async (values) => {
                try {
                const res = await fetch(`${apiBase}/admin/promotion-codes`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({
                    channel_org_id: values.channel_id,
                    acquisition_role_id: values.role_id,
                    code: values.code,
                  }),
                });
                const body = await res.json();
                setMessage(res.ok ? tUI("consoleCopy.createdPromotionCode", { value0: String(body.item?.code) }) : body.error?.message || tUI("consoleCopy.promotionCodeCreationFailed"));
                return res.ok;
                } catch {
                  setMessage(confirmNetworkUnavailable);
                  return false;
                }
})}
            >
              {tUI("channelUi.createPromo")}</ConfirmButton>
          </div>
        </form>
      </Form>
      </IfCan>
      <AdminListPanel<Role>
        path="/admin/acquisition-roles"
        title={tUI("consoleCopy.roles")}
        columns={[
          { accessorKey: "id", header: "ID" },
          { accessorKey: "type", header: tUI("user.ledColType") },
          { accessorKey: "channel_org_id", header: tUI("nav.channel") },
          { accessorKey: "status", header: tUI("compareUi.status") },
        ]}
      />
      <AdminListPanel<Promo>
        path="/admin/promotion-codes"
        title={tUI("consoleCopy.promotionCodes")}
        columns={[
          { accessorKey: "code", header: tUI("playgroundUi.code") },
          { accessorKey: "channel_org_id", header: tUI("nav.channel") },
          { accessorKey: "acquisition_role_id", header: tUI("staff.role") },
          { accessorKey: "status", header: tUI("compareUi.status") },
        ]}
      />
    </AdminShell>
  );
}
