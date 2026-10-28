package operations

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
)

func TestExportCSVPreservesPrecisionAndAuthoredText(t *testing.T) {
	for _, input := range []string{"=SUM(A1:A2)", " +cmd", "\t@formula", "\n=1"} {
		if !strings.HasPrefix(safeCSVCell(input), "'") {
			t.Fatalf("unsafe spreadsheet text: %q", input)
		}
	}
	for _, input := range []string{"-0.000000000000000001", "123456789012345678.123456789012345678", "Third OEM", "1+1"} {
		if safeCSVCell(input) != input {
			t.Fatalf("changed ordinary value: %q", input)
		}
	}
	var raw bytes.Buffer
	w := csv.NewWriter(&raw)
	_ = w.WriteAll([][]string{{"客户", "应付 USD", "状态"}, {"已完成,\nOEM", "0.000000000000000001", "已完成"}})
	data, err := LocalizeExportCSV(raw.Bytes(), func(s string) string {
		return map[string]string{"客户": "Customer", "应付 USD": "Due USD", "状态": "Status", "已完成": "Completed"}[s]
	})
	if err != nil || !bytes.HasPrefix(data, []byte("\xef\xbb\xbf")) {
		t.Fatal("missing Excel UTF-8 signature", err)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff"))).ReadAll()
	if err != nil || rows[1][0] != "已完成,\nOEM" || rows[1][1] != "0.000000000000000001" || rows[1][2] != "Completed" || rows[0][0] != "Customer" {
		t.Fatal("CSV localization changed authored text or amount", rows, err)
	}
}
