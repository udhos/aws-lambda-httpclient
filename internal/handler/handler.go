// Package handler implemetns lambda handler.
package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/udhos/boilerplate/envconfig"
)

// Version is lambda version.
const Version = "0.0.0"

// HandleRequest is lambda handler.
func HandleRequest() {

	const me = "aws-lambda-httpclient"

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

	client := http.DefaultClient
	client.Timeout = timeout

	var h http.Header
	if err := json.Unmarshal([]byte(headers), &h); err != nil {
		log.Printf("ERROR: headers json: %s: %v", headers, err)
	}

	u := fmt.Sprintf("%s://%s%s", proto, host, route)

	rd := strings.NewReader(body)

	for i := range count {
		begin := time.Now()

		resp, status, err := request(client, method, u, virtualHost, rd, h)

		elap := time.Since(begin)

		log.Printf("%d/%d: virtual_host='%s' %s %s: latency=%v status=%d response='%s' error='%v'",
			i+1, count, virtualHost, method, u, elap, status, resp, err)

		time.Sleep(interval)
	}
}

func request(client *http.Client, method, u, virtualHost string,
	reqBody io.Reader, h http.Header) (string, int, error) {

	req, errReq := http.NewRequest(method, u, reqBody)
	if errReq != nil {
		return "", 0, errReq
	}

	req.Header = h

	req.Host = virtualHost

	resp, errResp := client.Do(req)
	if errResp != nil {
		return "", 0, errResp
	}
	defer resp.Body.Close()

	respBody, errBody := io.ReadAll(resp.Body)
	if errBody != nil {
		return "", 0, errResp
	}

	return string(respBody), resp.StatusCode, nil
}
