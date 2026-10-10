package app

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const curlBearerHeader = `-H "Authorization: Bearer ${TOKENHUB_API_KEY}"`

// Local brands use the configured public listener, including its port. Public
// brands keep their own HTTPS API domain rather than the control-plane origin.
func docsAPIBase(apiDomain, publicBase string) string {
	domain := strings.TrimSpace(apiDomain)
	// Brand API domains are host[:port], not URLs. Never substitute a platform
	// address when a brand is missing its own endpoint.
	if domain == "" || strings.ContainsAny(domain, "/@?#\\") {
		return ""
	}
	brandURL, err := url.Parse("https://" + domain)
	if err != nil || brandURL.Hostname() == "" || brandURL.User != nil || brandURL.Path != "" || brandURL.RawQuery != "" || brandURL.Fragment != "" {
		return ""
	}
	host := brandURL.Hostname()
	if net.ParseIP(host) == nil && strings.ContainsAny(host, ":[] \t\r\n") {
		return ""
	}
	if strings.HasSuffix(domain, ":") {
		return ""
	}
	if port := brandURL.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return ""
		}
	}
	local := host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "127.0.0.1" || host == "::1"
	configured, err := url.Parse(publicBase)
	if local && err == nil && configured.Host != "" && (configured.Scheme == "http" || configured.Scheme == "https") {
		brandURL.Scheme = configured.Scheme
		if brandURL.Port() == "" && configured.Port() != "" {
			brandURL.Host = net.JoinHostPort(host, configured.Port())
		}
	}
	return strings.TrimRight(brandURL.String(), "/")
}

func docsEndpointList(caps map[string]any) []string {
	if endpoints, ok := caps["supported_endpoints"].([]string); ok {
		return endpoints
	}
	return []string{}
}
func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
func docsExamples(apiDomain, model string) gin.H {
	return docsExamplesFor(apiDomain, model, []string{"/v1/chat/completions", "/v1/messages"})
}
func docsExamplesFor(apiDomain, model string, endpoints []string) gin.H {
	base := strings.TrimRight(apiDomain, "/")
	if base == "" {
		return gin.H{}
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "https://" + base
	}
	out := gin.H{}
	for _, path := range endpoints {
		var body any
		switch path {
		case "/v1/chat/completions":
			body = map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": "hi"}}, "max_tokens": 32}
		case "/v1/messages":
			body = map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": "hi"}}, "max_tokens": 32}
		case "/v1/responses":
			body = map[string]any{"model": model, "input": "hi", "max_output_tokens": 32}
		case "/v1/videos":
			body = map[string]any{"model": model, "prompt": "a river at dusk", "duration": 5, "resolution": "720p"}
		case "/v1/images/generations":
			body = map[string]any{"model": model, "prompt": "a river"}
		default:
			continue
		}
		raw, _ := json.Marshal(body)
		curl := "curl --fail-with-body --silent --show-error --request POST " + shellQuote(base+path) + " " + curlBearerHeader + ` -H "Content-Type: application/json" --data ` + shellQuote(string(raw))
		entry := gin.H{"curl": curl, "python": docsHTTPPython(base+path, string(raw)), "node": docsHTTPNode(base+path, string(raw))}
		if path == "/v1/chat/completions" || path == "/v1/responses" {
			entry["python_sdk"] = docsOpenAIPython(base+"/v1", model, path)
			entry["node_sdk"] = docsOpenAINode(base+"/v1", model, path)
		}
		if path == "/v1/chat/completions" {
			entry["aider"] = "export TOKENHUB_API_KEY='PASTE_YOUR_KEY'\nexport OPENAI_API_BASE=" + shellQuote(base+"/v1") + "\nexport OPENAI_API_KEY=\"${TOKENHUB_API_KEY:?Set TOKENHUB_API_KEY first}\"\naider --model " + shellQuote("openai/"+model) + "\n"
		}
		out[path] = entry
		if path == endpoints[0] {
			out["curl"] = curl
			out["python"] = entry["python"]
			out["node"] = entry["node"]
		}
		if path == "/v1/messages" {
			out["messages"] = curl
		}
	}
	return out
}
func docsPythonExample(base, model string) string {
	return docsExamples(base, model)["python"].(string)
}
func docsNodeExample(base, model string) string { return docsExamples(base, model)["node"].(string) }
func docsHTTPPython(endpoint, body string) string {
	urlJSON, _ := json.Marshal(endpoint)
	bodyJSON, _ := json.Marshal(body)
	return fmt.Sprintf(`# 保存为 chat.py；python3 chat.py（仅标准库）
import json
import os
import urllib.error
import urllib.request

body = json.loads(%s)
request = urllib.request.Request(
    %s,
    data=json.dumps(body).encode("utf-8"),
    headers={"Authorization": "Bearer " + os.environ["TOKENHUB_API_KEY"], "Content-Type": "application/json"},
    method="POST",
)
try:
    with urllib.request.urlopen(request, timeout=60) as response:
        print(json.load(response))
except urllib.error.HTTPError as error:
    print(error.read().decode("utf-8", errors="replace"))
    raise SystemExit(1)
`, string(bodyJSON), string(urlJSON))
}
func docsHTTPNode(endpoint, body string) string {
	urlJSON, _ := json.Marshal(endpoint)
	return fmt.Sprintf(`// 保存为 chat.mjs；node chat.mjs（需要原生 fetch）
const response = await fetch(%s, {
  method: "POST",
  headers: {Authorization: "Bearer " + process.env.TOKENHUB_API_KEY, "Content-Type": "application/json"},
  body: JSON.stringify(%s),
  signal: AbortSignal.timeout(60000),
});
if (!response.ok) throw new Error(await response.text());
console.log(await response.json());
`, string(urlJSON), body)
}

