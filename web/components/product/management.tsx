"use client";

import { FinancialReportScreen } from "./financial-report";
import { DataExportsScreen } from "./data-exports";
import { TakeoversScreen } from "./takeovers";
import { FinanceReceiptsScreen, PaymentMethodsScreen } from "./finance-workspaces";
import { WorkspaceShell } from "./workspace-shell";
import { OperatorOverview, OperatorWorkflowScreen } from "./operator-workflows";
import { SubscriptionPoolsScreen } from "./subscription-pools";
import type { SupplierPoolSelection } from "./operator-resources";
export { ResourceScreen } from "./operator-resources";

export function ManagementPage({ operatorID, resource, record, supplierPool }: { operatorID: string; resource: string; record?: string; supplierPool?: SupplierPoolSelection }) {
  return <WorkspaceShell operatorID={operatorID}>{resource === "overview" ? <OperatorOverview operatorID={operatorID} /> : resource === "reports" ? <FinancialReportScreen operatorID={operatorID} /> : resource === "exports" ? <DataExportsScreen operatorID={operatorID} /> : resource === "takeovers" ? <TakeoversScreen operatorID={operatorID} /> : resource === "payments" ? <FinanceReceiptsScreen operatorID={operatorID} /> : resource === "payment-methods" ? <PaymentMethodsScreen operatorID={operatorID} /> : resource === "subscription-pools" ? <SubscriptionPoolsScreen operatorID={operatorID} /> : <OperatorWorkflowScreen operatorID={operatorID} resource={resource} record={record} supplierPool={supplierPool} />}</WorkspaceShell>;
}
