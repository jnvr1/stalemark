# Demo

Every command and path below exists, including the tricky ones.

```bash
npm run build
npm run build -- --watch
npm run build --if-present
pnpm --filter app run build
npm run <script-name>
make build
make -C sub build
task build
task --list
just build
just --list
poetry run serve
poetry run python -m app
go run ./cmd/app
go build -o ./bin/app ./cmd/app
go test ./...
cargo run --bin app
./scripts/run.sh
```

Write a config, the lines inside are file content, not commands:

```bash
cat <<EOF
npm run nothing
make nothing
task nothing
EOF
```

Work in a scratch directory that the commands create:

```bash
mkdir -p tmp/work && cd tmp/work
```

The result lands in `tmp/work/out.txt`.

The entry point is `cmd/app/main.go`, see the [guide](docs/guide.md).

Deliberately ignored:

<!-- stalemark-ignore -->
```bash
npm run bogus
```

Known drift, accepted in the baseline:

```bash
npm run buidl
```