// SDK examples opt out of automatic retries: after a timeout the original
// request may already be charged, so users should inspect its request record.
func docsOpenAIPython(base, model, path string) string {
	baseJSON, _ := json.Marshal(base)
	modelJSON, _ := json.Marshal(model)
	call := fmt.Sprintf(`result = client.chat.completions.create(
    model=%s,
    messages=[{"role": "user", "content": "hi"}],
    max_tokens=32,
)
print(result.choices[0].message.content)`, modelJSON)
	if path == "/v1/responses" {
		call = fmt.Sprintf(`result = client.responses.create(model=%s, input="hi", max_output_tokens=32)
print(result.output_text)`, modelJSON)
	}
	return fmt.Sprintf(`# Install: python3 -m pip install openai
# Save as chat.py; run: python3 chat.py (Python 3.10+)
import os
from openai import OpenAI

client = OpenAI(
    api_key=os.environ["TOKENHUB_API_KEY"],
    base_url=%s,
    max_retries=0,
    timeout=60.0,
)
%s
print("request_id:", getattr(result, "request_id", None) or getattr(result, "_request_id", None))
`, baseJSON, call)
}

func docsOpenAINode(base, model, path string) string {
	baseJSON, _ := json.Marshal(base)
	modelJSON, _ := json.Marshal(model)
	call := fmt.Sprintf(`const result = await client.chat.completions.create({
  model: %s,
  messages: [{role: "user", content: "hi"}],
  max_tokens: 32,
});
console.log(result.choices[0].message.content);`, modelJSON)
	if path == "/v1/responses" {
		call = fmt.Sprintf(`const result = await client.responses.create({model: %s, input: "hi", max_output_tokens: 32});
console.log(result.output_text);`, modelJSON)
	}
	return fmt.Sprintf(`// Install: npm install openai
// Save as chat.mjs; run: node chat.mjs (Node.js 20+)
import OpenAI from "openai";

const client = new OpenAI({
  apiKey: process.env.TOKENHUB_API_KEY,
  baseURL: %s,
  maxRetries: 0,
  timeout: 60000,
});
%s
console.log("request_id:", result.request_id ?? result._request_id);
`, baseJSON, call)
}

func docsNotes() gin.H {
	return gin.H{
		"auth":       "先执行 export TOKENHUB_API_KEY='控制台复制的密钥'。curl 必须用双引号 \"Authorization: Bearer ${TOKENHUB_API_KEY}\"；Python 用 os.environ[\"TOKENHUB_API_KEY\"]，Node 用 process.env.TOKENHUB_API_KEY。示例不会写入完整 Key。",
		"errors":     "错误体为 {error:{code,message,request_id}}。常见 code：invalid_request、key_invalid、key_unusable、model_not_allowed、key_budget_exceeded、price_estimate_unavailable、insufficient_balance、rate_limited、request_outcome_unknown。超时或未知结果先查原请求，不自动重发。",
		"budget":     "请求开始前按当前品牌价格和计费维度预估是否准入，结束后按真实用量扣费；单次实际用量可能使 Key 累计消费超过上限或账户余额为负。超过 Key 上限后停止准入新请求；充值只补账户余额，不重置 Key 累计消费。缺失用量保持待核对，不按估算结算。",
		"rate_limit": "超过 API Key RPM/并发返回 429 rate_limited。",
		"webhook":    "媒体回调 POST /v1/media/callbacks，校验 X-Tokenhub-Signature；支付回调按适配器验签，均按 event_id 幂等。",
	}
}
