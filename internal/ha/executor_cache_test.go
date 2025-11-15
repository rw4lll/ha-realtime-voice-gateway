package ha

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rw4lll/ha-realtime-voice-gateway/internal/backend"
	"go.uber.org/zap"
)

func TestResultCaching(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"context":{"id":"test"}}]`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "test",
		Logger:  zap.NewNop(),
	})

	executor, err := NewExecutor(ExecutorConfig{
		Client:    client,
		AllowList: []string{"light.*", "homeassistant.get_state"},
		Logger:    zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}

	executor.cacheTTL = 200 * time.Millisecond
	ctx := context.Background()

	call1 := &backend.ToolCall{
		ID:        "call1",
		Name:      "light.turn_on",
		Arguments: map[string]any{"entity_id": "light.living_room"},
	}

	result1, err := executor.Execute(ctx, call1)
	if err != nil || result1 == nil {
		t.Fatalf("first call failed: %v", err)
	}

	call2 := &backend.ToolCall{
		ID:        "call2",
		Name:      "light.turn_on",
		Arguments: map[string]any{"entity_id": "light.living_room"},
	}

	result2, err := executor.Execute(ctx, call2)
	if err != nil || result2 == nil {
		t.Fatalf("second call failed: %v", err)
	}

	hits, misses, _ := executor.GetCacheStats()
	if hits != 1 {
		t.Errorf("expected 1 cache hit, got %d", hits)
	}
	if misses != 1 {
		t.Errorf("expected 1 cache miss, got %d", misses)
	}

	t.Logf("✓ Cache hit/miss tracking works correctly")
}
