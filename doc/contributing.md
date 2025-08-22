# Contributing

We welcome contributions of all kinds. Before submitting a pull request:

1. Ensure your code is formatted with `gofmt` and targets Go 1.23.
2. Run the standard checks:
   ```bash
   make lint
   make build
   make test
   ```
3. Follow existing patterns in the `internal/` packages and prefer clarity over cleverness.
4. Use structured logging via `internal/logx` and choose log levels according to the project policy.

For larger changes, start a discussion or open an issue to align on design
before implementing.
