package main

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
	"github.com/codecrafters-io/redis-starter-go/app/server"
)

// Tests for the blocking commands: BLPOP and XREAD BLOCK.
// They call GetCom + HandleCom directly, so no network is involved.

const hangLimit = 2 * time.Second

// HandleCom needs a server. None of these commands read it, so any config works.
var testServer = server.New(&server.Config{Port: "6379", Role: server.Master})

type reply struct {
	out      []byte
	err      error
	panicked any
}

// resetState replaces the global state, so a test that deadlocked or
// leaked a goroutine cannot block the next test.
func resetState() {
	kmu = &sync.RWMutex{}
	keyspace = make(map[string]*Structure)
	blpopWaiters = make(map[string][]chan string)
	xreadWaiters = make(map[string][]chan string)
}

func run(base string, args ...string) (r reply) {
	defer func() { r.panicked = recover() }()
	com, err := GetCom(base, args)
	if err != nil {
		r.err = err
		return r
	}
	r.out, r.err = com.HandleCom(testServer, NewState())
	return r
}

// start runs a command in a goroutine and returns a channel for its reply.
func start(base string, args ...string) <-chan reply {
	ch := make(chan reply, 1)
	go func() { ch <- run(base, args...) }()
	return ch
}

// wait returns the reply, or fails the test if the command hangs or panics.
func wait(t *testing.T, ch <-chan reply, what string) reply {
	t.Helper()
	select {
	case r := <-ch:
		if r.panicked != nil {
			t.Fatalf("%s: handler panicked: %v", what, r.panicked)
		}
		return r
	case <-time.After(hangLimit):
		t.Fatalf("%s: no reply within %s (hang or deadlock)", what, hangLimit)
		return reply{}
	}
}

