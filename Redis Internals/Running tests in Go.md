
## `go test` — core commands

**Run every test in every package:**

```bash
go test ./...
```

**Run every test in one package:**

```bash
go test ./app/structures/listpack
```

**Run tests in more than one package:**

```bash
go test ./app/structures/list/... ./app/structures/listpack/...
```

The `/...` suffix means "this package and every package below it." For a leaf package (no subpackages), it makes no difference — `./app/structures/list` and `./app/structures/list/...` match the same thing.

## Flags you will use most

| Flag                   | Effect                                                                                  |
| ---------------------- | --------------------------------------------------------------------------------------- |
| `-v`                   | Print each test name and result (`PASS`/`FAIL`), not just a package summary.            |
| `-run <regex>`         | Run only tests whose name matches the regex. Skip everything else.                      |
| `-count=1`             | Force a real re-run. Disables Go's test cache (see below).                              |
| `-race`                | Enable the race detector. Slower, but catches data races.                               |
| `-timeout <duration>`  | Kill the test run if it exceeds this duration (default `10m`). Example: `-timeout 30s`. |
| `-cover`               | Print a coverage percentage after the run.                                              |
| `-coverprofile <file>` | Write detailed coverage data to a file, for `go tool cover -html=<file>`.               |
| `-list <regex>`        | List matching test names. Do not run them.                                              |

## `-run` and subtests

For a table-driven test like `TestPushR_Integers_RoundTrip`, each `t.Run(name, ...)` call creates a subtest addressed as `ParentTest/subtest-name`. `-run` matches against that full path, one `/`-separated regex per level.

```bash
# Run every subtest of TestPushR_Integers_RoundTrip
go test ./app/structures/listpack -run TestPushR_Integers_RoundTrip -v

# Run only the "127" subtest
go test ./app/structures/listpack -run '^TestPushR_Integers_RoundTrip$/^127$' -v

# Run every test whose name contains "NodeOverflow", in any package
go test ./... -run NodeOverflow -v
```

`-run` does a substring/regex match, not an exact match, unless you anchor it with `^...$`. `-run NodeOverflow` matches both `TestRPUSH_NodeOverflow_CreatesNewNode` and `TestLPUSH_NodeOverflow_CreatesNewNode`.

## The test cache — why `-count=1` matters

If nothing changed since the last run — same code, same flags — `go test` prints `(cached)` and skips real execution:

```
ok   github.com/.../listpack   (cached)
```

This is the "did my breakpoint even run?" trap from earlier. Any of these forces a real run:

- Change any source file (even a comment) — the most common case, so it usually isn't a problem.
- Add `-v` — cached runs never print `-v` output live, so you would notice.
- Add `-count=1` explicitly — always bypasses the cache, regardless of what changed.

Rule of thumb: `-count=1` any time you are debugging and unsure whether the binary you're looking at is fresh.

## Filtering the output down to pass/fail lines

The command I've been running in this session:

```bash
go test ./app/structures/list/... ./app/structures/listpack/... -v 2>&1 | grep -E "^(--- FAIL|--- PASS|FAIL|ok)"
```

`-v` prints every subtest line; `grep` trims it to just the pass/fail summary lines, dropping the `=== RUN` noise.

## Combining with the debugger (from earlier)

```bash
# Run one specific subtest under Delve, ready for a breakpoint
dlv test ./app/structures/listpack -- -test.run '^TestPushR_Integers_RoundTrip$/^127$'
```

Everything after `--` is passed straight to the compiled test binary as its own flags — `-test.run` is the compiled-binary form of `go test`'s `-run`.