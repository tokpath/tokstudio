package portal

import (
	"encoding/base64"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"net/http"
	"strings"
)

// A browser preview is only the current result. Provider URLs are not embedded
// into the page; base64 images must decode to a supported raster image.
func playgroundOutput(out gin.H, operation modelruntime.Operation, data []byte) {
	if len(data) == 0 {
		return
	}
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return
	}
	texts := []string{}
	if text, ok := value["text"].(string); ok {
		texts = append(texts, text)
	}
	if choices, ok := value["choices"].([]any); ok {
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			message, _ := choice["message"].(map[string]any)
			if text, ok := message["content"].(string); ok {
				texts = append(texts, text)
			}
		}
	}
	if outputs, ok := value["output"].([]any); ok {
		for _, item := range outputs {
			output, _ := item.(map[string]any)
			contents, _ := output["content"].([]any)
			for _, item := range contents {
				content, _ := item.(map[string]any)
				if text, ok := content["text"].(string); ok {
					texts = append(texts, text)
				}
			}
		}
	}
	if len(texts) > 0 {
		out["text"] = strings.Join(texts, "\n")
	}
	if operation == modelruntime.Image {
		images := []string{}
		items, _ := value["data"].([]any)
		for _, item := range items {
			row, _ := item.(map[string]any)
			encoded, _ := row["b64_json"].(string)
			if encoded == "" || len(encoded) > 14<<20 {
				continue
			}
			decoded, e := base64.StdEncoding.DecodeString(encoded)
			if e != nil {
				continue
			}
			content := http.DetectContentType(decoded)
			if content != "image/png" && content != "image/jpeg" && content != "image/webp" {
				continue
			}
			images = append(images, "data:"+content+";base64,"+encoded)
			if len(images) >= 4 {
				break
			}
		}
		if len(images) > 0 {
			out["images"] = images
			out["media_kind"] = "image"
		}
	}
	if operation == modelruntime.Embedding {
		out["text"] = "向量结果已生成，可取得本次私有暂存结果用于应用。"
	}
}