// waitForWaiters polls until n waiters are registered on key in the map.
func waitForWaiters(t *testing.T, m map[string][]chan string, key string, n int) {
	t.Helper()
	deadline := time.Now().Add(hangLimit)
	for time.Now().Before(deadline) {
		kmu.RLock()
		got := len(m[key])
		kmu.RUnlock()
		if got >= n {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("expected %d waiter(s) on %q, never registered", n, key)
}

func waiterCount(m map[string][]chan string, key string) int {
	kmu.RLock()
	defer kmu.RUnlock()
	return len(m[key])
}

func keyExists(key string) bool {
	kmu.RLock()
	defer kmu.RUnlock()
	_, ok := keyspace[key]
	return ok
}

var nullList = resp.RESPEncoder(nil, resp.NullBulkList)

// ---------- BLPOP ----------

func TestBLPOP_ReturnsExistingElement(t *testing.T) {
	resetState()
	run("rpush", "k", "x")

	r := wait(t, start("blpop", "k", "0"), "blpop")
	want := "*2\r\n$1\r\nk\r\n$1\r\nx\r\n"
	if string(r.out) != want {
		t.Errorf("got %q, want %q", r.out, want)
	}
}

func TestBLPOP_WakesOnRPush(t *testing.T) {
	resetState()
	ch := start("blpop", "k", "0")
	waitForWaiters(t, blpopWaiters, "k", 1)

	run("rpush", "k", "x")

	r := wait(t, ch, "blpop")
	want := "*2\r\n$1\r\nk\r\n$1\r\nx\r\n"
	if string(r.out) != want {
		t.Errorf("got %q, want %q", r.out, want)
	}
}

func TestBLPOP_WakesOnLPush(t *testing.T) {
	resetState()
	ch := start("blpop", "k", "0")
	waitForWaiters(t, blpopWaiters, "k", 1)

	run("lpush", "k", "x")

	r := wait(t, ch, "blpop")
	want := "*2\r\n$1\r\nk\r\n$1\r\nx\r\n"
	if string(r.out) != want {
		t.Errorf("got %q, want %q", r.out, want)
	}
}

func TestBLPOP_TimeoutReturnsNull(t *testing.T) {
	resetState()
	begin := time.Now()

	r := wait(t, start("blpop", "k", "0.1"), "blpop")

	if !bytes.Equal(r.out, nullList) {
		t.Errorf("got %q, want null list %q", r.out, nullList)
	}
	if el := time.Since(begin); el < 90*time.Millisecond {
		t.Errorf("returned after %s, want about 100ms", el)
	}
}

func TestBLPOP_TimeoutRemovesWaiter(t *testing.T) {
	resetState()
	wait(t, start("blpop", "k", "0.05"), "blpop")

	if n := waiterCount(blpopWaiters, "k"); n != 0 {
		t.Errorf("%d waiter(s) left after timeout, want 0", n)
	}
}

func TestBLPOP_WaitersServedInOrder(t *testing.T) {
	resetState()
	first := start("blpop", "k", "0")
	waitForWaiters(t, blpopWaiters, "k", 1)
	second := start("blpop", "k", "0")
	waitForWaiters(t, blpopWaiters, "k", 2)

	run("rpush", "k", "a", "b")

	r1 := wait(t, first, "first blpop")
	r2 := wait(t, second, "second blpop")
	if !strings.Contains(string(r1.out), "\r\na\r\n") {
		t.Errorf("first waiter got %q, want element a", r1.out)
	}
	if !strings.Contains(string(r2.out), "\r\nb\r\n") {
		t.Errorf("second waiter got %q, want element b", r2.out)
	}
}

func TestBLPOP_ExtraElementsStayInList(t *testing.T) {
	resetState()
	ch := start("blpop", "k", "0")
	waitForWaiters(t, blpopWaiters, "k", 1)

	run("rpush", "k", "a", "b", "c")
	wait(t, ch, "blpop")

	r := run("llen", "k")
	if string(r.out) != ":2\r\n" {
		t.Errorf("llen = %q, want :2", r.out)
	}
}

func TestBLPOP_HandOffLeavesNoEmptyKey(t *testing.T) {
	resetState()
	ch := start("blpop", "k", "0")
	waitForWaiters(t, blpopWaiters, "k", 1)

	run("rpush", "k", "x")
	wait(t, ch, "blpop")

	if keyExists("k") {
		t.Errorf("empty list key still in keyspace; TYPE would report list, Redis reports none")
	}
}

func TestBLPOP_NoPlaceholderWhileWaiting(t *testing.T) {
	resetState()
	start("blpop", "k", "0")
	waitForWaiters(t, blpopWaiters, "k", 1)

	if keyExists("k") {
		t.Errorf("waiting on a missing key created an entry in keyspace")
	}
}

func TestLPush_ReplyCountsPushedElements(t *testing.T) {
	resetState()
	ch := start("blpop", "k", "0")
	waitForWaiters(t, blpopWaiters, "k", 1)

	r := run("lpush", "k", "x")
	wait(t, ch, "blpop")

	if string(r.out) != ":1\r\n" {
		t.Errorf("lpush reply = %q, want :1", r.out)
	}
}

func TestRPush_ReplyCountsPushedElements(t *testing.T) {
	resetState()
	ch := start("blpop", "k", "0")
	waitForWaiters(t, blpopWaiters, "k", 1)

	r := run("rpush", "k", "x")
	wait(t, ch, "blpop")

	if string(r.out) != ":1\r\n" {
		t.Errorf("rpush reply = %q, want :1", r.out)
	}
}

// A push that lands at the same moment as the timeout must neither hang
// nor lose the element: it is either returned or still in the list.
func TestBLPOP_PushAtTimeoutLosesNothing(t *testing.T) {
	for i := 0; i < 50; i++ {
		resetState()
		ch := start("blpop", "k", "0.01")
		waitForWaiters(t, blpopWaiters, "k", 1)
		time.Sleep(10 * time.Millisecond)

		push := start("rpush", "k", "x")
		r := wait(t, ch, fmt.Sprintf("blpop (round %d)", i))
		wait(t, push, fmt.Sprintf("rpush (round %d)", i))

		got := string(r.out) == "*2\r\n$1\r\nk\r\n$1\r\nx\r\n"
		isNull := bytes.Equal(r.out, nullList)
		if !got && !isNull {
			t.Fatalf("round %d: unexpected reply %q", i, r.out)
		}
		left := string(run("llen", "k").out)
		if got && left != ":0\r\n" {
			t.Fatalf("round %d: element returned and still in list (llen %q)", i, left)
		}
		if isNull && left != ":1\r\n" {
			t.Fatalf("round %d: null reply and element lost (llen %q)", i, left)
		}
	}
}

// ---------- XREAD ----------

func TestXREAD_NoBlockNoData(t *testing.T) {
	resetState()
	r := wait(t, start("xread", "streams", "s", "0-0"), "xread")
	if !bytes.Equal(r.out, nullList) {
		t.Errorf("got %q, want null list", r.out)
	}
}

func TestXREAD_ReturnsOnlyEntriesAfterID(t *testing.T) {
	resetState()
	run("xadd", "s", "1-1", "a", "1")

	r := wait(t, start("xread", "streams", "s", "1-1"), "xread")
	if !bytes.Equal(r.out, nullList) {
		t.Errorf("id 1-1 must be exclusive; got %q, want null list", r.out)
	}
}

func TestXREAD_BlockTimeoutReturnsNull(t *testing.T) {
	resetState()
	begin := time.Now()

	r := wait(t, start("xread", "block", "100", "streams", "s", "0-0"), "xread")

	if !bytes.Equal(r.out, nullList) {
		t.Errorf("got %q, want null list", r.out)
	}
	if el := time.Since(begin); el < 90*time.Millisecond || el > time.Second {
		t.Errorf("returned after %s, want about 100ms (BLOCK is milliseconds)", el)
	}
}

func TestXREAD_BlockWakesOnXAdd(t *testing.T) {
	resetState()
	run("xadd", "s", "1-1", "a", "1")
	ch := start("xread", "block", "0", "streams", "s", "1-1")
	waitForWaiters(t, xreadWaiters, "s", 1)

	run("xadd", "s", "2-1", "b", "2")

	r := wait(t, ch, "xread")
	out := string(r.out)
	if !strings.Contains(out, "2-1") || strings.Contains(out, "1-1") {
		t.Errorf("got %q, want only entry 2-1", out)
	}
}

func TestXREAD_BlockOnMissingKeyWakesWhenCreated(t *testing.T) {
	resetState()
	ch := start("xread", "block", "0", "streams", "s", "0-0")
	waitForWaiters(t, xreadWaiters, "s", 1)

	run("xadd", "s", "1-1", "a", "1")

	r := wait(t, ch, "xread")
	if !strings.Contains(string(r.out), "1-1") {
		t.Errorf("got %q, want entry 1-1", r.out)
	}
}

func TestXREAD_BlockNoPlaceholderWhileWaiting(t *testing.T) {
	resetState()
	start("xread", "block", "0", "streams", "s", "0-0")
	waitForWaiters(t, xreadWaiters, "s", 1)

	if keyExists("s") {
		t.Errorf("waiting on a missing key created an entry in keyspace")
	}
}

func TestXREAD_BlockWakesOnAnyOfSeveralKeys(t *testing.T) {
	resetState()
	run("xadd", "a", "1-1", "x", "1")
	ch := start("xread", "block", "0", "streams", "a", "b", "1-1", "0-0")
	waitForWaiters(t, xreadWaiters, "a", 1)
	waitForWaiters(t, xreadWaiters, "b", 1)

	run("xadd", "b", "1-1", "y", "2")

	r := wait(t, ch, "xread")
	out := string(r.out)
	if !strings.Contains(out, "$1\r\nb\r\n") {
		t.Errorf("got %q, want entries for key b", out)
	}
}

// An XADD that wakes the client but is not newer than the client's ID
// must not end the wait.
func TestXREAD_WakeWithoutMatchingEntryKeepsWaiting(t *testing.T) {
	resetState()
	run("xadd", "s", "1-1", "a", "1")
	ch := start("xread", "block", "0", "streams", "s", "5-0")
	waitForWaiters(t, xreadWaiters, "s", 1)

	run("xadd", "s", "2-1", "b", "2") // not > 5-0

	select {
	case r := <-ch:
		t.Fatalf("returned early with %q; entry 2-1 is not after 5-0", r.out)
	case <-time.After(150 * time.Millisecond):
	}

	run("xadd", "s", "6-0", "c", "3")
	r := wait(t, ch, "xread")
	if !strings.Contains(string(r.out), "6-0") {
		t.Errorf("got %q, want entry 6-0", r.out)
	}
}

func TestXREAD_DollarOnExistingKeyOnlyReturnsNewEntries(t *testing.T) {
	resetState()
	run("xadd", "s", "1-1", "a", "1")
	ch := start("xread", "block", "0", "streams", "s", "$")
	waitForWaiters(t, xreadWaiters, "s", 1)

	run("xadd", "s", "2-1", "b", "2")

	r := wait(t, ch, "xread")
	out := string(r.out)
	if !strings.Contains(out, "2-1") || strings.Contains(out, "1-1") {
		t.Errorf("got %q, want only entry 2-1", out)
	}
}

func TestXREAD_DollarOnMissingKey(t *testing.T) {
	resetState()
	ch := start("xread", "block", "0", "streams", "s", "$")
	waitForWaiters(t, xreadWaiters, "s", 1)

	run("xadd", "s", "1-1", "a", "1")

	r := wait(t, ch, "xread")
	if !strings.Contains(string(r.out), "1-1") {
		t.Errorf("got %q, want entry 1-1", r.out)
	}
}

func TestXREAD_ReturnRemovesWaiters(t *testing.T) {
	resetState()
	ch := start("xread", "block", "0", "streams", "a", "b", "0-0", "0-0")
	waitForWaiters(t, xreadWaiters, "a", 1)
	waitForWaiters(t, xreadWaiters, "b", 1)

	run("xadd", "a", "1-1", "x", "1")
	wait(t, ch, "xread")

	for _, k := range []string{"a", "b"} {
		if n := waiterCount(xreadWaiters, k); n != 0 {
			t.Errorf("%d waiter(s) left on %q after return, want 0", n, k)
		}
	}
}

func TestXREAD_TimeoutRemovesWaiters(t *testing.T) {
	resetState()
	wait(t, start("xread", "block", "50", "streams", "s", "0-0"), "xread")

	if n := waiterCount(xreadWaiters, "s"); n != 0 {
		t.Errorf("%d waiter(s) left after timeout, want 0", n)
	}
}

// Many blocked clients and a burst of XADDs: every client must wake
// and the server must still answer afterwards.
func TestXREAD_BurstOfXAddWakesAllWaiters(t *testing.T) {
	resetState()
	run("xadd", "s", "1-1", "a", "1")

	const waiters = 20
	chans := make([]<-chan reply, waiters)
	for i := range chans {
		chans[i] = start("xread", "block", "0", "streams", "s", "$")
	}
	waitForWaiters(t, xreadWaiters, "s", waiters)

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run("xadd", "s", "*", "k", "v")
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(hangLimit):
		t.Fatalf("XADD burst did not finish within %s (deadlock)", hangLimit)
	}

	for i, ch := range chans {
		wait(t, ch, fmt.Sprintf("xread %d", i))
	}
	wait(t, start("ping"), "ping after burst")
}

// Burst of XADDs against one waiter: XADD must never wait for the client.
func TestXAdd_BurstDoesNotBlockOnOneWaiter(t *testing.T) {
	resetState()
	run("xadd", "s", "1-1", "a", "1")
	ch := start("xread", "block", "0", "streams", "s", "$")
	waitForWaiters(t, xreadWaiters, "s", 1)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			run("xadd", "s", "*", "k", "v")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(hangLimit):
		t.Fatalf("50 XADDs did not finish within %s", hangLimit)
	}
	wait(t, ch, "xread")
}
