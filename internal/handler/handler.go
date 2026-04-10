// Package handler implemetns lambda handler.
package handler

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"

	"github.com/udhos/boilerplate/envconfig"
)

// Version is lambda version.
const Version = "0.0.5"

// HandleRequest is lambda handler.
func HandleRequest() {

	const me = "aws-lambda-httpclient"

	log.Printf("%s %s", me, Version)

	env := envconfig.NewSimple(me)

	method := env.String("METHOD", "GET")
	proto := env.String("PROTO", "http")
	host := env.String("URL_HOST", "httpbin.org")
	virtualHost := env.String("VIRTUAL_HOST", "")
	route := env.String("ROUTE", "/get")
	body := env.String("BODY", "body")
	headers := env.String("HEADERS", `{"content-type":["application/json"],"who-am-i":["aws-lambda-httpclient"]}`)
	count := env.Int("COUNT", 3)
	interval := env.Duration("INTERVAL", time.Second)
	timeout := env.Duration("TIMEOUT", time.Second)
	tlsInsecureSkipVerify := env.Bool("TLS_INSECURE_SKIP_VERIFY", false)
	checkDNS := env.Bool("CHECK_DNS", true)
	checkConnect := env.Bool("CHECK_CONNECT", true)
	logHeaders := env.Bool("LOG_HEADERS", true)

	port, portSource := getPort(proto, host)

	client := newClient(timeout, tlsInsecureSkipVerify)

	var h http.Header
	if err := json.Unmarshal([]byte(headers), &h); err != nil {
		log.Printf("ERROR: headers json: %s: %v", headers, err)
	}

	u := fmt.Sprintf("%s://%s%s", proto, host, route)

	rd := strings.NewReader(body)

	for i := range count {

		attempt := fmt.Sprintf("attempt=%d/%d", i+1, count)

		if checkDNS {
			addrs, err := net.LookupHost(host)
			if err != nil {
				log.Printf("%s: DNS lookup ERROR host=%s: %v", attempt, host, err)
			} else {
				log.Printf("%s: DNS lookup SUCCESS host=%s: %v", attempt, host, addrs)
			}

			if checkConnect {
				for j, addr := range addrs {
					addrPos := fmt.Sprintf("addr=%d/%d", j+1, len(addrs))
					portLabel := fmt.Sprintf("%s:%s(%s)", addr, port, portSource)
					conn, err := net.DialTimeout("tcp", net.JoinHostPort(addr, port), timeout)
					if err != nil {
						log.Printf("%s: %s: connect ERROR: %s failed: %v",
							attempt, addrPos, portLabel, err)
					} else {
						log.Printf("%s: %s: connect SUCCESS: %s",
							attempt, addrPos, portLabel)
						conn.Close()
					}
				}
			}
		}

		begin := time.Now()
		resp, err := request(client, method, u, virtualHost, rd, h)
		elap := time.Since(begin)

		log.Printf("%s: virtual_host='%s' %s %s: latency=%v status=%d remote=%s http=%s tls=%q response='%s' error='%v'",
			attempt, virtualHost, method, u, elap, resp.status, resp.remote,
			resp.httpProto, resp.tlsVersion, resp.body, err)

		if logHeaders {
			var list []string
			for k, v := range resp.responseHeaders {
				list = append(list, fmt.Sprintf("%s=%q", k, v))
			}
			log.Printf("%s: response headers: %s", attempt, strings.Join(list, " "))
		}

		time.Sleep(interval)
	}
}

func newClient(timeout time.Duration, tlsInsecureSkipVerify bool) *http.Client {

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 10 * time.Second,
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       10 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: tlsInsecureSkipVerify,
		},
	}

	client := http.DefaultClient
	client.Timeout = timeout
	client.Transport = transport

	return client
}

func getPort(proto, host string) (string, string) {
	// 1. If host contains port, use it.
	_, port, found := strings.Cut(host, ":")
	if found {
		return port, "host-port"
	}

	// 2. Lookup default port for proto.
	p, err := net.LookupPort("tcp", proto)
	if err == nil {
		return fmt.Sprintf("%d", p), "port-lookup"
	}

	// 3. Some builtin defaults.
	switch proto {
	case "https":
		return "443", "builtin-port"
	}

	// 4. If all else fails, default to 80.
	return "80", "builtin-port"
}

type response struct {
	body            string
	status          int
	remote          string
	httpProto       string
	tlsVersion      string
	responseHeaders http.Header
}

func request(client *http.Client, method, u, virtualHost string,
	reqBody io.Reader, h http.Header) (out response, err error) {

	var remote string

	trace := &httptrace.ClientTrace{
		GotConn: func(connInfo httptrace.GotConnInfo) {
			remote = connInfo.Conn.RemoteAddr().String()
		},
	}

	req, errReq := http.NewRequest(method, u, reqBody)
	if errReq != nil {
		err = errReq
		return
	}

	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	req.Header = h

	req.Host = virtualHost

	resp, errResp := client.Do(req)
	if errResp != nil {
		err = errResp
		return
	}
	defer resp.Body.Close()

	respBody, errBody := io.ReadAll(resp.Body)
	if errBody != nil {
		err = errBody
		return
	}

	out = response{
		body:            string(respBody),
		status:          resp.StatusCode,
		remote:          remote,
		httpProto:       resp.Proto,
		responseHeaders: resp.Header,
	}

	if resp.TLS != nil {
		out.tlsVersion = tls.VersionName(resp.TLS.Version)
	}

	return
}
