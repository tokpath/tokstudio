"use client";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { apiBase } from "@/lib/api";
import { formatSellPrice, type CatalogModel } from "@/lib/catalog";
import type { APIKeyItem } from "@/app/console/keys-panel";
import { maskAPIKey } from "@/app/console/keys-panel";
import { keyAllowsModel, keyState } from "@/lib/key-policy";
import { fetchKeyPages } from "@/lib/key-resources";
import { keyVerifyRequest } from "@/lib/key-example";
import { copyText, readResponseBody, errorMessageFromBody } from "@/lib/submit-result";
import { useViewer } from "@/components/rbac/viewer-context";
import { loginHref, pagePathWithSearch, safeNextPath } from "@/lib/login-next";
import { instructionsHref } from "@/lib/model-instructions-context";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
type Docs = {
    api_base_url?: string;
    model?: string;
    supported_endpoints?: string[];
    examples?: Record<string, unknown>;
};
type Tab = "overview" | "agent" | "protocol";
type Verification = {
    ok: boolean;
    message: string;
    code?: string;
    requestID?: string;
};
const agentSources = {
    cline: "https://docs.cline.bot/provider-config/openai-compatible",
    aider: "https://aider.chat/docs/llms/openai-compat.html",
    claude: "https://code.claude.com/docs/en/llm-gateway",
    codex: "https://developers.openai.com/codex/config-reference",
};
export function ModelUsagePanel({ model, models = [model], keyID = "", initialTab = "protocol", initialHref }: {
    model: CatalogModel;
    models?: CatalogModel[];
    keyID?: string;
    initialTab?: Tab;
    initialHref?: string;
}) {
    const t = useTranslations("keyUX");
    const tc = useTranslations("common");
    const viewer = useViewer();
    const scope = `${viewer.userId ?? ""}:${typeof window === "undefined" ? "" : window.location.host}`;
    const initialLocation = new URL(safeNextPath(initialHref) || instructionsHref("/app/docs", {key_id: keyID}, model.id, initialTab), "https://instructions.invalid");
    const [locationHref, setLocationHref] = useState(() => initialLocation.pathname + initialLocation.search);
    const [tab, setTab] = useState<Tab>(initialTab);
    const [docs, setDocs] = useState<Docs | null>(null);
    const [error, setError] = useState("");
    const [keys, setKeys] = useState<APIKeyItem[]>([]);
    const [keysState, setKeysState] = useState<"loading" | "ready" | "anonymous" | "error">("loading");
    const [keysError, setKeysError] = useState("");
    const [keysRevision, setKeysRevision] = useState(0);
    const [selectedKey, setSelectedKey] = useState(keyID);
    const [path, setPath] = useState(() => initialLocation.searchParams.get("protocol") || "");
    const [language, setLanguage] = useState(() => {
        const value = initialLocation.searchParams.get("language");
        return ["curl", "python", "node", "python_sdk", "node_sdk"].includes(value ?? "") ? value! : "curl";
    });
    const [agent, setAgent] = useState<"cline" | "aider">(() => initialLocation.searchParams.get("tool") === "aider" ? "aider" : "cline");
    useEffect(() => {
        setLocationHref(pagePathWithSearch(window.location.pathname, window.location.search));
        const params = new URLSearchParams(window.location.search);
        const tab = params.get("tab");
        if (tab === "agent" || tab === "protocol" || tab === "overview") setTab(tab);
        if (params.get("tool") === "aider") setAgent("aider");
        const value = params.get("language");
        if (["curl", "python", "node", "python_sdk", "node_sdk"].includes(value ?? "")) setLanguage(value!);
    }, [initialHref]);
    const [notice, setNotice] = useState("");
    const [fallback, setFallback] = useState("");
    const [revision, setRevision] = useState(0);
    const [verification, setVerification] = useState<Verification | null>(null);
    const [verifying, setVerifying] = useState(false);
    const verifySession = useRef(0);
    useEffect(() => {
        let cancelled = false;
        const controller = new AbortController();
        setDocs(null);
        setError("");
        setPath(new URLSearchParams(window.location.search).get("protocol") || "");
        const params = new URLSearchParams({ host: window.location.host, model: model.id });
        void fetch(`${apiBase}/v1/public/docs-context?${params}`, { credentials: "include", signal: controller.signal }).then(async (response) => {
            const body = await readResponseBody(response);
            if (cancelled)
                return;
            if (!response.ok) {
                setError(errorMessageFromBody(body, t("docsFailed")));
                return;
            }
            const result = body as Docs;
            if (result.model !== model.id || !result.api_base_url) {
                setError(t("docsFailed"));
                return;
            }
            setDocs(result);
            const requested = new URLSearchParams(window.location.search).get("protocol");
            setPath(result.supported_endpoints?.includes(requested ?? "") ? requested! : result.supported_endpoints?.[0] ?? "");
        }).catch(() => { if (!cancelled)
            setError(t("docsFailed")); });
        return () => { cancelled = true; controller.abort(); };
    }, [model.id, revision, scope, t]);
    useEffect(() => {
        let cancelled = false;
        const controller = new AbortController();
        setKeys([]);
        setKeysState("loading");
        setKeysError("");
        setFallback("");
        setNotice("");
        void fetchKeyPages<APIKeyItem>(`${apiBase}/v1/me/api-keys`, controller.signal).then(result => {
            if (cancelled)
                return;
            if (result.ok) {
                setKeys(result.items ?? []);
                setKeysState("ready");
            }
            else if (result.status === 401 && !viewer.signedIn) {
                setKeysState("anonymous");
            }
            else {
                setKeysState("error");
                setKeysError(result.message || t("keysFailed"));
            }
        });
        return () => { cancelled = true; controller.abort(); };
    }, [scope, keysRevision, viewer.signedIn, t]);
    useEffect(() => { setSelectedKey(keyID); }, [keyID, scope]);
    useEffect(() => {
        verifySession.current++;
        setVerification(null);
        setVerifying(false);
    }, [model.id, selectedKey, path, scope]);
    useEffect(() => () => { verifySession.current++; }, []);
    function context(nextTab: Tab, key = selectedKey) {
        setTab(nextTab);
        setSelectedKey(key);
        const url = new URL(window.location.href);
        url.searchParams.set("tab", nextTab);
        if (key)
            url.searchParams.set("key_id", key);
        else
            url.searchParams.delete("key_id");
        window.history.replaceState(null, "", url.pathname + url.search);
        setLocationHref(url.pathname + url.search);
    }
    function choose(name: "tool" | "protocol" | "language", value: string) {
        const url = new URL(window.location.href);
        url.searchParams.set(name, value);
        window.history.replaceState(null, "", url.pathname + url.search);
        setLocationHref(url.pathname + url.search);
        if (name === "tool") setAgent(value === "aider" ? "aider" : "cline");
        if (name === "protocol") setPath(value);
        if (name === "language") setLanguage(value);
    }
    async function copy(value: string) {
        if (await copyText(value)) {
            setNotice(t("copied"));
            setFallback("");
        }
        else {
            setNotice(t("copyFailed"));
            setFallback(value);
        }
    }
    const key = keys.find(item => item.id === selectedKey);
    const state = key ? keyState(key) : "active";
    const endpoints = docs?.supported_endpoints ?? [];
    const apiRoot = docs?.api_base_url?.replace(/\/$/, "");
    const apiURL = apiRoot ? `${apiRoot}/v1` : "";
    const examples = docs?.examples?.[path] as Record<string, string> | undefined;
    const languages = ["curl", "python_sdk", "node_sdk", "python", "node"].filter(value => !!examples?.[value]);
    const effectiveLanguage = docs === null || languages.includes(language) ? language : "curl";
    const snippet = examples?.[effectiveLanguage] ?? "";
    const sdkExample = effectiveLanguage.endsWith("_sdk");
    const languageLabel = (value: string) => value === "python_sdk" ? "Python · OpenAI SDK" : value === "node_sdk" ? "Node.js · OpenAI SDK" : value === "python" ? t("pythonHTTP") : value === "node" ? t("nodeHTTP") : "curl";
    const location = new URL(locationHref, "https://instructions.invalid");
    const params = new URLSearchParams(location.search);
    params.set("model", model.id);
    params.set("tab", tab);
    params.set("tool", agent);
    if (path) params.set("protocol", path);
    params.set("language", effectiveLanguage);
    if (selectedKey)
        params.set("key_id", selectedKey);
    else params.delete("key_id");
    const currentPath = location.pathname;
    const returnHref = `${currentPath}?${params}`;
    const protocolParams = new URLSearchParams(params);
    protocolParams.set("tab", "protocol");
    const protocolHref = `${currentPath}?${protocolParams}`;
    const createParams = new URLSearchParams(params);
    createParams.set("create", "1");
    createParams.set("return_to", locationHref);
    const createTarget = `/app/keys?${createParams}`;
    const createHref = viewer.signedIn ? createTarget : loginHref(createTarget);
    const editHref = `/app/keys?edit=${encodeURIComponent(selectedKey)}&${params}`;
    const walletHref = `/app/wallet?next=${encodeURIComponent(returnHref)}`;
    const requestsHref = `/app/usage?tab=requests&public_model_id=${encodeURIComponent(model.id)}${selectedKey ? `&api_key_id=${encodeURIComponent(selectedKey)}` : ""}`;
    const verifyRequest = keyVerifyRequest(model.id, path);
    const canVerify = !!(key?.key && apiRoot && verifyRequest && state === "active" && keyAllowsModel(key, model.id));
    async function verify() {
        if (!canVerify || !key?.key || !verifyRequest)
            return;
        const active = ++verifySession.current;
        setVerification(null);
        setVerifying(true);
        try {
            const response = await fetch(`${apiRoot}${verifyRequest.path}`, {
                method: "POST", headers: { Authorization: `Bearer ${key.key}`, "Content-Type": "application/json" },
                body: JSON.stringify(verifyRequest.body), signal: AbortSignal.timeout(60000),
            });
            const body = await readResponseBody(response) as {
                request_id?: string;
                error?: {
                    code?: string;
                    request_id?: string;
                };
            };
            if (active !== verifySession.current)
                return;
            setVerification({ ok: response.ok, code: body.error?.code, requestID: body.request_id ?? body.error?.request_id,
                message: response.ok ? t("verified") : errorMessageFromBody(body, t("verifyFailed")) });
        }
        catch {
            if (active === verifySession.current)
                setVerification({ ok: false, code: "request_outcome_unknown", message: t("verifyUnknown") });
        }
        finally {
            if (active === verifySession.current)
                setVerifying(false);
        }
    }
    const recovery = verification?.ok ? "requests" : verification?.code === "insufficient_balance" ? "wallet"
        : ["key_invalid", "key_unusable", "key_expired", "key_budget_exceeded", "model_not_allowed"].includes(verification?.code ?? "") ? "key"
            : ["rate_limited", "request_outcome_unknown"].includes(verification?.code ?? "") ? "requests" : "protocol";
    return <Card><div className="grid gap-4">
  {models.length > 1 ? <label className="grid gap-1 text-sm">{t("chooseModel")}<select aria-label={t("chooseModel")} className="h-10 rounded-control border border-hairline bg-canvas px-2" value={model.id} onChange={event => {
                const next = new URLSearchParams(params);
                next.set("model", event.target.value);
                window.location.assign(`${currentPath}?${next}`);
            }}>{models.map(item => <option key={item.id} value={item.id}>{item.display_name} · {item.id}</option>)}</select></label> : null}
  <div role="tablist" aria-label={t("instructions")} className="flex flex-wrap gap-2">{(["overview", "agent", "protocol"] as const).map(value => <Button key={value} role="tab" aria-selected={tab === value} variant={tab === value ? "default" : "outline"} onClick={() => context(value)}>{t(value)}</Button>)}</div>
  <label className="grid gap-1 text-sm">{t("chooseKey")}<select aria-label={t("chooseKey")} disabled={keysState === "loading" || keysState === "error"} className="h-10 rounded-control border border-hairline bg-canvas px-2" value={selectedKey} onChange={event => context(tab, event.target.value)}>
   <option value="">{t("noKey")}</option>{selectedKey && !key ? <option value={selectedKey}>{keysState === "ready" ? t("keyUnavailable") : t("keysLoading")}</option> : null}
   {keys.map(item => <option key={item.id} value={item.id}>{item.name} · {maskAPIKey(item.prefix)}</option>)}
  </select></label>
  {keysState === "error" ? <div role="alert" className="text-sm">{keysError} <Button variant="outline" onClick={() => setKeysRevision(v => v + 1)}>{t("reload")}</Button></div> : keysState === "loading" ? <p role="status" className="text-sm">{t("keysLoading")}</p> : null}
  {key && !keyAllowsModel(key, model.id) ? <p role="alert" className="text-sm text-warning">{t("notAllowed")} <Link className="underline" href={editHref}>{t("edit")}</Link></p> : null}
  {key && state !== "active" ? <p role="alert" className="text-sm text-warning">{t("unusable", { state: state === "disabled" ? tc("stDisabled") : t(state) })} <Link className="underline" href={editHref}>{t("edit")}</Link></p> : null}
  {error ? <div role="alert"><p>{error}</p><Button variant="outline" onClick={() => setRevision(value => value + 1)}>{t("reload")}</Button></div> : docs === null ? <p role="status">{t("loading")}</p> : null}
  {docs ? <p className="text-sm text-ink-secondary">{t("serviceStatus", { status: t(`service_${["available", "degraded", "unavailable"].includes(String((model as CatalogModel & {
            service_status?: string;
        }).service_status)) ? (model as CatalogModel & {
            service_status?: string;
        }).service_status : "unknown"}`) })}</p> : null}
  {docs && tab === "overview" ? <><p className="text-sm">{model.description || model.display_name}</p><p className="text-sm">{t("price")} · {formatSellPrice(model.sell_price)} USD</p></> : null}
  {docs && tab === "agent" ? <>
   <label className="grid gap-1 text-sm">{t("chooseAgent")}<select aria-label={t("chooseAgent")} className="h-10 rounded-control border border-hairline bg-canvas px-2" value={agent} onChange={event => choose("tool", event.target.value)}><option value="cline">Cline · OpenAI Compatible</option><option value="aider">Aider · OpenAI Compatible</option></select></label>
   {endpoints.includes("/v1/chat/completions") ? <>
    {agent === "cline" ? <>
    <h2 className="text-base font-medium">Cline · OpenAI Compatible</h2>
    <p className="text-xs text-ink-secondary">{t("agentVersion")}</p>
    <ol className="list-decimal space-y-2 pl-5 text-sm"><li>{t("clineStep1")}</li><li>{t("clineStep2")}</li><li>{t("clineStep3")}</li></ol>
    <dl className="space-y-3 text-sm"><div><dt>Base URL</dt><dd className="mt-1 break-all font-mono">{apiURL} <Button size="sm" variant="outline" onClick={() => void copy(apiURL)}>{tc("copy")}</Button></dd></div><div><dt>Model ID</dt><dd className="mt-1 break-all font-mono">{model.id} <Button size="sm" variant="outline" onClick={() => void copy(model.id)}>{tc("copy")}</Button></dd></div><div><dt>API Key</dt><dd>{key ? maskAPIKey(key.prefix) : t("pasteKey")}</dd></div></dl>
    <p className="text-sm text-ink-secondary">{t("clineSuccess")}</p>
    <p className="text-xs text-ink-secondary">{t("clineCapabilities", { context: model.context_length || t("notPublished"), output: model.max_completion_tokens || t("notPublished") })}</p>
    </> : <>
    <h2 className="text-base font-medium">Aider · OpenAI Compatible</h2>
    <ol className="list-decimal space-y-2 pl-5 text-sm"><li>{t("aiderStep1")} <code>python3 -m pip install aider-install</code><br/><code>aider-install</code></li><li>{t("aiderStep2")}</li><li>{t("aiderStep3")}</li></ol>
    {typeof (docs.examples?.["/v1/chat/completions"] as Record<string, unknown> | undefined)?.aider === "string" ? <><pre data-testid="model-agent-config" className="th-code overflow-x-auto whitespace-pre-wrap break-all p-3 text-xs">{(docs.examples!["/v1/chat/completions"] as Record<string, string>).aider}</pre><Button variant="outline" onClick={() => void copy((docs.examples!["/v1/chat/completions"] as Record<string, string>).aider)}>{tc("copy")}</Button></> : <p>{t("noProtocol")}</p>}
    <p className="text-xs text-ink-secondary">{t("aiderModelPrefix")}</p>
    <p className="text-sm text-ink-secondary">{t("clineSuccess")}</p>
    </>}
   </> : <p role="status">{t("agentUnsupported")}</p>}
   <a className="text-xs underline" href={agentSources[agent]} target="_blank" rel="noreferrer">{t("officialDocs")}</a>
   <details><summary className="cursor-pointer text-sm">Codex / Claude Code · {t("agentBoundaries")}</summary><div className="mt-2 space-y-2 text-sm text-ink-secondary"><p>{t("codexUnsupported")}</p><a className="underline" href={agentSources.codex} target="_blank" rel="noreferrer">Codex · {t("officialDocs")}</a><p>{t("claudeUnsupported")}</p><a className="underline" href={agentSources.claude} target="_blank" rel="noreferrer">Claude Code · {t("officialDocs")}</a></div></details>
  </> : null}
  {docs && tab === "protocol" ? endpoints.length ? <>
   <label className="grid gap-1 text-sm">{t("protocol")}<select aria-label={t("protocol")} className="h-10 rounded-control border border-hairline bg-canvas px-2" value={path} onChange={event => choose("protocol", event.target.value)}>{endpoints.map(endpoint => <option key={endpoint} value={endpoint}>{endpoint}</option>)}</select></label>
   <code className="break-all text-sm">POST {apiRoot}{path}</code><p className="text-sm text-ink-secondary">{t("auth")}</p>
   {snippet ? <><p className="text-sm">{t("runExample")}</p><pre data-testid="model-key-environment" className="th-code whitespace-pre-wrap break-all p-3 text-xs">{'export TOKENHUB_API_KEY=\'PASTE_YOUR_KEY\''}</pre><Button size="sm" variant="outline" onClick={() => void copy("export TOKENHUB_API_KEY='PASTE_YOUR_KEY'")}>{t("copyEnvironment")}</Button>{sdkExample ? <p className="text-xs text-ink-secondary">{t("sdkRetries")} <a className="underline" href={effectiveLanguage === "python_sdk" ? "https://developers.openai.com/api/reference/python" : "https://developers.openai.com/api/reference/typescript"} target="_blank" rel="noreferrer">{t("officialDocs")}</a></p> : null}</> : null}
   {snippet ? <><div className="flex flex-wrap gap-2">{languages.map(value => <Button key={value} size="sm" variant={effectiveLanguage === value ? "default" : "outline"} onClick={() => choose("language", value)}>{languageLabel(value)}</Button>)}</div><pre data-testid="model-protocol-example" className="th-code overflow-x-auto whitespace-pre-wrap break-all p-3 text-xs">{snippet}</pre><Button variant="outline" onClick={() => void copy(snippet)}>{tc("copy")}</Button></> : <p>{t("noProtocol")}</p>}
   <p className="text-sm text-ink-secondary">{path.includes("/images") || path.includes("/videos") ? t("mediaHint") : t("responseHint")}</p>
   {path !== "/v1/chat/completions" && !path.includes("/images") && !path.includes("/videos") ? <p className="text-xs text-ink-secondary">{t("partialProtocol")}</p> : null}
   <p className="text-xs text-ink-secondary">{(model.capabilities?.budget_estimate_supported ?? model.capabilities?.budget_control_supported ?? model.capabilities?.text_budget_control_supported) === true ? t("budgetSupported") : t("budgetUnsupported")}</p>
   <details><summary className="cursor-pointer text-sm">{t("errorsTitle")}</summary><ul className="mt-2 list-disc space-y-2 pl-5 text-sm text-ink-secondary">{["errorAuth", "errorModel", "errorBudget", "errorBalance", "errorEstimate", "errorRate", "errorUnknown", "errorEndpoint"].map(value => <li key={value}>{t(value)}</li>)}</ul></details>
  </> : <p>{t("noProtocol")}</p> : null}
  {docs && verifyRequest ? <div className="grid gap-2 border-t border-hairline pt-3"><p className="text-sm text-ink-secondary">{t("verifyCost")} <code>{path}</code></p><Button variant="outline" disabled={!canVerify || verifying} onClick={() => void verify()}>{verifying ? tc("submitting") : t("verify")}</Button></div> : null}
  {verification ? <div data-testid="model-verify-status" role={verification.ok ? "status" : "alert"} className="grid gap-2 text-sm"><p>{verification.code === "price_estimate_unavailable" ? `${verification.message} · ${t("errorEstimate")}` : verification.message}</p>{verification.requestID ? <code>{verification.requestID}</code> : null}<Link className="underline" onClick={event => { if (recovery === "protocol") {
        event.preventDefault();
        context("protocol");
    } }} href={recovery === "wallet" ? walletHref : recovery === "key" ? editHref : recovery === "protocol" ? protocolHref : verification?.requestID ? `/app/usage/requests/${encodeURIComponent(verification.requestID)}` : requestsHref}>{recovery === "wallet" ? t("wallet") : recovery === "key" ? t("edit") : recovery === "protocol" ? t("protocol") : t("requests")}</Link></div> : null}
  <div className="flex flex-wrap gap-2"><Button asChild variant="outline"><Link href={createHref}>{t("create")}</Link></Button>{selectedKey ? <Button asChild variant="outline"><Link href={editHref}>{t("edit")}</Link></Button> : null}<Button asChild variant="outline"><Link href={walletHref}>{t("wallet")}</Link></Button><Button asChild variant="outline"><Link href={requestsHref}>{t("requests")}</Link></Button>{key?.key ? <Button variant="outline" onClick={() => {
                void fetch(`${apiBase}/v1/me/api-keys/${encodeURIComponent(key.id)}/copy`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: "{}" }).then(response => { if (response.ok)
                    return copy(key.key!); throw new Error(); }).catch(() => setNotice(t("copyFailed")));
            }}>{t("copyKey")}</Button> : null}</div>
  {notice ? <p role="status" className="text-sm">{notice}</p> : null}{fallback ? <pre className="th-code whitespace-pre-wrap break-all p-3 text-xs">{fallback}</pre> : null}
 </div></Card>;
}
