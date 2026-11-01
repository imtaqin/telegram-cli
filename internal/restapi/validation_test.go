package restapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONRequiresSingleValue(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		valid      bool
	}{
		{"one_value", `{"chat_id":123,"text":"demo"}`, true},
		{"trailing_whitespace", "{\"chat_id\":123,\"text\":\"demo\"}\n \t", true},
		{"trailing_garbage", `{"chat_id":123,"text":"demo"}garbage`, false},
		{"second_object", `{"chat_id":123,"text":"demo"}{"text":"other"}`, false},
		{"second_null", `{"chat_id":123,"text":"demo"}null`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/send", strings.NewReader(tt.body))
			recorder := httptest.NewRecorder()
			var input sendIn
			if valid := decodeJSON(recorder, request, &input); valid != tt.valid {
				t.Fatalf("accepted=%t, want %t", valid, tt.valid)
			}
			if !tt.valid && recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d, want 400", recorder.Code)
			}
		})
	}
}

func TestInvalidNumericQueries(t *testing.T) {
	for _, path := range []string{
		"/api/chats?limit=not-a-number",
		"/api/chats?limit=0",
		"/api/chats?limit=-1",
		"/api/chats?limit=4294967297",
		"/api/chats/123/history?limit=4294967297",
		"/api/chats/123/history?from_message_id=4294967297",
		"/api/chats/123/history?from_message_id=-1",
		"/api/chats/123/history?offset=4294967297",
		"/api/chats/123/history?offset=not-a-number",
		"/api/search/chats?q=demo&limit=4294967297",
		"/api/search/messages?q=demo&limit=4294967297",
	} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			recorder := httptest.NewRecorder()
			// Invalid arguments must be rejected before calling Telegram.
			testServer().ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d, want 400", recorder.Code)
			}
		})
	}
}
