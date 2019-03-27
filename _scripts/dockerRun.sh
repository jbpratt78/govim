#!/usr/bin/env vbash

./_scripts/revendorToolsInternal.sh

go generate ./...
go test ./...

go mod tidy
# https://github.com/golang/go/issues/27868#issuecomment-431413621
go list all > /dev/null

if [[ -n $CHECK_GOFMT ]]; then diff <(echo -n) <(gofmt -d .); fi
test -z "$(git status --porcelain)" || (git status; git diff; false)
