#!/usr/bin/env python3
"""Multi-client transaction tests for the Go Redis clone.

Each test starts a fresh server process, so a deadlock or panic in one test
cannot break the next one. Expected values come from real Redis behaviour.

Usage:
  python3 testscripts/txn_test.py --list
  python3 testscripts/txn_test.py                      # run every test
  python3 testscripts/txn_test.py TEST [TEST ...]      # run chosen tests
  python3 testscripts/txn_test.py --bin PATH [TEST ..] # use a prebuilt server

TEST is a test id from --list, or any part of a test name.
"""
import argparse
import os
import shutil
import socket
import subprocess
import sys
import tempfile
import time

HOST, PORT = "127.0.0.1", 6379
TIMEOUT = 2.0
REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


class Hang(Exception):
    pass


class Client:
    """Raw RESP client. Sends one command per write, because the server
    reads one command per conn.Read call."""

    def __init__(self, name):
        self.name = name
        self.sock = socket.create_connection((HOST, PORT), timeout=TIMEOUT)
        self.buf = b""

    def close(self):
        self.sock.close()

    def send(self, *args):
        out = b"*%d\r\n" % len(args)
        for a in args:
            a = str(a).encode()
            out += b"$%d\r\n%s\r\n" % (len(a), a)
        self.sock.sendall(out)
        return self.read()

    def _line(self):
        while b"\r\n" not in self.buf:
            try:
                chunk = self.sock.recv(4096)
            except socket.timeout:
                raise Hang(f"{self.name}: no reply within {TIMEOUT}s")
            if not chunk:
                raise Hang(f"{self.name}: server closed the connection")
            self.buf += chunk
        line, self.buf = self.buf.split(b"\r\n", 1)
        return line

    def _need(self, n):
        while len(self.buf) < n + 2:
            try:
                chunk = self.sock.recv(4096)
            except socket.timeout:
                raise Hang(f"{self.name}: no reply within {TIMEOUT}s")
            if not chunk:
                raise Hang(f"{self.name}: server closed the connection")
            self.buf += chunk
        data, self.buf = self.buf[:n], self.buf[n + 2:]
        return data.decode(errors="replace")

    def read(self):
        line = self._line()
        t, rest = chr(line[0]), line[1:].decode(errors="replace")
        if t == "+":
            return rest
        if t == "-":
            return ("ERR", rest)
        if t == ":":
            return int(rest)
        if t == "$":
            n = int(rest)
            return None if n < 0 else self._need(n)
        if t == "*":
            n = int(rest)
            return None if n < 0 else [self.read() for _ in range(n)]
        raise ValueError(f"bad reply {line!r}")


def is_err(v, contains=""):
    return isinstance(v, tuple) and v[0] == "ERR" and contains.lower() in v[1].lower()


# ---------------------------------------------------------------- tests
# Each test takes a `new` factory that returns a connected Client and
# returns a list of (check description, ok, got) tuples.

def t_basic_exec(new):
    a = new("A")
    r = [a.send("MULTI")]
    r += [a.send("SET", "k1", "10"), a.send("INCR", "k1"), a.send("GET", "k1")]
    ex = a.send("EXEC")
    return [
        ("MULTI replies OK", r[0] == "OK", r[0]),
        ("each command replies QUEUED", r[1:] == ["QUEUED"] * 3, r[1:]),
        ("EXEC returns 3 replies [OK, 11, '11']", ex == ["OK", 11, "11"], ex),
    ]


def t_exec_without_multi(new):
    a = new("A")
    r = a.send("EXEC")
    return [("EXEC without MULTI -> error", is_err(r, "without multi"), r)]


def t_discard_without_multi(new):
    a = new("A")
    r = a.send("DISCARD")
    return [("DISCARD without MULTI -> error", is_err(r, "without multi"), r)]


def t_discard_drops_queue(new):
    a = new("A")
    a.send("SET", "k2", "1")
    a.send("MULTI")
    a.send("SET", "k2", "99")
    d = a.send("DISCARD")
    g = a.send("GET", "k2")
    return [
        ("DISCARD replies OK", d == "OK", d),
        ("queued SET was not applied", g == "1", g),
        ("connection is back to normal mode (GET is not QUEUED)", g != "QUEUED", g),
    ]


