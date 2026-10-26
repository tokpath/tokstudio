"use client";

import { FinancialReportScreen } from "./financial-report";
import { TakeoversScreen } from "./takeovers";
import { WorkspaceShell } from "./workspace-shell";
import { OperatorOverview, OperatorWorkflowScreen } from "./operator-workflows";
export { ResourceScreen } from "./operator-resources";

export function ManagementPage({ operatorID, resource, record }: { operatorID: string; resource: string; record?: string }) {
  return <WorkspaceShell operatorID={operatorID}>{resource === "overview" ? <OperatorOverview operatorID={operatorID} /> : resource === "reports" ? <FinancialReportScreen operatorID={operatorID} /> : resource === "takeovers" ? <TakeoversScreen operatorID={operatorID} /> : <OperatorWorkflowScreen operatorID={operatorID} resource={resource} record={record} />}</WorkspaceShell>;
}
