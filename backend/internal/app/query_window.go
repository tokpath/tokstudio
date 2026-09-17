package app

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

var (
	errInvalidQueryTime  = errors.New("invalid query time")
	errInvalidQueryRange = errors.New("invalid query range")
)

// parseQueryWindow 解析 from/to。区间为起点包含、终点不包含。
// RFC3339 原样作为瞬间；仅日期时 from 为该 UTC 日 00:00，to 为次日 UTC 00:00。
func parseQueryWindow(fromRaw, toRaw string) (since, until time.Time, err error) {
	since, err = parseQueryBound(fromRaw, false)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	until, err = parseQueryBound(toRaw, true)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !since.IsZero() && !until.IsZero() && !since.Before(until) {
		return time.Time{}, time.Time{}, errInvalidQueryRange
	}
	return since, until, nil
}

func parseQueryBound(raw string, exclusiveDateEnd bool) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z"} {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts.UTC(), nil
		}
	}
	if ts, err := time.Parse("2006-01-02", raw); err == nil {
		if ts.Format("2006-01-02") != raw {
			return time.Time{}, errInvalidQueryTime
		}
		if exclusiveDateEnd {
			return ts.UTC().AddDate(0, 0, 1), nil
		}
		return ts.UTC(), nil
	}
	return time.Time{}, errInvalidQueryTime
}

func queryWindowOrAbort(c *gin.Context) (since, until time.Time, ok bool) {
	since, until, err := parseQueryWindow(c.Query("from"), c.Query("to"))
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "时间范围无效", false)
		return time.Time{}, time.Time{}, false
	}
	return since, until, true
}