def t_queue_is_per_connection(new):
    a, b = new("A"), new("B")
    a.send("MULTI")
    qa = a.send("SET", "k3", "fromA")
    rb = b.send("SET", "k3", "fromB")
    gb = b.send("GET", "k3")
    return [
        ("A's SET is QUEUED", qa == "QUEUED", qa),
        ("B is not in a txn: SET replies OK, not QUEUED", rb == "OK", rb),
        ("B's GET runs immediately and returns B's value", gb == "fromB", gb),
    ]


def t_queued_write_invisible_until_exec(new):
    a, b = new("A"), new("B")
    a.send("SET", "k4", "old")
    a.send("MULTI")
    a.send("SET", "k4", "new")
    before = b.send("GET", "k4")
    ex = a.send("EXEC")
    after = b.send("GET", "k4")
    return [
        ("B sees old value before A's EXEC", before == "old", before),
        ("A's EXEC succeeds", ex == ["OK"], ex),
        ("B sees new value after A's EXEC", after == "new", after),
    ]


def t_error_inside_exec(new):
    a = new("A")
    a.send("SET", "s", "abc")
    a.send("MULTI")
    a.send("INCR", "s")          # fails at exec time: not an integer
    a.send("SET", "after", "1")  # must still run
    ex = a.send("EXEC")
    ok = (isinstance(ex, list) and len(ex) == 2 and is_err(ex[0], "integer")
          and ex[1] == "OK")
    return [("failing INCR gives an error entry, next command still runs", ok, ex)]


def t_nested_multi(new):
    a = new("A")
    a.send("MULTI")
    r = a.send("MULTI")
    return [("MULTI inside MULTI -> error (real Redis: 'MULTI calls can not be nested')",
             is_err(r, "nested"), r)]


def t_watch_untouched_succeeds(new):
    a = new("A")
    a.send("SET", "w1", "1")
    w = a.send("WATCH", "w1")
    a.send("MULTI")
    a.send("PING")  # PING takes no lock, so EXEC cannot self-deadlock
    ex = a.send("EXEC")
    return [
        ("WATCH replies OK", w == "OK", w),
        ("EXEC succeeds when nobody touched the key", ex == ["PONG"], ex),
    ]


def t_watch_modified_aborts(new):
    a, b = new("A"), new("B")
    a.send("SET", "w2", "1")
    a.send("WATCH", "w2")
    b.send("SET", "w2", "from-B")
    a.send("MULTI")
    q = a.send("SET", "w2", "from-A")
    ex = a.send("EXEC")
    g = b.send("GET", "w2")
    return [
        ("command still replies QUEUED", q == "QUEUED", q),
        ("EXEC returns null (aborted)", ex is None, ex),
        ("A's queued SET was not applied", g == "from-B", g),
    ]


def t_watch_other_key_no_abort(new):
    a, b = new("A"), new("B")
    a.send("WATCH", "w3")
    b.send("SET", "unrelated", "x")
    a.send("MULTI")
    a.send("PING")
    ex = a.send("EXEC")
    return [("SET on an unwatched key does not abort", ex == ["PONG"], ex)]


def t_watch_multiple_keys(new):
    a, b = new("A"), new("B")
    a.send("WATCH", "m1", "m2", "m3")
    b.send("SET", "m3", "touched")
    a.send("MULTI")
    a.send("GET", "m1")
    ex = a.send("EXEC")
    return [("touching any one of several watched keys aborts", ex is None, ex)]


def t_watch_inside_multi(new):
    a = new("A")
    a.send("MULTI")
    r = a.send("WATCH", "x")
    return [("WATCH inside MULTI -> error", is_err(r, "watch"), r)]


def t_two_watchers_one_writer(new):
    a, b, c = new("A"), new("B"), new("C")
    a.send("WATCH", "shared")
    b.send("WATCH", "shared")
    c.send("SET", "shared", "changed")
    out = []
    for cl in (a, b):
        cl.send("MULTI")
        cl.send("GET", "shared")
        ex = cl.send("EXEC")
        out.append((f"{cl.name}'s EXEC aborts", ex is None, ex))
    return out


