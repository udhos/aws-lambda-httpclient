// Package main implements lambda-like app.
package main

import (
	"flag"
	"fmt"

	"github.com/udhos/aws-lambda-httpclient/internal/handler"
)

func main() {

	const me = "aws-lambda-httpclient"

	var version bool
	flag.BoolVar(&version, "version", false, "show version")
	flag.Parse()

	if version {
		fmt.Printf("%s version %s\n", me, handler.Version)
		return
	}

	handler.HandleRequest()
}
