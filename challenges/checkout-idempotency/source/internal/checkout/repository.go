package checkout

import (
	"context"
	"fmt"
	"sync"
)

type NewOrder struct {
	UserID      string
	Key         string
	Fingerprint string
	Items       []Item
	TotalCents  int
}

type SavedOrder struct {
	Order       Order
	Fingerprint string
}

type Repository interface {
	Create(context.Context, NewOrder) (SavedOrder, error)
	FindByKey(context.Context, string, string) (SavedOrder, bool, error)
}

type MemoryRepository struct {
	mu     sync.Mutex
	nextID int
	byKey  map[string]SavedOrder
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{byKey: make(map[string]SavedOrder)}
}

func keyFor(userID, key string) string {
	return userID + "\x00" + key
}

func cloneItems(items []Item) []Item {
	return append([]Item(nil), items...)
}

func cloneSaved(saved SavedOrder) SavedOrder {
	saved.Order.Items = cloneItems(saved.Order.Items)
	return saved
}

func (r *MemoryRepository) Create(ctx context.Context, input NewOrder) (SavedOrder, error) {
	if err := ctx.Err(); err != nil {
		return SavedOrder{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	key := keyFor(input.UserID, input.Key)
	if _, exists := r.byKey[key]; exists {
		return SavedOrder{}, ErrDuplicateKey
	}
	r.nextID++
	saved := SavedOrder{
		Order: Order{
			ID:         fmt.Sprintf("ord_%06d", r.nextID),
			UserID:     input.UserID,
			Items:      cloneItems(input.Items),
			TotalCents: input.TotalCents,
		},
		Fingerprint: input.Fingerprint,
	}
	r.byKey[key] = saved
	return cloneSaved(saved), nil
}

func (r *MemoryRepository) FindByKey(ctx context.Context, userID, idempotencyKey string) (SavedOrder, bool, error) {
	if err := ctx.Err(); err != nil {
		return SavedOrder{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	saved, ok := r.byKey[keyFor(userID, idempotencyKey)]
	return cloneSaved(saved), ok, nil
}
