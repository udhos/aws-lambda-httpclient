#!/bin/bash

go install golang.org/x/vuln/cmd/govulncheck@latest
go install golang.org/x/tools/cmd/deadcode@latest
go install github.com/mgechev/revive@latest
go install honnef.co/go/tools/cmd/staticcheck@latest
go install golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest
go install github.com/gordonklaus/ineffassign@latest
go install github.com/client9/misspell/cmd/misspell@latest
go install github.com/fzipp/gocyclo/cmd/gocyclo@latest

gofmt -s -w .

revive ./...

staticcheck ./...

modernize -fix ./...

gocyclo -over 15 .

ineffassign ./...

misspell .

go mod tidy

govulncheck ./...

deadcode ./cmd/*

go env -w CGO_ENABLED=1

go test -race ./...

go env -w CGO_ENABLED=0

go install ./...

go build -tags lambda.norpc -o bootstrap ./cmd/aws-lambda-httpclient

chmod a+rx bootstrap

rm -f lambda.zip

zip -r lambda.zip bootstrap

go env -u CGO_ENABLED
