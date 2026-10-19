package checkout

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

var (
	ErrInvalidRequest = errors.New("invalid checkout request")
	ErrDuplicateKey   = errors.New("duplicate idempotency key")
)

type Item struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

type Request struct {
	Items []Item `json:"items"`
}

type Order struct {
	ID         string `json:"id"`
	UserID     string `json:"userId"`
	Items      []Item `json:"items"`
	TotalCents int    `json:"totalCents"`
}

func (r Request) validate() error {
	if len(r.Items) == 0 || len(r.Items) > 20 {
		return ErrInvalidRequest
	}
	for _, item := range r.Items {
		if strings.TrimSpace(item.SKU) == "" || item.Quantity < 1 || item.Quantity > 100 {
			return ErrInvalidRequest
		}
	}
	return nil
}

func fingerprint(r Request) string {
	data, _ := json.Marshal(r)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
