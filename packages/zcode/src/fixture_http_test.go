package zcode

import (
	"errors"
	"net/http"
)

const fixtureHTTPHeader = "X-ZCode-Fixture"

type fixtureHTTPClient struct {
	Doer
	origin string
	marker string
}

func (client fixtureHTTPClient) Do(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme+"://"+request.URL.Host != client.origin || request.URL.User != nil {
		return nil, errors.New("request escaped the ZCode fixture origin")
	}
	request = request.Clone(request.Context())
	request.Header.Set(fixtureHTTPHeader, client.marker)
	return client.Doer.Do(request)
}

func FixtureHTTPFactory(origin, marker string) HTTPFactory {
	return func(proxy string) (Doer, func(), error) {
		client, cleanup, err := defaultHTTP(proxy)
		if err != nil {
			return nil, nil, err
		}
		return fixtureHTTPClient{Doer: client, origin: origin, marker: marker}, cleanup, nil
	}
}
