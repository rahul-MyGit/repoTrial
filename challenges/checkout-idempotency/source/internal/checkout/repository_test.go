package checkout

import (
	"context"
	"errors"
	"testing"
)

func TestRepositoryKeepsUserKeysSeparate(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	input := NewOrder{
		UserID: "alice", Key: "same-key", Fingerprint: "first",
		Items: []Item{{SKU: "pen", Quantity: 1}}, TotalCents: 199,
	}
	first, err := repo.Create(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	input.UserID = "bob"
	second, err := repo.Create(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Order.ID == second.Order.ID {
		t.Fatalf("different users received same order ID %q", first.Order.ID)
	}
	if _, err := repo.Create(ctx, input); !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("duplicate create error = %v, want %v", err, ErrDuplicateKey)
	}
}

func TestRepositoryReturnsIsolatedCopies(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	saved, err := repo.Create(ctx, NewOrder{
		UserID: "alice", Key: "copy-key", Fingerprint: "first",
		Items: []Item{{SKU: "pen", Quantity: 1}}, TotalCents: 199,
	})
	if err != nil {
		t.Fatal(err)
	}
	saved.Order.Items[0].SKU = "changed"
	loaded, found, err := repo.FindByKey(ctx, "alice", "copy-key")
	if err != nil || !found {
		t.Fatalf("find: found=%v, err=%v", found, err)
	}
	if loaded.Order.Items[0].SKU != "pen" {
		t.Fatalf("stored order was mutated: %+v", loaded.Order)
	}
}
