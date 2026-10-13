package httpx

import (
	"encoding/csv"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCSVExportCompletesBeforeCommittingResponse(t *testing.T) {
	for _, fail := range []bool{false, true} {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		WriteCSVExport(c, "test.csv", []string{"id", "message"}, func(write func([]string) error) error {
			for i := 0; i < 300; i++ {
				if err := write([]string{"record", "value, \"quoted\"\n" + strings.Repeat("x", 40)}); err != nil {
					return err
				}
			}
			if fail {
				return errors.New("synthetic database error")
			}
			return nil
		})
		if fail {
			if response.Code != 500 || strings.Contains(response.Body.String(), "record") || response.Header().Get("Content-Disposition") != "" {
				t.Fatal("partial CSV advertised as successful")
			}
		} else {
			rows, err := csv.NewReader(response.Body).ReadAll()
			if response.Code != 200 || err != nil || len(rows) != 301 || rows[1][1] != "value, \"quoted\"\n"+strings.Repeat("x", 40) {
				t.Fatalf("invalid full CSV: %d %v", response.Code, err)
			}
		}
	}
}
