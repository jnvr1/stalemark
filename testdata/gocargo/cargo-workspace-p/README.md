# cargo workspace p

```bash
cargo run -p cli
cargo build --package core
cargo run -p cli --bin extra
cargo run --bin extra
cargo test -p core@0.1.0
cargo run -p skip
cargo run -p clii
cargo run -p cli --bin missing
cargo run -p ../elsewhere
cd tools/cli && cargo run --bin extra
cd ../.. && cargo run -p core
cargo doc -p serde
cargo build -p anyhow
cargo test -p log
cargo test -p my-log
cargo check -p libc
cargo doc -p tokio --open
cargo run -p serde --bin extra
cargo run -p nope
```
