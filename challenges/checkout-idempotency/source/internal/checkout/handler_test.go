package checkout

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testHandler() http.Handler {
	mux := http.NewServeMux()
	NewHandler(NewService(NewMemoryRepository())).Register(mux)
	return mux
}

func postCheckout(t *testing.T, handler http.Handler, userID, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader(body))
	request.Header.Set("X-User-ID", userID)
	request.Header.Set("Idempotency-Key", key)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeOrder(t *testing.T, response *httptest.ResponseRecorder) Order {
	t.Helper()
	var order Order
	if err := json.Unmarshal(response.Body.Bytes(), &order); err != nil {
		t.Fatalf("decode order: %v; response: %s", err, response.Body.String())
	}
	return order
}

func TestCheckoutCreatesOrder(t *testing.T) {
	handler := testHandler()
	response := postCheckout(t, handler, "alice", "checkout-1", `{"items":[{"sku":"notebook","quantity":2},{"sku":"pen","quantity":1}]}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	order := decodeOrder(t, response)
	if order.ID == "" || order.UserID != "alice" || order.TotalCents != 2797 {
		t.Fatalf("unexpected order: %+v", order)
	}
}

func TestCheckoutRetryReturnsOriginalOrder(t *testing.T) {
	handler := testHandler()
	body := `{"items":[{"sku":"notebook","quantity":1}]}`
	first := postCheckout(t, handler, "alice", "checkout-retry", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first request status = %d: %s", first.Code, first.Body.String())
	}
	retry := postCheckout(t, handler, "alice", "checkout-retry", body)
	if retry.Code != first.Code {
		t.Fatalf("retry status = %d, want original status %d; body: %s", retry.Code, first.Code, retry.Body.String())
	}
	if retry.Body.String() != first.Body.String() {
		t.Fatalf("retry response = %s, want original response = %s", retry.Body.String(), first.Body.String())
	}
}

func TestCheckoutRejectsInvalidItem(t *testing.T) {
	handler := testHandler()
	response := postCheckout(t, handler, "alice", "checkout-invalid", `{"items":[{"sku":"unknown","quantity":1}]}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}
