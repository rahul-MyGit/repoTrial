package checkout

var prices = map[string]int{
	"notebook": 1299,
	"pen":      199,
	"sticker":  299,
}

func totalCents(items []Item) (int, error) {
	total := 0
	for _, item := range items {
		price, ok := prices[item.SKU]
		if !ok {
			return 0, ErrInvalidRequest
		}
		total += price * item.Quantity
	}
	return total, nil
}
