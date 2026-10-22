package invoiceitems

import (
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxTotalCents = 1_000_000_000_000

var quantityPattern = regexp.MustCompile(`^[0-9]{1,7}(\.[0-9]{1,6})?$`)

type Item struct {
	Description    string `json:"description"`
	Quantity       string `json:"quantity"`
	UnitPriceCents int    `json:"unit_price_cents"`
	LineTotalCents int    `json:"line_total_cents"`
}

func Normalize(items []Item) ([]Item, int, error) {
	if len(items) < 1 || len(items) > 100 {
		return nil, 0, fmt.Errorf("items must contain between 1 and 100 rows")
	}
	out := make([]Item, len(items))
	total := 0
	for i, item := range items {
		item.Description = strings.TrimSpace(item.Description)
		if item.Description == "" || utf8.RuneCountInString(item.Description) > 500 {
			return nil, 0, fmt.Errorf("item %d description must contain 1 to 500 characters", i+1)
		}
		quantity := strings.TrimSpace(item.Quantity)
		if !quantityPattern.MatchString(quantity) {
			return nil, 0, fmt.Errorf("item %d quantity must be a decimal with up to 6 decimal places", i+1)
		}
		parts := strings.SplitN(quantity, ".", 2)
		fraction := ""
		if len(parts) == 2 {
			fraction = parts[1]
		}
		scaled, _ := strconv.ParseInt(parts[0]+fraction+strings.Repeat("0", 6-len(fraction)), 10, 64)
		if scaled <= 0 || scaled > 1_000_000_000_000 {
			return nil, 0, fmt.Errorf("item %d quantity must be greater than zero and at most 1000000", i+1)
		}
		if item.UnitPriceCents < 0 || item.UnitPriceCents > MaxTotalCents {
			return nil, 0, fmt.Errorf("item %d unit_price_cents is out of range", i+1)
		}
		product := new(big.Int).Mul(big.NewInt(scaled), big.NewInt(int64(item.UnitPriceCents)))
		product.Add(product, big.NewInt(500_000))
		product.Quo(product, big.NewInt(1_000_000))
		if !product.IsInt64() || product.Int64() > int64(MaxTotalCents-total) {
			return nil, 0, fmt.Errorf("invoice item total exceeds maximum amount")
		}
		item.Quantity = strconv.FormatInt(scaled/1_000_000, 10)
		if remainder := scaled % 1_000_000; remainder != 0 {
			item.Quantity += "." + strings.TrimRight(fmt.Sprintf("%06d", remainder), "0")
		}
		item.LineTotalCents = int(product.Int64())
		total += item.LineTotalCents
		out[i] = item
	}
	return out, total, nil
}

func Legacy(language string, total int) []Item {
	description := "Accommodation"
	if language == "sk" {
		description = "Ubytovanie"
	}
	return []Item{{Description: description, Quantity: "1", UnitPriceCents: total, LineTotalCents: total}}
}
