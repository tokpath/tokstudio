package modelruntime

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestRuntimeProxyDeniesProductionLoopbackAndRequiresAuthentication(t *testing.T) {
	p, e := newEgressProxy(false)
	if e != nil {
		t.Fatal(e)
	}
	defer p.close()
	connect := func(auth bool) *http.Response {
		t.Helper()
		c, e := net.DialTimeout("tcp", p.listener.Addr().String(), time.Second)
		if e != nil {
			t.Fatal(e)
		}
		defer c.Close()
		authorization := ""
		if auth {
			authorization = "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("runtime:"+p.token)) + "\r\n"
		}
		if _, e = fmt.Fprintf(c, "CONNECT 127.0.0.1:9 HTTP/1.1\r\nHost: 127.0.0.1:9\r\n%s\r\n", authorization); e != nil {
			t.Fatal(e)
		}
		response, e := http.ReadResponse(bufio.NewReader(c), &http.Request{Method: http.MethodConnect})
		if e != nil {
			t.Fatal(e)
		}
		return response
	}
	response := connect(false)
	response.Body.Close()
	if response.StatusCode != 407 {
		t.Fatal("unauthenticated SDK egress accepted")
	}
	response = connect(true)
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatalf("loopback dial allowed in production: %d", response.StatusCode)
	}
}
