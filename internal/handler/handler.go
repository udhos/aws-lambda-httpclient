// Package handler implemetns lambda handler.
package handler

import (
	"context"
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
const Version = "0.0.6"

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
	familyDNS := env.String("FAMILY_DNS", "ip")
	familyConnect := env.String("FAMILY_CONNECT", "tcp")
	logHeaders := env.Bool("LOG_HEADERS", true)
	logBody := env.Bool("LOG_BODY", true)

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
			begin := time.Now()
			addrs, err := lookupHost(familyDNS, host)
			{
				elap := time.Since(begin)
				if err != nil {
					log.Printf("%s: DNS lookup ERROR latency=%v host=%s: %v", attempt, elap, host, err)
				} else {
					log.Printf("%s: DNS lookup SUCCESS latency=%v host=%s: %v", attempt, elap, host, addrs)
				}
			}

			if checkConnect {
				for j, addr := range addrs {
					addrStr := addr.String()
					addrPos := fmt.Sprintf("addr=%d/%d", j+1, len(addrs))
					portLabel := fmt.Sprintf("%s:%s(%s)", addrStr, port, portSource)

					if familyConnect == "tcp4" && !isIPv4(addr) {
						log.Printf("%s: %s: connect SKIP: %s is not IPv4", attempt, addrPos, portLabel)
						continue
					}

					if familyConnect == "tcp6" && !isIPv6(addr) {
						log.Printf("%s: %s: connect SKIP: %s is not IPv6", attempt, addrPos, portLabel)
						continue
					}

					beginConnect := time.Now()
					conn, err := net.DialTimeout(familyConnect,
						net.JoinHostPort(addrStr, port), timeout)
					elapConnect := time.Since(beginConnect)

					if err != nil {
						log.Printf("%s: %s: connect ERROR latency=%v: %s failed: %v", attempt, addrPos, elapConnect, portLabel, err)
					} else {
						log.Printf("%s: %s: connect SUCCESS latency=%v: %s", attempt, addrPos, elapConnect, portLabel)
						conn.Close()
					}
				}
			}
		}

		begin := time.Now()
		resp, err := request(client, method, u, virtualHost, rd, h)
		elap := time.Since(begin)

		log.Printf("%s: virtual_host='%s' %s %s: latency=%v status=%d remote=%s http=%s tls=%q body_size=%d error='%v'",
			attempt, virtualHost, method, u, elap, resp.status, resp.remote,
			resp.httpProto, resp.tlsVersion, len(resp.body), err)

		if logBody {
			log.Printf("%s: response body size=%d: %s", attempt, len(resp.body), resp.body)
		}

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

func isIPv4(addr net.IP) bool {
	return addr.To4() != nil
}

func isIPv6(addr net.IP) bool {
	return addr.To16() != nil && addr.To4() == nil
}

func lookupHost(network, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(context.Background(), network, host)
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
