// Package main implements the lambda function.
package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/udhos/aws-lambda-httpclient/internal/handler"
)

func main() {
	lambda.Start(handler.HandleRequest)
}
