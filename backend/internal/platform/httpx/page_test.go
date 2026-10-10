package httpx

import (
	"strconv"
	"testing"
)

func TestPaginateBoundsDirectCallerLimit(t *testing.T) {
	items := make([]int, 150)
	for i := range items {
		items[i] = i + 1
	}
	id := strconv.Itoa
	for _, limit := range []int{-1, 0, 101, int(^uint(0) >> 1)} {
		page, next := Paginate(items, limit, "", id)
		want := 100
		if limit <= 0 {
			want = 20
		}
		if len(page) != want || cap(page) > 100 || next != id(want) {
			t.Fatalf("limit %d: len=%d cap=%d next=%q", limit, len(page), cap(page), next)
		}
		page[0] = -1
		if items[0] != 1 {
			t.Fatal("page aliases original data")
		}
		second, _ := Paginate(items, limit, next, id)
		if len(second) == 0 || second[0] != want+1 {
			t.Fatalf("limit %d: cursor did not advance", limit)
		}
	}
}
