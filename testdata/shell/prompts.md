# Prompts

Mixed prompt and output lines:

```bash
$ go version
go version go1.27.2 linux/amd64
$ go test ./...
ok      github.com/jnvr1/stalemark      0.1s
$
$ make build
```

No prompt anywhere, every line is code:

```sh
make build
make test
```

Unlabeled dollar block:

```
$ ls -la
total 0
```

Other languages yield nothing:

```python
print("hi")
```
