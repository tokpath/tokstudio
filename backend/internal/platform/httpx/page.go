package httpx

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// Page 对齐 docs/06：limit 默认 20、最大 100，cursor 为上一页最后一条 id。
func Page(c *gin.Context, defaultLimit int) (limit int, cursor string) {
	if defaultLimit <= 0 {
		defaultLimit = 20
	}
	limit = defaultLimit
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}
	return limit, c.Query("cursor")
}

func NextCursor(items []string, limit int) string {
	if len(items) < limit || len(items) == 0 {
		return ""
	}
	return items[len(items)-1]
}

// Paginate 按 id cursor 切一页，避免复用原切片底层数组。
func Paginate[T any](items []T, limit int, cursor string, idFn func(T) string) (page []T, next string) {
	page = make([]T, 0, limit)
	seen := cursor == ""
	for _, item := range items {
		if !seen {
			if idFn(item) == cursor {
				seen = true
			}
			continue
		}
		page = append(page, item)
		if len(page) >= limit {
			break
		}
	}
	ids := make([]string, 0, len(page))
	for _, item := range page {
		ids = append(ids, idFn(item))
	}
	return page, NextCursor(ids, limit)
}

func OKPage[T any](c *gin.Context, items []T, defaultLimit int, idFn func(T) string) {
	limit, cursor := Page(c, defaultLimit)
	page, next := Paginate(items, limit, cursor, idFn)
	OK(c, gin.H{"items": page, "limit": limit, "next_cursor": next, "request_id": c.GetString(ContextRequestID)})
}

func WriteCSV[T any](c *gin.Context, filename string, headers []string, items []T, row func(T) []string) {
	var b strings.Builder
	b.WriteString(strings.Join(headers, ",") + "\n")
	for _, item := range items {
		b.WriteString(strings.Join(row(item), ",") + "\n")
	}
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.String(200, b.String())
}
