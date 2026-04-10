// Package handler implemetns lambda handler.
package handler

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"

	"github.com/udhos/boilerplate/envconfig"
)

// Version is lambda version.
const Version = "0.0.4"

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

	client := http.DefaultClient
	client.Timeout = timeout

	if tlsInsecureSkipVerify {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: tlsInsecureSkipVerify,
			},
		}
	}

	var h http.Header
	if err := json.Unmarshal([]byte(headers), &h); err != nil {
		log.Printf("ERROR: headers json: %s: %v", headers, err)
	}

	u := fmt.Sprintf("%s://%s%s", proto, host, route)

	rd := strings.NewReader(body)

	for i := range count {
		begin := time.Now()

		resp, err := request(client, method, u, virtualHost, rd, h)

		elap := time.Since(begin)

		log.Printf("%d/%d: virtual_host='%s' %s %s: latency=%v status=%d remote=%s http=%s tls=%q response='%s' error='%v'",
			i+1, count, virtualHost, method, u, elap, resp.status, resp.remote,
			resp.httpProto, resp.tlsVersion, resp.body, err)

		time.Sleep(interval)
	}
}

type response struct {
	body       string
	status     int
	remote     string
	httpProto  string
	tlsVersion string
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
		body:      string(respBody),
		status:    resp.StatusCode,
		remote:    remote,
		httpProto: resp.Proto,
	}

	if resp.TLS != nil {
		out.tlsVersion = tls.VersionName(resp.TLS.Version)
	}

	return
}
