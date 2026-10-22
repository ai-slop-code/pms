package invoiceitems

import (
	"strings"
	"testing"
)

func TestNormalizeExactFractionalAmounts(t *testing.T) {
	for _, tc := range []struct {
		quantity    string
		price, want int
	}{
		{"1.5", 101, 152},
		{"0.5", 1, 1},
		{"0.499999", 1, 0},
		{"0.000001", 999999999999, 1000000},
		{"1000000", 1000000, MaxTotalCents},
		{"2.25", 12399, 27898},
	} {
		t.Run(tc.quantity, func(t *testing.T) {
			items, total, err := Normalize([]Item{{Description: " Cleaning ", Quantity: tc.quantity, UnitPriceCents: tc.price, LineTotalCents: -1}})
			if err != nil || total != tc.want || items[0].LineTotalCents != tc.want || items[0].Description != "Cleaning" {
				t.Fatalf("normalize: items=%+v total=%d err=%v want=%d", items, total, err, tc.want)
			}
		})
	}
	items, total, err := Normalize([]Item{
		{Description: "First", Quantity: "00.500000", UnitPriceCents: 1},
		{Description: "Second", Quantity: "0.5", UnitPriceCents: 1},
	})
	if err != nil || total != 2 || items[0].Quantity != "0.5" {
		t.Fatalf("round each line before summing: items=%+v total=%d err=%v", items, total, err)
	}
}

func TestNormalizeRejectsInvalidItems(t *testing.T) {
	for _, quantity := range []string{"", "0", "-1", "NaN", "1e2", "1/2", "1.0000001", "1000000.000001", "12345678", "1,5"} {
		if _, _, err := Normalize([]Item{{Description: "Item", Quantity: quantity, UnitPriceCents: 1}}); err == nil {
			t.Fatalf("accepted invalid quantity %q", quantity)
		}
	}
	for _, items := range [][]Item{
		nil,
		make([]Item, 101),
		{{Description: " ", Quantity: "1"}},
		{{Description: strings.Repeat("é", 501), Quantity: "1"}},
		{{Description: "Item", Quantity: "1", UnitPriceCents: -1}},
		{{Description: "Item", Quantity: "1000000", UnitPriceCents: MaxTotalCents}},
		{{Description: "Item", Quantity: "1", UnitPriceCents: MaxTotalCents}, {Description: "Extra", Quantity: "1", UnitPriceCents: 1}},
	} {
		if _, _, err := Normalize(items); err == nil {
			t.Fatalf("accepted invalid items %+v", items)
		}
	}
}
