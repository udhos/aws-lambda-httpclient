# aws-lambda-httpclient

[aws-lambda-httpclient](https://github.com/udhos/aws-lambda-httpclient) is an AWS lambda function that makes HTTP requests and logs results.

You can control the requests from env vars.

# Build

The command below will yield output files `bootstrap` and `lambda.zip`.

You can upload `lambda.zip` directly to Lambda.

    ./build.sh

# App

The build also generates the app `aws-lambda-httpclient-app` that can be invoked from the command-line.

Example:

```bash
$ COUNT=1 aws-lambda-httpclient-app
2026/04/10 01:58:25 aws-lambda-httpclient 0.0.5
2026/04/10 01:58:25 envconfig.NewSimple: SECRET_ROLE_ARN=''
2026/04/10 01:58:25 METHOD=[] using METHOD=GET default=GET
2026/04/10 01:58:25 PROTO=[] using PROTO=http default=http
2026/04/10 01:58:25 URL_HOST=[] using URL_HOST=httpbin.org default=httpbin.org
2026/04/10 01:58:25 VIRTUAL_HOST=[] using VIRTUAL_HOST= default=
2026/04/10 01:58:25 ROUTE=[] using ROUTE=/get default=/get
2026/04/10 01:58:25 BODY=[] using BODY=body default=body
2026/04/10 01:58:25 HEADERS=[] using HEADERS={"content-type":["application/json"],"who-am-i":["aws-lambda-httpclient"]} default={"content-type":["application/json"],"who-am-i":["aws-lambda-httpclient"]}
2026/04/10 01:58:25 COUNT=[1] using COUNT=1 default=3
2026/04/10 01:58:25 INTERVAL=[] using INTERVAL=1s default=1s
2026/04/10 01:58:25 TIMEOUT=[] using TIMEOUT=1s default=1s
2026/04/10 01:58:25 TLS_INSECURE_SKIP_VERIFY=[] using TLS_INSECURE_SKIP_VERIFY=false default=false
2026/04/10 01:58:25 CHECK_DNS=[] using CHECK_DNS=true default=true
2026/04/10 01:58:25 CHECK_CONNECT=[] using CHECK_CONNECT=true default=true
2026/04/10 01:58:25 attempt=1/1: DNS lookup SUCCESS host=httpbin.org: [44.198.227.194 32.194.101.183 52.6.193.180 100.52.42.97 52.71.108.149 54.145.142.3 18.214.245.199 98.94.233.70]
2026/04/10 01:58:25 attempt=1/1: addr=1/8: connect SUCCESS: 44.198.227.194:80(port-lookup)
2026/04/10 01:58:25 attempt=1/1: addr=2/8: connect SUCCESS: 32.194.101.183:80(port-lookup)
2026/04/10 01:58:25 attempt=1/1: addr=3/8: connect SUCCESS: 52.6.193.180:80(port-lookup)
2026/04/10 01:58:25 attempt=1/1: addr=4/8: connect SUCCESS: 100.52.42.97:80(port-lookup)
2026/04/10 01:58:26 attempt=1/1: addr=5/8: connect SUCCESS: 52.71.108.149:80(port-lookup)
2026/04/10 01:58:26 attempt=1/1: addr=6/8: connect SUCCESS: 54.145.142.3:80(port-lookup)
2026/04/10 01:58:26 attempt=1/1: addr=7/8: connect SUCCESS: 18.214.245.199:80(port-lookup)
2026/04/10 01:58:26 attempt=1/1: addr=8/8: connect SUCCESS: 98.94.233.70:80(port-lookup)
2026/04/10 01:58:27 attempt=1/1: virtual_host='' GET http://httpbin.org/get: latency=409.957839ms status=200 remote=18.214.245.199:80 http=HTTP/1.1 tls="" response='{
  "args": {}, 
  "headers": {
    "Accept-Encoding": "gzip", 
    "Content-Length": "4", 
    "Content-Type": "application/json", 
    "Host": "httpbin.org", 
    "User-Agent": "Go-http-client/1.1",
    "Who-Am-I": "aws-lambda-httpclient", 
    "X-Amzn-Trace-Id": "Root=1-69d88373-4d6df8d420e33dbd65b35651"
  }, 
  "origin": "177.33.85.207", 
  "url": "http://httpbin.org/get"
}
' error='<nil>'
```

# Env vars

Env var                    | Default     | Comment
--                         | --          | --
METHOD                     | GET         |
PROTO                      | http        |
URL_HOST                   | httpbin.org | URL hostname (address to connect). You can force the port by using URL_HOST=hostname:port.
VIRTUAL_HOST               | ""          | Force Host header
ROUTE                      | /get        |
BODY                       | body        | Request body
HEADERS                    | {"content-type":["application/json"],"who-am-i":["aws-lambda-httpclient"]} | Request headers
COUNT                      | 3           | How many times to run
INTERVAL                   | 1s          | Interval between requests
TIMEOUT                    | 1s          | Request timeout
TLS_INSECURE_SKIP_VERIFY   | false       | Skip TLS certificate verification
CHECK_DNS                  | true        | Perform DNS lookup
CHECK_CONNECT              | true        | Perform TCP connect check
LOG_HEADERS                | true        | Log response headers
HTTP_PROXY or http_proxy   | ""          | HTTP proxy like "http://proxy:8080"
HTTPS_PROXY or https_proxy | ""          | HTTPS proxy like "http://proxy:8080"
NO_PROXY or no_proxy       | ""          | No proxy for hosts like "localhost"

# Virtual Host

You can use URL_HOST and VIRTUAL_HOST to connect to an address and request another

URL_HOST = address to connect

VIRTUAL_HOST = server requested

Example:

```bash
URL_HOST=52.71.170.232 VIRTUAL_HOST=httpbin.org aws-lambda-httpclient-app
```
