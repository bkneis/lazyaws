.PHONY: test vulncheck gosec ci

test:
	go test ./...

vulncheck:
	@which govulncheck > /dev/null 2>&1 || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

gosec:
	@which gosec > /dev/null 2>&1 || go install github.com/securego/gosec/v2/cmd/gosec@latest
	gosec ./...

ci: test vulncheck gosec
