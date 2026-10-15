# Environment variable checks

Three rules compare the variables your docs name with the repository.

| Rule | Severity | Reports |
|------|----------|---------|
| SM-E001 | error | a documented variable missing from the nearest example file |
| SM-E003 | note | a variable read by code that no doc mentions |
| SM-E004 | warning | a documented variable that no code reads (opt-in) |

## Sources

- Documented: `export` lines, `NAME=value` prefixes, dotenv blocks, and backticked
  UPPER_SNAKE names under setup-like headings, in every checked Markdown file.
- Example file: the nearest `.env.example`, `.env.sample` or `.env.template`
  (SM-E001 only).
- Code reads, literal names only, in `.js .mjs .cjs .ts .tsx .jsx .py .go .rs`:
  `process.env.X`, `process.env['X']`, `os.environ["X"]`, `os.environ.get("X")`,
  `os.getenv("X")`, `os.Getenv("X")`, `os.LookupEnv("X")`, `std::env::var("X")`
  and `env::var_os("X")`. Dynamic keys such as `process.env[name]` are ignored.
- Skipped directories: `.git`, `node_modules`, `vendor`, `target`, `dist`,
  `build`, `.venv`, `venv` and `testdata`. `.gitignore` is not parsed.
- Platform and toolchain names (`HOME`, `CI`, `GITHUB_*`, `CARGO_*`...) are
  never reported.

## Test-only reads

A variable read only under `test`, `tests`, `spec` or `__tests__`, or in files
named `*_test.go`, `test_*`, `*.test.*` or `*.spec.*`, is never reported as
undocumented. Those reads still count as reads for SM-E004.

## Precision limits

The scan is a regex over lines, not a parser, so it can match text inside
strings. Single-line comments are skipped (`//`, `#`, one-line `/* */` and lines
starting with `*`), but multi-line block comments and Python docstrings are not
detected. A `//` or `#` before a read hides it, even inside a string. This is
why SM-E003 is a note: it is printed by every output format but does not make
`stalemark check` exit 1. SM-E004 is a warning and is off by default.

## Options

In `.stalemark.yml`:

```yaml
env:
  documented-unused: true
```

Disable a rule entirely with `disable: [SM-E003]`.
