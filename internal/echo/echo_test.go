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
			wantJSON: `{"Headers":{"Content-Type":"application/json"},"Params":{"expand":["items","customer"]},"Body":{"id":42},"Path":"/orders/42"}`,
		},
		{
			name: "repeated headers are joined",
			req: Request{
				Path:    "/",
				Headers: map[string][]string{"Accept": {"text/html", "application/json"}},
			},
			wantJSON: `{"Headers":{"Accept":"text/html, application/json"},"Params":{},"Body":"","Path":"/"}`,
		},
		{
			name:     "plain text body is returned as a string",
			req:      Request{Path: "/", Body: []byte("hello there")},
			wantJSON: `{"Headers":{},"Params":{},"Body":"hello there","Path":"/"}`,
		},
		{
			name:     "broken json falls back to a string",
			req:      Request{Path: "/", Body: []byte(`{"id":`)},
			wantJSON: `{"Headers":{},"Params":{},"Body":"{\"id\":","Path":"/"}`,
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
