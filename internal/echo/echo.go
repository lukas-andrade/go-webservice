package echo

import (
	"encoding/json"
	"strings"
)

type Request struct {
	Path    string
	Headers map[string][]string
	Params  map[string][]string
	Body    []byte
}

type Response struct {
	Headers map[string]string   `json:"Headers"`
	Params  map[string][]string `json:"Params"`
	Body    any                 `json:"Body"`
	Path    string              `json:"Path"`
}

// Service turns an incoming request into its echo. It holds no state, so a
// single instance is safe to share across goroutines.
type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Echo(req Request) Response {
	params := req.Params
	if params == nil {
		params = map[string][]string{}
	}
	return Response{
		Headers: flattenHeaders(req.Headers),
		Params:  params,
		Body:    decodeBody(req.Body),
		Path:    req.Path,
	}
}

// Repeated headers are joined with ", ", which is how RFC 9110 says multiple
// values of the same field can be combined.
func flattenHeaders(h map[string][]string) map[string]string {
	flat := make(map[string]string, len(h))
	for name, values := range h {
		flat[name] = strings.Join(values, ", ")
	}
	return flat
}

// A JSON body comes back as JSON so clients don't have to unescape it;
// anything else comes back as a plain string.
func decodeBody(body []byte) any {
	if len(body) > 0 && json.Valid(body) {
		return json.RawMessage(body)
	}
	return string(body)
}
