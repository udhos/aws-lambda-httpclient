# aws-lambda-httpclient

[aws-lambda-httpclient](https://github.com/udhos/aws-lambda-httpclient) is an AWS lambda function that makes HTTP requests and logs results.

You can control the request parameters using env vars.

# Synopsis

It is intended as a handy tool that can be easily deployed to AWS Lambda and serve as a curl-like network probe to investigate network issues related to http endpoints. It helps with the common situation of having to spin up a new EC2 host for running curl on an VPC that lacks a preprovisioned bastion host. This tool can be provisioned to AWS in a matter of seconds (we provide a companion helper utility for that), and later deprovisioned as easily. From the Lambda console, we can fine tune its behavior using env vars, fire it up with the Test button, and finally check the logs for the results.

# Build

The command below will yield output files `bootstrap` and `lambda.zip`.

You can upload `lambda.zip` directly to Lambda.

    ./build.sh

# App

The build also generates the app `aws-lambda-httpclient-app` that can be invoked from the command-line.

Example:

```bash
$ COUNT=1 aws-lambda-httpclient-app 
2026/04/16 19:37:31 aws-lambda-httpclient 0.0.8
2026/04/16 19:37:31 envconfig.NewSimple: SECRET_ROLE_ARN=''
2026/04/16 19:37:31 METHOD=[] using METHOD=GET default=GET
2026/04/16 19:37:31 PROTO=[] using PROTO=http default=http
2026/04/16 19:37:31 URL_HOST=[] using URL_HOST=httpbin.org default=httpbin.org
2026/04/16 19:37:31 VIRTUAL_HOST=[] using VIRTUAL_HOST= default=
2026/04/16 19:37:31 ROUTE=[] using ROUTE=/get default=/get
2026/04/16 19:37:31 BODY=[] using BODY=body default=body
2026/04/16 19:37:31 HEADERS=[] using HEADERS={"content-type":["application/json"],"who-am-i":["aws-lambda-httpclient"]} default={"content-type":["application/json"],"who-am-i":["aws-lambda-httpclient"]}
2026/04/16 19:37:31 COUNT=[1] using COUNT=1 default=3
2026/04/16 19:37:31 INTERVAL=[] using INTERVAL=1s default=1s
2026/04/16 19:37:31 TIMEOUT=[] using TIMEOUT=1s default=1s
2026/04/16 19:37:31 TLS_INSECURE_SKIP_VERIFY=[] using TLS_INSECURE_SKIP_VERIFY=false default=false
2026/04/16 19:37:31 CHECK_DNS=[] using CHECK_DNS=true default=true
2026/04/16 19:37:31 CHECK_CONNECT=[] using CHECK_CONNECT=true default=true
2026/04/16 19:37:31 FAMILY_DNS=[] using FAMILY_DNS=ip default=ip
2026/04/16 19:37:31 FAMILY_CONNECT=[] using FAMILY_CONNECT=tcp default=tcp
2026/04/16 19:37:31 LOG_HEADERS=[] using LOG_HEADERS=true default=true
2026/04/16 19:37:31 LOG_BODY=[] using LOG_BODY=true default=true
2026/04/16 19:37:31 REUSE_CLIENT=[] using REUSE_CLIENT=false default=false
2026/04/16 19:37:31 attempt=1/1: DNS lookup SUCCESS latency=30.200961ms host=httpbin.org: [98.89.132.151 98.94.233.70 52.6.193.180 34.234.13.116 18.214.245.199 32.194.101.183 54.145.142.3 54.82.157.242]
2026/04/16 19:37:31 attempt=1/1: addr=1/8: connect SUCCESS latency=241.192096ms: 98.89.132.151:80(port-lookup)
2026/04/16 19:37:32 attempt=1/1: addr=2/8: connect SUCCESS latency=203.860443ms: 98.94.233.70:80(port-lookup)
2026/04/16 19:37:32 attempt=1/1: addr=3/8: connect SUCCESS latency=204.408218ms: 52.6.193.180:80(port-lookup)
2026/04/16 19:37:32 attempt=1/1: addr=4/8: connect SUCCESS latency=204.772739ms: 34.234.13.116:80(port-lookup)
2026/04/16 19:37:32 attempt=1/1: addr=5/8: connect SUCCESS latency=204.804875ms: 18.214.245.199:80(port-lookup)
2026/04/16 19:37:32 attempt=1/1: addr=6/8: connect SUCCESS latency=203.939483ms: 32.194.101.183:80(port-lookup)
2026/04/16 19:37:33 attempt=1/1: addr=7/8: connect SUCCESS latency=204.795626ms: 54.145.142.3:80(port-lookup)
2026/04/16 19:37:33 attempt=1/1: addr=8/8: connect SUCCESS latency=204.554574ms: 54.82.157.242:80(port-lookup)
2026/04/16 19:37:33 attempt=1/1: virtual_host='' GET http://httpbin.org/get: latency=512.583246ms status=200 remote=32.194.101.183:80 http=HTTP/1.1 tls="" body_size=382 error='<nil>'
2026/04/16 19:37:33 attempt=1/1: response body size=382: {
  "args": {}, 
  "headers": {
    "Accept-Encoding": "gzip", 
    "Content-Length": "4", 
    "Content-Type": "application/json", 
    "Host": "httpbin.org", 
    "User-Agent": "Go-http-client/1.1", 
    "Who-Am-I": "aws-lambda-httpclient", 
    "X-Amzn-Trace-Id": "Root=1-69e164ad-67576a8269716678664a0ddf"
  }, 
  "origin": "177.33.85.207", 
  "url": "http://httpbin.org/get"
}
2026/04/16 19:37:33 attempt=1/1: response headers: Content-Length=["382"] Server=["gunicorn/19.9.0"] Access-Control-Allow-Origin=["*"] Access-Control-Allow-Credentials=["true"] Date=["Thu, 16 Apr 2026 22:37:33 GMT"] Content-Type=["application/json"]
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
REUSE_CLIENT               | false       | Reuse http client for multiple requests
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

# Deployment

Once you have built the `lambda.zip` file, you can upload it to Lambda.

You can make use of customary deployment options like Console, AWS CLI,
Terraform, CloudFormation, etc.

We also provide a quick option to deploy the Lambda function into AWS from
the command-line using `aws-lambda-httpclient-deploy`.

## Deploy

Just run `aws-lambda-httpclient-deploy` and it will create everything for you.

```bash
$ aws-lambda-httpclient-deploy
2026/04/10 23:21:05 ensuring role: name=aws-lambda-httpclient
2026/04/10 23:21:05 finding role: name=aws-lambda-httpclient
2026/04/10 23:21:06 ensuring kms key: alias=alias/aws-lambda-httpclient
2026/04/10 23:21:08 ensuring lambda function: name=aws-lambda-httpclient
2026/04/10 23:21:13 ensuring log group: name=aws-lambda-httpclient
2026/04/10 23:21:14 deployment complete: function=aws-lambda-httpclient role=aws-lambda-httpclient security_group= handler=main kms_key=arn:aws:kms:us-east-1:140330866198:key/d042a291-c54c-466f-985a-60c8551c7550
```

## Destroy

Do not forget to clean-up everything when you are done:

```bash
$ aws-lambda-httpclient-deploy -destroy
2026/04/10 23:20:25 deleting lambda function: name=aws-lambda-httpclient
2026/04/10 23:20:26 deleting kms key: alias=alias/aws-lambda-httpclient
2026/04/10 23:20:27 scheduled kms key deletion: key=310024ba-0b86-462e-95f8-bb5e35066546 pending_days=7
2026/04/10 23:20:27 deleting role: name=aws-lambda-httpclient
2026/04/10 23:20:31 skipping security group deletion because -vpc-id was not provided
2026/04/10 23:20:31 deleting log group: function=aws-lambda-httpclient
2026/04/10 23:20:32 destroy complete: function=aws-lambda-httpclient
```
