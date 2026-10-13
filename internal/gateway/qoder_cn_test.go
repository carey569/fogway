package gateway

import (
	"net/http"
)

type qoderRoundTrip func(*http.Request) (*http.Response, error)

func (f qoderRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
