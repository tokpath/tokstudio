package app

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const curlBearerHeader = `-H "Authorization: Bearer ${TOKENHUB_API_KEY}"`

func docsExamples(apiDomain, model string) gin.H {
	if model == "" {
		model = "tokenhub/echo-1"
	}
	base := "https://" + apiDomain
	chatBody := `{"model":"` + model + `","messages":[{"role":"user","content":"hi"}]}`
	return gin.H{
		"curl":     "curl -sS " + base + "/v1/chat/completions " + curlBearerHeader + ` -H "Content-Type: application/json" -d '` + chatBody + "'",
		"python":   "from openai import OpenAI\nclient = OpenAI(base_url='" + base + "/v1', api_key='...')\nprint(client.chat.completions.create(model='" + model + "', messages=[{'role':'user','content':'hi'}]))",
		"node":     "const OpenAI = require('openai')\nconst client = new OpenAI({ baseURL: '" + base + "/v1', apiKey: process.env.TOKENHUB_API_KEY })\nawait client.chat.completions.create({ model: '" + model + "', messages: [{ role: 'user', content: 'hi' }] })",
		"messages": "curl -sS " + base + "/v1/messages " + curlBearerHeader + ` -H "Content-Type: application/json" -d '{"model":"` + model + `","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}'`,
		"video":    "curl -sS " + base + "/v1/videos " + curlBearerHeader + ` -H "Content-Type: application/json" -d '{"model":"tokenhub/video-demo","prompt":"a river at dusk"}'`,
	}
}

func docsNotes() gin.H {
	return gin.H{
		"auth":       "先执行 export TOKENHUB_API_KEY='控制台复制的密钥'。curl 必须用双引号 \"Authorization: Bearer ${TOKENHUB_API_KEY}\"，单引号不会展开变量。示例不会写入完整 Key。",
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