def t_watch_cleared_after_failed_exec(new):
    # Real Redis: EXEC unwatches everything, even when it aborts.
    a, b = new("A"), new("B")
    a.send("WATCH", "w5")
    b.send("SET", "w5", "1")
    a.send("MULTI")
    first = a.send("EXEC")          # aborts
    b.send("SET", "w5", "2")        # A is no longer watching w5
    a.send("MULTI")
    a.send("PING")
    second = a.send("EXEC")
    return [
        ("first EXEC aborts", first is None, first),
        ("second txn (no new WATCH) must NOT abort", second == ["PONG"], second),
    ]


def t_watch_cleared_after_good_exec(new):
    a, b = new("A"), new("B")
    a.send("WATCH", "w6")
    a.send("MULTI")
    a.send("PING")
    first = a.send("EXEC")          # succeeds
    b.send("SET", "w6", "late")     # A is no longer watching w6
    a.send("MULTI")
    a.send("PING")
    second = a.send("EXEC")
    return [
        ("first EXEC succeeds", first == ["PONG"], first),
        ("second txn (no new WATCH) must NOT abort", second == ["PONG"], second),
    ]


def t_watch_cleared_after_discard(new):
    a, b = new("A"), new("B")
    a.send("WATCH", "w7")
    a.send("MULTI")
    a.send("DISCARD")
    b.send("SET", "w7", "x")
    a.send("MULTI")
    a.send("PING")
    ex = a.send("EXEC")
    return [("DISCARD unwatches: next txn must NOT abort", ex == ["PONG"], ex)]


def t_own_write_inside_exec(new):
    # A watches k, writes k inside its own EXEC; that must not poison the
    # next transaction on A.
    a = new("A")
    a.send("WATCH", "w8")
    a.send("MULTI")
    a.send("SET", "w8", "1")
    first = a.send("EXEC")
    a.send("MULTI")
    a.send("SET", "w8", "2")
    second = a.send("EXEC")
    return [
        ("first EXEC succeeds", first == ["OK"], first),
        ("second EXEC succeeds", second == ["OK"], second),
    ]


def t_queue_reset_after_exec(new):
    a = new("A")
    a.send("MULTI")
    a.send("SET", "q1", "1")
    a.send("EXEC")
    a.send("MULTI")
    ex = a.send("EXEC")
    return [("second empty txn returns empty array, not a replay", ex == [], ex)]


def t_queue_reset_after_discard(new):
    a = new("A")
    a.send("MULTI")
    a.send("SET", "q2", "1")
    a.send("DISCARD")
    a.send("MULTI")
    ex = a.send("EXEC")
    return [("EXEC after DISCARD returns empty array", ex == [], ex)]


def t_empty_exec(new):
    a = new("A")
    a.send("MULTI")
    ex = a.send("EXEC")
    return [("empty MULTI/EXEC -> empty array", ex == [], ex)]


def t_connection_close_unwatches(new):
    a, b = new("A"), new("B")
    a.send("WATCH", "w9")
    a.close()
    time.sleep(0.2)
    # Not observable from the wire except by absence of crashes: B writes the
    # key, which touches the dead client's txn. Server must stay alive.
    b.send("SET", "w9", "x")
    p = b.send("PING")
    return [("server survives a write to a key a dead client watched",
             p == "PONG", p)]


TESTS = [
    ("basic MULTI/EXEC", t_basic_exec),
    ("EXEC without MULTI", t_exec_without_multi),
    ("DISCARD without MULTI", t_discard_without_multi),
    ("DISCARD drops the queue", t_discard_drops_queue),
    ("queue is per connection", t_queue_is_per_connection),
    ("queued write invisible until EXEC", t_queued_write_invisible_until_exec),
    ("error inside EXEC", t_error_inside_exec),
    ("nested MULTI", t_nested_multi),
    ("empty MULTI/EXEC", t_empty_exec),
    ("queue reset after EXEC", t_queue_reset_after_exec),
    ("queue reset after DISCARD", t_queue_reset_after_discard),
    ("WATCH untouched -> EXEC ok", t_watch_untouched_succeeds),
    ("WATCH modified -> EXEC aborts", t_watch_modified_aborts),
    ("WATCH other key -> no abort", t_watch_other_key_no_abort),
    ("WATCH several keys", t_watch_multiple_keys),
    ("WATCH inside MULTI", t_watch_inside_multi),
    ("two watchers, one writer", t_two_watchers_one_writer),
    ("WATCH cleared after failed EXEC", t_watch_cleared_after_failed_exec),
    ("WATCH cleared after good EXEC", t_watch_cleared_after_good_exec),
    ("WATCH cleared after DISCARD", t_watch_cleared_after_discard),
    ("WATCH own SET inside EXEC (needs EXEC lock fix)", t_own_write_inside_exec),
    ("dead client's watch", t_connection_close_unwatches),
]


