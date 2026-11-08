type TransitionWorkspace = { transition?: { state: string } | null } | undefined;

// Mirrors the server's channel command boundary. Read permissions and record
// ownership remain independent; this only removes unavailable business actions.
export function canRunOperatorAction(workspace: TransitionWorkspace, path: string): boolean {
  if (!workspace?.transition) return true;
  const tail=path.split("/operators/")[1]?.split("/").slice(1).join("/").split("?")[0];
  if (!tail) return true;
  const parts=tail.split("/"), resource=parts[0], action=parts.length>1?parts.at(-1):"";
  if (["plans","pricing","marketing","commissions","gifts","coverage","alerts","branding","channel-themes","domains","payment-methods","payment-quotes","mail","contracts","exits","business-status","takeovers"].includes(resource)) return false;
  if (resource==="procurement") return workspace.transition.state!=="completed" && ["cancel","cancel_authorization","query_payment"].includes(action||"");
  if (["customers","customer-context","orders","order-context"].includes(resource)) return workspace.transition.state!=="completed";
  if (resource==="payments") return !!action;
  if (resource==="notifications") return action==="read" || action==="read-all";
  return true;
}
