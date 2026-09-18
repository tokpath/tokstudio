package app

import (
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

func docsExamples(apiDomain, model string) gin.H {
	if model == "" {
		model = "tokenhub/echo-1"
	}
	base := strings.TrimRight(apiDomain, "/")
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "https://" + base
	}
	chatBody := `{"model":"` + model + `","messages":[{"role":"user","content":"hi"}]}`
	return gin.H{
		"curl":     "curl -sS " + base + "/v1/chat/completions " + curlBearerHeader + ` -H "Content-Type: application/json" -d '` + chatBody + "'",
		"python":   docsPythonExample(base, model),
		"node":     docsNodeExample(base, model),
		"messages": "curl -sS " + base + "/v1/messages " + curlBearerHeader + ` -H "Content-Type: application/json" -d '{"model":"` + model + `","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}'`,
		"video":    "curl -sS " + base + "/v1/videos " + curlBearerHeader + ` -H "Content-Type: application/json" -d '{"model":"tokenhub/video-demo","prompt":"a river at dusk"}'`,
	}
}

func docsPythonExample(base, model string) string {
	return fmt.Sprintf(`# 保存为 chat.py
# pip install openai
# python chat.py
import os
from openai import OpenAI

client = OpenAI(base_url="%s/v1", api_key=os.environ["TOKENHUB_API_KEY"])
print(client.chat.completions.create(model="%s", messages=[{"role": "user", "content": "hi"}]))
`, base, model)
}

func docsNodeExample(base, model string) string {
	return fmt.Sprintf(`// 保存为 chat.cjs
// npm install openai
// node chat.cjs
const OpenAI = require("openai")

async function main() {
  const client = new OpenAI({
    baseURL: "%s/v1",
    apiKey: process.env.TOKENHUB_API_KEY,
  })
  try {
    const result = await client.chat.completions.create({
      model: "%s",
      messages: [{ role: "user", content: "hi" }],
    })
    console.log(result)
  } catch (err) {
    console.error(err)
    process.exit(1)
  }
}

main()
`, base, model)
}

func docsNotes() gin.H {
	return gin.H{
		"auth":       "先执行 export TOKENHUB_API_KEY='控制台复制的密钥'。curl 必须用双引号 \"Authorization: Bearer ${TOKENHUB_API_KEY}\"；Python 用 os.environ[\"TOKENHUB_API_KEY\"]，Node 用 process.env.TOKENHUB_API_KEY。示例不会写入完整 Key。",
		"errors":     "错误体为 {error:{code,message,request_id}}。常见 code：invalid_request、authentication_error、rate_limited、insufficient_balance。",
		"rate_limit": "超过 API Key RPM/并发返回 429 rate_limited。",
		"webhook":    "媒体回调 POST /v1/media/callbacks，校验 X-Tokenhub-Signature；支付回调按适配器验签，均按 event_id 幂等。",
	}
}

func firstModel(ids []string) string {
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			return id
		}
	}
	return "tokenhub/echo-1"
}
