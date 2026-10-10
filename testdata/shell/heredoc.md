# Heredoc

```bash
cat <<EOF > config.yml
name: demo
EOF
cat <<'EOF'
echo "not a command"
EOF
echo done
```

Console heredoc with secondary prompts:

```console
$ cat <<EOF
> hello
> EOF
hello
```
