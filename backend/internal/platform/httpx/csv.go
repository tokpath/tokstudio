package httpx

import (
	"encoding/csv"
	"os"

	"github.com/gin-gonic/gin"
)

// WriteCSVExport spools a streaming database export to a private temporary file.
// A failed query never produces a successful, silently incomplete CSV download.
func WriteCSVExport(c *gin.Context, filename string, headers []string, export func(func([]string) error) error) {
	file, err := os.CreateTemp("", "tokenhub-export-*.csv")
	if err != nil {
		Abort(c, 500, "internal_error", "导出失败，请重试", true)
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	w := csv.NewWriter(file)
	err = w.Write(headers)
	if err == nil {
		err = export(w.Write)
	}
	w.Flush()
	if err == nil {
		err = w.Error()
	}
	if err == nil {
		_, err = file.Seek(0, 0)
	}
	var size int64
	if err == nil {
		info, statErr := file.Stat()
		err = statErr
		if info != nil {
			size = info.Size()
		}
	}
	if err != nil {
		Abort(c, 500, "internal_error", "导出失败，请重试", true)
		return
	}
	c.DataFromReader(200, size, "text/csv; charset=utf-8", file, map[string]string{"Content-Disposition": `attachment; filename="` + filename + `"`})
}
