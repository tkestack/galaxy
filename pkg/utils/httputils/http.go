package httputils

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

type HttpClient struct {
	*http.Client
	retry uint32
}

func NewClient(timeout time.Duration, retry uint32) *HttpClient {
	return &HttpClient{
		&http.Client{
			Timeout:   timeout,
			Transport: &http.Transport{},
		},
		retry,
	}
}

func NewDefaultClient() *HttpClient {
	return NewClient(3*time.Second, 5)
}

func (c *HttpClient) Post(url string, bodyType string, body io.Reader) (resp *http.Response, err error) {
	var i uint32 = 0
	for ; i < c.retry; i++ {
		resp, err := c.Client.Post(url, bodyType, body)
		if err == nil {
			return resp, nil
		} else {
			if i == c.retry-1 {
				return nil, fmt.Errorf("retried %d times to send post request to %s: %v", c.retry, url, err)
			}
		}
	}
	return nil, nil
}
