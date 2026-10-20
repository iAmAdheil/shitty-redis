# Test scripts

Black-box tests that talk to the server over TCP, the way a real client does.

## `txn_test.py` — transactions (MULTI / EXEC / DISCARD / WATCH)

The script opens 2–3 client connections (A, B, C) and checks the replies.
Expected values follow real Redis behaviour, not the current state of this code.
A failing test means the server differs from Redis.

Each test starts its own server process, so one hang or crash cannot break the next test.

### Requirements

- Go and Python 3. No other packages.
- Port 6379 must be free. The server always binds to it. Stop any other Redis first.

### Run all tests

```sh
python3 testscripts/txn_test.py
```

The script builds the server from `./app` into a temporary folder and deletes it afterwards.

### List the tests

```sh
python3 testscripts/txn_test.py --list
```

Each line shows a test id and its name.

### Run a single test

Give the test id from `--list`:

```sh
python3 testscripts/txn_test.py watch_cleared_after_discard
```

You can also give part of a name. Every test that matches runs:

```sh
python3 testscripts/txn_test.py watch_cleared     # three tests
python3 testscripts/txn_test.py watch             # all WATCH tests
```

You can pass more than one:

```sh
python3 testscripts/txn_test.py basic_exec discard_drops_queue
```

An id that matches nothing stops the script with an error. Nothing runs.

### Skip the build

To reuse a server binary you built yourself:

```sh
cd app && go build -o /tmp/my-redis . && cd ..
python3 testscripts/txn_test.py --bin /tmp/my-redis watch
```

### Read the output

```
PASS  WATCH modified -> EXEC aborts
FAIL  WATCH cleared after DISCARD
        x DISCARD unwatches: next txn must NOT abort
          got: None
FAIL  basic MULTI/EXEC
        ! HANG: A: no reply within 2.0s
```

- `x` is a check that got a different reply than expected. `got:` shows the reply.
- `!` is a test that hung or crashed the server. A hang means no reply for 2 seconds.
- The exit code is 0 if all selected tests pass, and 1 otherwise.

### Notes

- Tests inside `MULTI` queue `PING` when the test is about WATCH only. `PING` takes no lock,
  so a failure in the WATCH logic does not get hidden by a problem in `EXEC`.
- The client sends one command per write and waits for the reply. The server reads
  one command per `conn.Read`, so do not pipeline commands in a new test.
- To add a test, write a `t_<id>(new)` function and add it to the `TESTS` list.
  `new("A")` creates a connected client. Return a list of `(description, ok, got)`.
