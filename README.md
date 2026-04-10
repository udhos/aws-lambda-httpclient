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
2026/04/10 03:36:07 aws-lambda-httpclient 0.0.7
2026/04/10 03:36:07 envconfig.NewSimple: SECRET_ROLE_ARN=''
2026/04/10 03:36:07 METHOD=[] using METHOD=GET default=GET
2026/04/10 03:36:07 PROTO=[] using PROTO=http default=http
2026/04/10 03:36:07 URL_HOST=[] using URL_HOST=httpbin.org default=httpbin.org
2026/04/10 03:36:07 VIRTUAL_HOST=[] using VIRTUAL_HOST= default=
2026/04/10 03:36:07 ROUTE=[] using ROUTE=/get default=/get
2026/04/10 03:36:07 BODY=[] using BODY=body default=body
2026/04/10 03:36:07 HEADERS=[] using HEADERS={"content-type":["application/json"],"who-am-i":["aws-lambda-httpclient"]} default={"content-type":["application/json"],"who-am-i":["aws-lambda-httpclient"]}
2026/04/10 03:36:07 COUNT=[1] using COUNT=1 default=3
2026/04/10 03:36:07 INTERVAL=[] using INTERVAL=1s default=1s
2026/04/10 03:36:07 TIMEOUT=[] using TIMEOUT=1s default=1s
2026/04/10 03:36:07 TLS_INSECURE_SKIP_VERIFY=[] using TLS_INSECURE_SKIP_VERIFY=false default=false
2026/04/10 03:36:07 CHECK_DNS=[] using CHECK_DNS=true default=true
2026/04/10 03:36:07 CHECK_CONNECT=[] using CHECK_CONNECT=true default=true
2026/04/10 03:36:07 FAMILY_DNS=[] using FAMILY_DNS=ip default=ip
2026/04/10 03:36:07 FAMILY_CONNECT=[] using FAMILY_CONNECT=tcp default=tcp
2026/04/10 03:36:07 LOG_HEADERS=[] using LOG_HEADERS=true default=true
2026/04/10 03:36:07 LOG_BODY=[] using LOG_BODY=true default=true
2026/04/10 03:36:07 attempt=1/1: DNS lookup SUCCESS latency=12.254891ms host=httpbin.org: [98.94.233.70 52.6.211.202 44.198.227.194 34.234.13.116 52.71.108.149 54.82.157.242 18.214.245.199 52.6.193.180]
2026/04/10 03:36:07 attempt=1/1: addr=1/8: connect SUCCESS latency=193.252162ms: 98.94.233.70:80(port-lookup)
2026/04/10 03:36:07 attempt=1/1: addr=2/8: connect SUCCESS latency=204.582775ms: 52.6.211.202:80(port-lookup)
2026/04/10 03:36:07 attempt=1/1: addr=3/8: connect SUCCESS latency=133.980783ms: 44.198.227.194:80(port-lookup)
2026/04/10 03:36:08 attempt=1/1: addr=4/8: connect SUCCESS latency=172.790136ms: 34.234.13.116:80(port-lookup)
2026/04/10 03:36:08 attempt=1/1: addr=5/8: connect SUCCESS latency=204.586248ms: 52.71.108.149:80(port-lookup)
2026/04/10 03:36:08 attempt=1/1: addr=6/8: connect SUCCESS latency=204.350907ms: 54.82.157.242:80(port-lookup)
2026/04/10 03:36:08 attempt=1/1: addr=7/8: connect SUCCESS latency=205.397775ms: 18.214.245.199:80(port-lookup)
2026/04/10 03:36:08 attempt=1/1: addr=8/8: connect SUCCESS latency=204.149377ms: 52.6.193.180:80(port-lookup)
2026/04/10 03:36:09 attempt=1/1: virtual_host='' GET http://httpbin.org/get: latency=356.944976ms status=200 remote=52.71.108.149:80 http=HTTP/1.1 tls="" body_size=382 error='<nil>'
2026/04/10 03:36:09 attempt=1/1: response body size=382: {
  "args": {}, 
  "headers": {
    "Accept-Encoding": "gzip", 
    "Content-Length": "4", 
    "Content-Type": "application/json", 
    "Host": "httpbin.org", 
    "User-Agent": "Go-http-client/1.1", 
    "Who-Am-I": "aws-lambda-httpclient", 
    "X-Amzn-Trace-Id": "Root=1-69d89a59-51e1bf494a1658dd586bf876"
  }, 
  "origin": "177.33.85.207", 
  "url": "http://httpbin.org/get"
}
2026/04/10 03:36:09 attempt=1/1: response headers: Date=["Fri, 10 Apr 2026 06:36:09 GMT"] Content-Type=["application/json"] Content-Length=["382"] Server=["gunicorn/19.9.0"] Access-Control-Allow-Origin=["*"] Access-Control-Allow-Credentials=["true"]
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
FAMILY_DNS                 | ip          | IP family for DNS lookup: "ip", "ip4" or "ip6"
FAMILY_CONNECT             | tcp         | Network family for TCP connect: "tcp", "tcp4" or "tcp6"
LOG_HEADERS                | true        | Log response headers
LOG_BODY                   | true        | Log response body
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
