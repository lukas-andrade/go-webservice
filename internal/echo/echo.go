package echo

import "encoding/json"

type Request struct {
	Path    string
	Headers map[string][]string
	Params  map[string][]string
	Body    []byte
}

type Response struct {
	Headers map[string][]string `json:"headers"`
	Params  map[string][]string `json:"params"`
	Body    any                 `json:"body"`
	Path    string              `json:"path"`
}

// Service turns an incoming request into its echo. It holds no state, so a
// single instance is safe to share across goroutines.
type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Echo(req Request) Response {
	return Response{
		Headers: nonNil(req.Headers),
		Params:  nonNil(req.Params),
		Body:    decodeBody(req.Body),
		Path:    req.Path,
	}
}

// A JSON body comes back as JSON so clients don't have to unescape it;
// anything else comes back as a plain string.
func decodeBody(body []byte) any {
	if len(body) > 0 && json.Valid(body) {
		return json.RawMessage(body)
	}
	return string(body)
}

func nonNil(m map[string][]string) map[string][]string {
	if m == nil {
		return map[string][]string{}
	}
	return m
}
