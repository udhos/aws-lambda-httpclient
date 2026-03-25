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
2026/03/24 19:28:32 envconfig.NewSimple: SECRET_ROLE_ARN=''
2026/03/24 19:28:32 METHOD=[] using METHOD=GET default=GET
2026/03/24 19:28:32 PROTO=[] using PROTO=http default=http
2026/03/24 19:28:32 URL_HOST=[] using URL_HOST=httpbin.org default=httpbin.org
2026/03/24 19:28:32 VIRTUAL_HOST=[] using VIRTUAL_HOST= default=
2026/03/24 19:28:32 ROUTE=[] using ROUTE=/get default=/get
2026/03/24 19:28:32 BODY=[] using BODY=body default=body
2026/03/24 19:28:32 HEADERS=[] using HEADERS={"content-type":["application/json"],"who-am-i":["aws-lambda-httpclient"]} default={"content-type":["application/json"],"who-am-i":["aws-lambda-httpclient"]}
2026/03/24 19:28:32 COUNT=[1] using COUNT=1 default=3
2026/03/24 19:28:32 INTERVAL=[] using INTERVAL=1s default=1s
2026/03/24 19:28:32 TIMEOUT=[] using TIMEOUT=1s default=1s
2026/03/24 19:28:33 1/1: virtual_host='' GET http://httpbin.org/get: latency=920.234425ms status=200 response='{
  "args": {}, 
  "headers": {
    "Accept-Encoding": "gzip", 
    "Content-Length": "4", 
    "Content-Type": "application/json", 
    "Host": "httpbin.org", 
    "User-Agent": "Go-http-client/1.1", 
    "Who-Am-I": "aws-lambda-httpclient", 
    "X-Amzn-Trace-Id": "Root=1-69c31010-2ccccd31478d26b4397b7ad1"
  }, 
  "origin": "177.33.85.207", 
  "url": "http://httpbin.org/get"
}
' error='<nil>'
```

# Env vars

Env var      | Default     | Comment
--           | --          | --
METHOD       | GET         |
PROTO        | http        |
URL_HOST     | httpbin.org | URL hostname (address to connect)
VIRTUAL_HOST | ""          | Force Host header
ROUTE        | /get        |
HEADERS      | {"content-type":["application/json"],"who-am-i":["aws-lambda-httpclient"]}
COUNT        | 3           | How many times to run
INTERVAL     | 1s          | Interval between requests
TIMEOUT      | 1s          | Request timeout

# Virtual Host

You can use URL_HOST and VIRTUAL_HOST to connect to an address and request another

URL_HOST = address to connect

VIRTUAL_HOST = server requested

Example:

```bash
URL_HOST=52.71.170.232 VIRTUAL_HOST=httpbin.org aws-lambda-httpclient-app
```
