package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// DoRequest fires an arbitrary HTTP request (method/headers/body) at the app,
// then waits for the resulting Xdebug engine connection, applies breakpoints
// (done in adopt) and runs to the first break.
//
// Because the container has xdebug.start_with_request=yes, any request makes
// php-fpm dial the DBGp port. The request is sent in a goroutine — it blocks at
// the breakpoint and won't return until the session is resumed — while we wait
// on the listener in the foreground.
func (s *session) DoRequest(rawurl, method string, headers map[string]string, body string, timeout time.Duration) (string, error) {
	if rawurl == "" {
		return "", fmt.Errorf("url required")
	}
	if method == "" {
		method = "GET"
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	s.mu.Lock()
	ready := s.ready
	s.mu.Unlock()

	go func() {
		req, err := http.NewRequest(strings.ToUpper(method), rawurl, strings.NewReader(body))
		if err != nil {
			log.Printf("request build: %v", err)
			return
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		// No client timeout: the request legitimately blocks while paused at a breakpoint.
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("request error: %v", err)
			return
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		log.Printf("request completed: %s %s -> %s", method, rawurl, resp.Status)
	}()

	select {
	case <-ready:
		// engine connected & adopted (breakpoints applied); run to first break
		return s.step("run")
	case <-time.After(timeout):
		return "", fmt.Errorf("no Xdebug connection within %s — is Xdebug enabled in the container? (docker compose exec php-sub-api xdebug 1)", timeout)
	}
}
