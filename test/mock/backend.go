package mock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
)

type MockBackend struct {
	Server *httptest.Server
}

func NewMockBackend() *MockBackend {
	mb := &MockBackend{}
	mb.Server = httptest.NewServer(mb)
	return mb
}

func (mb *MockBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      "test-123",
		"model":   "test-model",
		"choices": []map[string]interface{}{
			{"message": map[string]string{"role": "assistant", "content": "Hello from mock"}},
		},
		"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 20},
	})
}

func (mb *MockBackend) Close() {
	mb.Server.Close()
}
