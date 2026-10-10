# Continuation

Backslash continuation across three lines:

```bash
docker run \
  -p 8080:80 \
  nginx:alpine
echo done
```

Prompt mode with `> ` secondary prompts:

```console
$ curl -fsSL https://example.com/install.sh \
>   -o install.sh \
>   && sh install.sh
installing...
```

Prompt mode without secondary prompts keeps the continued lines:

```bash
$ tar \
    -xzf archive.tar.gz
extracted 3 files
$ ls
```
