package gateway

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/carey569/fogway/internal/provider"
)

func TestRequestBodyProgressRenewsIdleDeadline(t *testing.T) {
	for _, encoding := range []string{"identity", "gzip", "zstd"} {
		t.Run(encoding, func(t *testing.T) {
			payload := bytes.Repeat([]byte("body"), 128)
			wire := encodedBody(t, payload, encoding)
			s := New()
			s.requestLimits.readTimeout = 200 * time.Millisecond
			gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, ok := s.readRequestBody(w, r, provider.Responses, 0)
				if !ok {
					return
				}
				if !bytes.Equal(body, payload) {
					t.Error("body changed")
				}
				io.WriteString(w, "ok")
			}))
			defer gw.Close()
			conn, err := net.Dial("tcp", strings.TrimPrefix(gw.URL, "http://"))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(3 * time.Second))
			fmt.Fprintf(conn, "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Length: %d\r\nContent-Encoding: %s\r\n\r\n", len(wire), encoding)
			// Six progressing reads take longer than one idle interval.
			step := (len(wire) + 5) / 6
			for offset := 0; offset < len(wire); offset += step {
				time.Sleep(50 * time.Millisecond)
				if _, err := conn.Write(wire[offset:min(offset+step, len(wire))]); err != nil {
					break
				}
			}
			res, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				t.Fatalf("progressing upload: %d", res.StatusCode)
			}
			body, err := io.ReadAll(res.Body)
			if err != nil || string(body) != "ok" {
				t.Fatalf("response: %q, %v", body, err)
			}
		})
	}
}

func TestBodyRejectionErrorShapes(t *testing.T) {
	for _, tc := range []struct {
		proto  provider.Protocol
		status int
		want   string
	}{
		{provider.Anthropic, 413, `"type":"request_too_large"`},
		{provider.Gemini, 413, `"status":"INVALID_ARGUMENT"`},
		{provider.Gemini, 408, `"status":"DEADLINE_EXCEEDED"`},
	} {
		w := httptest.NewRecorder()
		writeError(w, tc.proto, tc.status, "request body rejected")
		if !strings.Contains(w.Body.String(), tc.want) {
			t.Fatalf("%s/%d: %s", tc.proto, tc.status, w.Body.String())
		}
	}
}