# ---------------------------------------------------------------- runner

def start_server(binary):
    proc = subprocess.Popen([binary], stdout=subprocess.PIPE,
                            stderr=subprocess.STDOUT)
    for _ in range(50):
        try:
            socket.create_connection((HOST, PORT), timeout=0.2).close()
            return proc
        except OSError:
            time.sleep(0.05)
    proc.kill()
    raise RuntimeError("server did not start on port 6379")


def stop_server(proc):
    if proc.poll() is None:
        proc.kill()
    out = proc.communicate()[0].decode(errors="replace")
    return out


def run_one(binary, fn):
    proc = start_server(binary)
    clients = []

    def new(name):
        c = Client(name)
        clients.append(c)
        return c

    checks, crash = [], None
    try:
        checks = fn(new)
    except Hang as e:
        crash = f"HANG: {e}"
    except Exception as e:  # noqa: BLE001
        crash = f"{type(e).__name__}: {e}"
    finally:
        for c in clients:
            try:
                c.close()
            except OSError:
                pass
        alive = proc.poll() is None
        log = stop_server(proc)
    if not alive and not crash:
        crash = "server process died during the test"
    if not alive:
        panic = [l for l in log.splitlines() if "panic" in l or ".go:" in l][:3]
        if panic:
            crash = (crash or "") + " | " + " / ".join(p.strip() for p in panic)
    return checks, crash


def test_id(fn):
    return fn.__name__[len("t_"):]


def select(selectors):
    """Return the tests that match the selectors. Exit if one matches nothing."""
    if not selectors:
        return TESTS
    chosen = []
    for sel in selectors:
        s = sel.lower()
        hits = [(n, f) for n, f in TESTS if s == test_id(f) or s in test_id(f)
                or s in n.lower()]
        exact = [h for h in hits if s == test_id(h[1])]
        hits = exact or hits
        if not hits:
            sys.exit(f"No test matches '{sel}'. Run with --list to see test ids.")
        chosen += [h for h in hits if h not in chosen]
    return chosen


def port_is_free():
    try:
        socket.create_connection((HOST, PORT), timeout=0.2).close()
    except OSError:
        return True
    return False


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("tests", nargs="*", help="test ids or name fragments")
    ap.add_argument("--list", action="store_true", help="list test ids and exit")
    ap.add_argument("--bin", help="path to a prebuilt server binary")
    args = ap.parse_args()

    if args.list:
        for name, fn in TESTS:
            print(f"{test_id(fn):45} {name}")
        return

    chosen = select(args.tests)

    if not port_is_free():
        sys.exit(f"Port {PORT} is already in use. Stop the other server first.")

    tmpdir = None
    binary = args.bin
    if not binary:
        tmpdir = tempfile.mkdtemp(prefix="txn-test-")
        binary = os.path.join(tmpdir, "redis-under-test")
        print("Building server from ./app ...")
        subprocess.run(["go", "build", "-o", binary, "."],
                       cwd=os.path.join(REPO, "app"), check=True)

    try:
        run_all(binary, chosen)
    finally:
        if tmpdir:
            shutil.rmtree(tmpdir, ignore_errors=True)


def run_all(binary, chosen):
    passed = failed = 0
    failures = []
    for name, fn in chosen:
        checks, crash = run_one(binary, fn)
        ok = bool(checks) and all(c[1] for c in checks) and not crash
        print(f"{'PASS' if ok else 'FAIL'}  {name}")
        for desc, good, got in checks:
            if not good:
                print(f"        x {desc}\n          got: {got!r}")
        if crash:
            print(f"        ! {crash}")
        if ok:
            passed += 1
        else:
            failed += 1
            failures.append(name)
    print(f"\n{passed} passed, {failed} failed")
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
