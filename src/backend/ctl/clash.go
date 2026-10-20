package ctl

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/podkop-net/podkop-experimental/src/backend/api"
)

// clashCaller asks the Clash API of sing-box. Where the API listens and its key are what the
// backend that runs tells (api.Facts), asked once, at the first call: with no backend there
// is no API, and every call answers nothing (status 0).
type clashCaller struct {
	client api.Client
	once   sync.Once
	facts  api.Facts
}

func (c *clashCaller) call(method string, path string, body string, timeout time.Duration) (int, []byte) {
	c.once.Do(func() {
		if facts, _ := c.client.Facts(); facts != nil {
			c.facts = *facts
		}
	})
	if c.facts.ClashAPI == "" {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, "http://"+c.facts.ClashAPI+path, strings.NewReader(body))
	if err != nil {
		return 0, nil
	}
	if c.facts.ClashSecret != "" {
		request.Header.Set("Authorization", "Bearer "+c.facts.ClashSecret)
	}
	transport := &http.Transport{DisableKeepAlives: true, Proxy: nil}
	defer transport.CloseIdleConnections()
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		return 0, nil
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	return response.StatusCode, content
}
