# js-yaml-go

This is the Go 1.23 port of nodeca/js-yaml 5.5.0. It provides the `js-yaml`
command for loading YAML as JSON and dumping JSON as YAML. The implementation
is self-contained and uses no third-party Go modules.

Build and test offline:

```sh
go test ./...
go build ./cmd/js-yaml
printf 'answer: 42\n' | ./js-yaml load
printf '{"answer":42}\n' | ./js-yaml dump
```

Run `js-yaml load --all`, select a schema with `--schema`, or pass the dumper
options listed in the migration instruction. Input defaults to standard input;
a final path reads that UTF-8 file. Errors are printed to standard error.

The original js-yaml MIT license is in `LICENSE`. `internal/yaml/ORIGINS.md`
retains the MIT and Apache-2.0 notices of the parser/emitter foundation adapted
by this port.
