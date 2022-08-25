package client

import "net/http"

//go:generate mockery --output ../mocks --name HTTPClient
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}
