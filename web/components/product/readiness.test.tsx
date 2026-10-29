// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { ReadinessAction } from "./readiness";

afterEach(cleanup);
it("tells a child to wait for platform setup and a channel to wait for its brand", () => {
 render(<><ReadinessAction operatorID="oem" workspace={{ operator: { kind: "oem" }, permissions: ["*"] }} requirement={{ code: "monetary_foundation", description: "base", ready: false, resource: "", action_hint: "等待平台负责人完成清算币种和 ATG 基础比例设置并生效。" }} /><ReadinessAction operatorID="channel" workspace={{ operator: { kind: "channel" }, permissions: ["*"] }} requirement={{ code: "verification_mail", description: "mail", ready: false, resource: "" }} /></>);
 expect(screen.getByText("等待平台负责人完成清算币种和 ATG 基础比例设置并生效。")).toBeTruthy();
 expect(screen.getByText("等待所属品牌方配置可用的邮件服务。")).toBeTruthy();
 expect(screen.queryByRole("link")).toBeNull();
});
it("takes an authorized OEM to its own mail settings and names local permission responsibility", () => {
 render(<><ReadinessAction operatorID="oem" workspace={{ operator: { kind: "oem" }, permissions: ["mail.read"] }} requirement={{ code: "verification_mail", description: "mail", ready: false, resource: "mail" }} /><ReadinessAction operatorID="oem" workspace={{ operator: { kind: "oem" }, permissions: [] }} requirement={{ code: "package", description: "plan", ready: false, resource: "plans" }} /></>);
 expect(screen.getByRole("link").getAttribute("href")).toBe("/manage/oem/mail");
 expect(screen.getByText("请联系本经营方负责人授权或办理。")).toBeTruthy();
});
it("identifies platform-only configuration without presenting an unusable link", () => {
 render(<ReadinessAction operatorID="oem" workspace={{ operator: { kind: "oem" }, permissions: ["*"] }} requirement={{ code: "upstream", description: "routes", ready: false, resource: "routes" }} />);
 expect(screen.getByText("等待平台完成配置。")).toBeTruthy();
 expect(screen.queryByRole("link")).toBeNull();
});
