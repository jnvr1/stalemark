# go ok

```bash
go run ./cmd/app
go run ./cmd/app --port 8080
go build ./...
go test ./...
go test -race -run TestX -count=1 ./internal/...
go build -o ./bin/app ./cmd/app
go run cmd/app/main.go
go build .
go run example.com/other/tool@latest
go test example.com/ok/internal/lib
go vet ./nope
go mod tidy
go test -coverprofile=cover.out ./...
go run -tags dev ./cmd/app
go run -ldflags "-X main.version=1" ./cmd/app
go test -v -timeout 30s ./internal/lib
go install ./cmd/nope
go generate ./...
```
