package app

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

const curlBearerHeader = `-H "Authorization: Bearer ${TOKENHUB_API_KEY}"`

// Local brands use the configured public listener, including its port. Public
// brands keep their own HTTPS API domain rather than the control-plane origin.
func docsAPIBase(apiDomain, publicBase string) string {
	brandURL, err := url.Parse("https://" + strings.TrimSpace(apiDomain))
	if err != nil || brandURL.Hostname() == "" {
		return strings.TrimRight(publicBase, "/")
	}
	host := brandURL.Hostname()
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
		curl := "curl --request POST " + shellQuote(base+path) + " " + curlBearerHeader + ` -H "Content-Type: application/json" --data ` + shellQuote(string(raw))
		entry := gin.H{"curl": curl, "python": docsHTTPPython(base+path, string(raw)), "node": docsHTTPNode(base+path, string(raw))}
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
import urllib.request

body = json.loads(%s)
request = urllib.request.Request(
    %s,
    data=json.dumps(body).encode("utf-8"),
    headers={"Authorization": "Bearer " + os.environ["TOKENHUB_API_KEY"], "Content-Type": "application/json"},
    method="POST",
)
with urllib.request.urlopen(request, timeout=60) as response:
    print(json.load(response))
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

func docsNotes() gin.H {
	return gin.H{
		"auth":       "先执行 export TOKENHUB_API_KEY='控制台复制的密钥'。curl 必须用双引号 \"Authorization: Bearer ${TOKENHUB_API_KEY}\"；Python 用 os.environ[\"TOKENHUB_API_KEY\"]，Node 用 process.env.TOKENHUB_API_KEY。示例不会写入完整 Key。",
		"errors":     "错误体为 {error:{code,message,request_id}}。常见 code：invalid_request、key_invalid、key_unusable、model_not_allowed、key_budget_exceeded、key_budget_unbounded、insufficient_balance、rate_limited、invalid_request、request_outcome_unknown。",
		"rate_limit": "超过 API Key RPM/并发返回 429 rate_limited。",
		"webhook":    "媒体回调 POST /v1/media/callbacks，校验 X-Tokenhub-Signature；支付回调按适配器验签，均按 event_id 幂等。",
	}
}
