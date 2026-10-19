# Demo

```bash
export CI=true
export HOME=/x
NODE_ENV=production npm test
export PATH=$PATH:/x
CGO_ENABLED=0 GOOS=linux go build ./...
DEBUG=* NODE_OPTIONS=--max-old-space-size=4096 npm start
RUST_LOG=debug cargo run
export GOPRIVATE=github.com/acme
CI=1 CI_STRICT_SMOKE=1 CI_COMMIT_SHA=abc python scripts/ci_validate.py
```
