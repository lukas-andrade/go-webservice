package echo

import (
	"encoding/json"
	"testing"
)

func TestServiceEcho(t *testing.T) {
	tests := []struct {
		name     string
		req      Request
		wantJSON string
	}{
		{
			name: "json body is embedded as json",
			req: Request{
				Path:    "/orders/42",
				Headers: map[string][]string{"Content-Type": {"application/json"}},
				Params:  map[string][]string{"expand": {"items", "customer"}},
				Body:    []byte(`{"id": 42}`),
			},
			wantJSON: `{"headers":{"Content-Type":["application/json"]},"params":{"expand":["items","customer"]},"body":{"id":42},"path":"/orders/42"}`,
		},
		{
			name:     "plain text body is returned as a string",
			req:      Request{Path: "/", Body: []byte("hello there")},
			wantJSON: `{"headers":{},"params":{},"body":"hello there","path":"/"}`,
		},
		{
			name:     "empty body becomes an empty string",
			req:      Request{Path: "/empty"},
			wantJSON: `{"headers":{},"params":{},"body":"","path":"/empty"}`,
		},
		{
			name:     "broken json falls back to a string",
			req:      Request{Path: "/", Body: []byte(`{"id":`)},
			wantJSON: `{"headers":{},"params":{},"body":"{\"id\":","path":"/"}`,
		},
	}

	svc := NewService()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(svc.Echo(tt.req))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != tt.wantJSON {
				t.Errorf("\n got: %s\nwant: %s", got, tt.wantJSON)
			}
		})
	}
}
