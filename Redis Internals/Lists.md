# Redis Lists: The Underlying Data Structure

A Redis list is a **quicklist**: a doubly linked list of nodes, where each node holds a compact byte array (a **listpack**) of elements — not a chain of one-element-per-pointer nodes.

## Structure, by example

```
RPUSH mylist "apple" "42" "banana" "7" "cherry" "100" "date"
(example node cap: 4 entries — real default is a byte cap, -2 = 8KB)
```

```
mylist → quicklist { count: 7, num_nodes: 2, head, tail }
              │
              ├─ Node A { prev: NULL, next: →B, listpack: [apple, 42, banana, 7] }
              └─ Node B { prev: →A,   next: NULL, listpack: [cherry, 100, date] }
```

- **Key** (`mylist`) lives in the keyspace dict, one level above the quicklist. Not part of the quicklist itself.
- **Quicklist** — one per key. Tracks total `count` and `num_nodes`. Points at `head` and `tail`.
- **Node** — one link in the chain. Holds `prev`/`next` pointers and one listpack. No node-splitting logic beyond a size cap.
- **Listpack** — packed bytes inside a node: `[total bytes][num entries][entry 1]...[entry N][0xFF]`. Each entry is `[encoding+data][backlen]`. Integers get compact encodings (e.g. `42` → 1 byte), not stored as digit strings.

**Where an element lands:** keep appending to the tail node until it hits the cap, then open a new node. `LPUSH` does the same at the head. Never a manual choice — decided purely by remaining room in the current head/tail node.

## Listpack, byte by byte

Example: `"apple", 42, "banana"`.

```
Byte offset:  0        4      6                     14                   22
              [size=22][cnt=3][ entry: apple (7B) ][ entry: 42 (2B) ][ entry: banana (8B) ][0xFF]
```

```
Header:
  bytes 0-3: total size = 22        (whole listpack, in bytes)
  bytes 4-5: entry count = 3

Entry 1 — "apple":
  byte 6:     tag = 6-bit-string, len=5
  bytes 7-11: a p p l e
  byte 12:    backlen = 6            (bytes 6-11 = 6 bytes)

Entry 2 — 42:
  byte 13:    tag = 7-bit-uint, value=42
  byte 14:    backlen = 1

Entry 3 — "banana":
  byte 15:    tag = 6-bit-string, len=6
  bytes 16-21: b a n a n a
  byte 22:    backlen = 7

byte 23:      0xFF  (end marker)
```

One malloc, 24 bytes, holding 3 elements — no pointers inside it.

### All possible tags

The tag's own bits *are* the type — no separate type field. From Redis's source, `src/listpack.c`:

| Tag pattern (binary) | Byte value | Type          | Holds                                       |
| -------------------- | ---------- | ------------- | ------------------------------------------- |
| `0xxxxxxx`           | 0x00–0x7F  | 7-bit uint    | value 0–127, in the tag itself              |
| `10xxxxxx`           | 0x80–0xBF  | 6-bit string  | length 0–63, in the tag itself              |
| `110xxxxx`           | 0xC0–0xDF  | 13-bit int    | 5 bits here + next byte = 13-bit signed int |
| `1110xxxx`           | 0xE0–0xEF  | 12-bit string | 4 bits here + next byte = 12-bit length     |
| `11110000`           | 0xF0       | 32-bit string | length in next 4 bytes                      |
| `11110001`           | 0xF1       | 16-bit int    | value in next 2 bytes                       |
| `11110010`           | 0xF2       | 24-bit int    | value in next 3 bytes                       |
| `11110011`           | 0xF3       | 32-bit int    | value in next 4 bytes                       |
| `11110100`           | 0xF4       | 64-bit int    | value in next 8 bytes                       |
| `11111111`           | 0xFF       | end marker    | not a real entry — marks end of listpack    |

Reading order: check bit 7 first (`0` = small int), else bit 6 (`10` = small string), else bit 5 (`110` = medium int), and so on. Each prefix is unambiguous, so one byte tells Redis exactly how many more bytes to read.

```
42  →  0 0101010
       └┬┘ └──┬──┘
      tag=0  value=42     (1 byte total, value packed into the tag byte)

"apple" (len 5) → 10 000101
                  └┬┘ └─┬──┘
               tag=10  len=5      (1 byte tag, then 5 raw bytes "apple" follow)
```

**Why bother:** a generic per-element object (Go interface value, a boxed struct) forces whatever layout the language runtime imposes — a type header, GC bookkeeping, a pointer to the next one. A hand-rolled byte layout removes all of that, because the code that reads it already knows what shape to expect. You trade "let the runtime decide" for "control every byte," and spend that control on cutting overhead a generic structure can't avoid.

**Listpack is not list-only.** It's a general-purpose packed encoding, reused by:
- **List** — a small list, or one node inside a quicklist.
- **Hash** — a small hash, below `hash-max-listpack-entries`/`-value`.
- **Sorted set** — a small zset, packing member+score pairs.
- **Set** — a small set (Redis 7.2+).
- **Stream** — the block holding entries under one radix-tree node.

Same struct, same byte format. Each caller just decides what values go in.

## Parsing an Entry Back: Bytes to Value

Encoding turns a value into bytes (`getEntry`, `handleIntEntry`, `handleStrEntry`). Decoding is the reverse: given a cursor sitting at the first byte of an entry, recover the original value and know exactly where the next entry starts.

### Step 1 — read the first byte, match it against the tag table

Check the tag byte's leading bits, in the same top-to-bottom order as "All possible tags" above. The first matching pattern wins — the prefixes never overlap, so exactly one row applies:

| Tag pattern | What the decoder does next |
|---|---|
| `0xxxxxxx` | Value is the tag byte itself, 0–127. No more bytes to read for the value. |
| `10xxxxxx` | String length = tag `& 0x3F` (the low 6 bits). Read that many raw bytes right after the tag — that's the string. |
| `110xxxxx` | Read 1 more byte. Combine the tag's low 5 bits (high bits of the value) with the next byte (low 8 bits) into a 13-bit value, then sign-extend from bit 12. |
| `1110xxxx` | Read 1 more byte. String length = (tag `& 0x0F`) shifted left 8, OR'd with that byte — a 12-bit length. Read that many raw bytes as the string. |
| `0xF0` | Read the next 4 bytes as a `uint32` string length. Read that many raw bytes as the string. |
| `0xF1` | Read the next 2 bytes, sign-extend as a 16-bit int. |
| `0xF2` | Read the next 3 bytes, sign-extend as a 24-bit int. |
| `0xF3` | Read the next 4 bytes, sign-extend as a 32-bit int. |
| `0xF4` | Read the next 8 bytes, sign-extend as a 64-bit int. |
| `0xFF` | Not an entry — this is the end marker. Stop walking the listpack. |

### Step 2 — turn the raw bytes into a Go value

- **String tags** (`10xxxxxx`, `1110xxxx`, `0xF0`): the bytes just read *are* the string — no further conversion.
- **Int tags** (everything else, except the 7-bit case which needs none): the bytes are a big-endian two's-complement integer, narrower than a normal Go `int`. Sign-extend it — copy the sign bit outward — before widening to `int64`, or a negative 13-bit value reads back as a large positive number instead of a negative one.
- Whether the original command argument was typed as a string or a number, the RESP reply is always a bulk string — `LRANGE`/`LINDEX` convert the decoded int back to its decimal string form (e.g. `strconv.FormatInt`) before sending it to the client.

### Step 3 — read backlen, advance the cursor

After the tag and its data, one or more `backlen` bytes record the entry's own total length (tag + data, not counting `backlen` itself).

- **Walking forward** (head to tail): you don't need `backlen` at all — the tag already told you how many data bytes follow, so the next entry starts right after `backlen`.
- **Walking backward** (tail to head, e.g. for `RPOP` or a right-anchored `LRANGE`): `backlen` is exactly what makes this possible. Read the byte(s) immediately before the current entry — that's the *previous* entry's `backlen` — and step back that many bytes to land on the previous entry's tag byte.

This backward-walk trick is the reason `backlen` exists at all — without it, a listpack could only be read start-to-end, never from the tail inward.

### Worked example: decoding entry 2 from the earlier walkthrough

```
byte 13:    tag = 0x2A   (0 0101010 → top bit 0 → 7-bit uint)
```

- Tag pattern `0xxxxxxx` matches. Value = `0x2A` = 42, decoded directly from the tag byte. No extra data bytes to read.
- `byte 14: backlen = 1` — confirms the whole entry (tag only) was 1 byte, matching what was just read.
- Next entry starts at byte 15.

### Loop until `0xFF`

Decoding a whole listpack into a slice of values repeats Steps 1–3 from the first entry (byte 6, right after the 6-byte header) until the tag byte read is `0xFF` — that's the signal to stop, not a normal entry to decode.

## Performance vs. a naive linked list

Naive = CS101 doubly linked list, one `struct listNode` (prev/next/value pointers) + one boxed value per element.

| | Naive linked list | Quicklist |
|---|---|---|
| Per-element overhead | 40+ bytes (3 pointers + object wrapper + string header) | 1–10 bytes (encoding + backlen inside a listpack) |
| Memory locality | Scattered heap allocations, one per element | Packed contiguous bytes per node — cache-friendly |
| `LPUSH`/`RPUSH` | O(1) | O(1) — same |
| `LPOP`/`RPOP` | O(1) | O(1) — same |
| `LINDEX`/`LRANGE` walk | O(N), pointer-chasing across scattered memory | O(N), but sequential reads within a node — much faster in practice |
| Measured memory (1M ints per list, jemalloc)* | 11.86 GB | 0.3–1.0 GB depending on node size |

*Source: Matt Stancliff's original quicklist benchmark, [matt.sh/redis-quicklist](https://matt.sh/redis-quicklist).

Big-O is the same for push/pop on both. The win is memory (10x+) and cache locality, not asymptotic complexity.

## Pushing an Element: RPUSH and LPUSH

A push does three things: find the target node, fit the new entry into that node's listpack, and update the quicklist's bookkeeping.

### Step 1 — pick the target node

- `RPUSH` targets the **tail** node.
- `LPUSH` targets the **head** node.
- If the quicklist has no nodes yet (a brand new key), create one node with an empty listpack (header + `0xFF` only). This node becomes both head and tail.

### Step 2 — encode the entry

Build the entry's bytes the same way as any listpack entry: pick a tag from the parse rules above (integer if the value parses as one, else string), write the tag plus data, then append the backlen byte(s). This is the encoding step already shown in "Listpack, byte by byte."

### Step 3 — check room in the target node

Compare the node's current listpack size plus the new entry's size against the node's byte cap (default `-2` = 8KB).

- **Entry fits** — splice the entry into the node's listpack:
  - `RPUSH`: insert the entry bytes just before the `0xFF` end marker.
  - `LPUSH`: insert the entry bytes right after the 6-byte header, before the first existing entry.
  - Update that listpack's `Size` and `Count` header fields.
- **Entry does not fit** — open a new node:
  - `RPUSH`: allocate a node after the current tail, link it in (`tail.Next = newNode`, `newNode.Prev = tail`), make it the new tail.
  - `LPUSH`: allocate a node before the current head, link it in, make it the new head.
  - Put the entry into the new node's (otherwise empty) listpack.
  - `NumNodes += 1`.

### Step 4 — update the quicklist

- `Count += 1` for each element pushed.
- `Head`/`Tail` pointers change only when step 3 opened a new node at that end.
- The command reply is the quicklist's `Count` after all values in the command are pushed.

### Multiple values in one command

`RPUSH key a b c` and `LPUSH key a b c` push one element at a time, in argument order, repeating steps 1–4 for each value. A mid-command push can still trigger a new node if the current node fills up partway through the batch — each value re-checks room independently.

`LPUSH key a b c` inserts `a` at the head first, then `b` at the new head, then `c` at the newest head. The result has `c` closest to the head — the reverse of argument order, because each element is inserted at the front in turn.

## Popping an Element: LPOP and RPOP

A pop is a push in reverse, with one extra wrinkle: it can shrink a node down to nothing, which means the quicklist may need to drop a node — something a push never has to do.

### Step 1 — pick the source node

- `LPOP` targets the **head** node.
- `RPOP` targets the **tail** node.
- If the quicklist has no nodes (`NumNodes == 0`), there is nothing to pop — return nil, no further steps.

### Step 2 — decode the target entry

- `LPOP`: the entry starts right after the listpack header — the same starting point `Read`/`LRANGE` uses for offset 0.
- `RPOP`: the entry sits right before the `0xFF` end marker. Find it via the backward-walk trick from "Parsing an Entry Back": read the `backlen` byte(s) immediately before `0xFF`, step back that many bytes, and land on the last entry's tag.
- Decode it the same way as any entry (tag → value), but for a pop this decode step is doing two jobs at once: recovering the **value** for the command's reply, and recovering the entry's **total byte length** (tag + data + `backlen`) — the only way to know how many bytes to cut out in the next step. A listpack has no notion of "element 0"; it only has bytes, so you cannot remove "one element" without first finding out how many bytes it occupies.

### Step 3 — splice the entry out, update the listpack header

Let `n` be the decoded entry's total byte length.

- `LPOP`: new `Entries` = old `Entries[n:]` — drop the first `n` bytes, keep the rest (including the trailing `0xFF`) unchanged.
- `RPOP`: new `Entries` = old `Entries[:start]` with `0xFF` appended back, where `start` is the byte offset Step 2 walked back to.
- Either way, update that listpack's `Size` (`-n`) and `Count` (`-1`) header fields.
- Nothing about the *surviving* entries needs to change. Each entry's `backlen` describes only itself, not its neighbors, so the remaining bytes are correct as-is — just sitting at a new offset within the array.

### Step 4 — update the quicklist

- `Count -= 1` on the quicklist.
- Check whether the source node's listpack `Count` just reached 0. If it did, that node is now empty and gets unlinked:
  - `LPOP`: `Head = Head.Next`; if the new `Head` is not nil, set `Head.Prev = nil`; if it is nil (the list just became empty), set `Tail = nil` too.
  - `RPOP`: mirror at the tail — `Tail = Tail.Prev`; if not nil, `Tail.Next = nil`; else `Head = nil` too.
  - `NumNodes -= 1`.
- The reply is the popped value, decoded back to its string form the same way `LRANGE` does (e.g. `strconv.FormatInt` for an integer entry).

### `LPOP key count` — popping more than one

Repeat Steps 1–4 up to `count` times, or until the list runs out of elements, whichever comes first. Collect each popped value in pop order for the RESP array reply. If the source node empties and gets unlinked partway through, the next iteration just re-reads `Head` (or `Tail`) — it does not need to know a node boundary was crossed; Step 1 already picks whatever node is current.

## Blocking Pop: BLPOP

> **Prior art:** the wait/wake mechanism below (buffered channel per waiter, `select` against a timeout, re-check the wait queue under the lock to resolve races) is already implemented and working in `app/handlers.go`/`app/utils.go`, against an older `lists map[string]*[]string` representation that predates the quicklist. It's proven — the design here is that same mechanism, re-pointed at `List`/`Node`/`Listpack` instead of the old map. Nothing about the concurrency pattern needs to change, only what it synchronizes access to.

`LPOP` on an empty list returns immediately with nothing. `BLPOP` is not allowed to give up that easily — if the list is empty, it must wait, for up to `timeout` seconds, for some other client to push a value.

### What has to exist first

None of this is wired up yet — `List`/`Node`/`Listpack` currently only exists as a standalone package exercised by tests, with nothing in `com.go`/`handlers.go` holding an instance of it. Before `BLPOP` (or any list command) can run against it, there needs to be:
- a per-key registry, e.g. `map[string]*list.List`, plus a mutex guarding it — the `List` equivalent of the old `lists`/`lmu`.
- a per-key waiter registry, e.g. `map[string][]chan string` — the `List` equivalent of the old `listch`. This one doesn't care what backs the list; it can be copied over unchanged.

### Step 1 — data already there: no blocking needed

Under the registry's lock, look up the key's `*List`. If it exists and `Count > 0`, this is a plain pop: call `list.LPOP(1)`, unlock, reply immediately. Same shape as a normal `LPOP`, just reached through a different command.

### Step 2 — nothing there: register to wait

If the key has no `*List` yet, or `Count == 0`, the client doesn't get a reply yet. Instead, still under the lock:

- Create a channel: `ch := make(chan string, 1)`.
- Append it to `listch[key]` — a per-key slice of channels, one per waiting client, in registration order.
- Unlock and move on to waiting.

**Why buffered with capacity 1, not unbuffered:** whichever client later pushes a value has to hand it to this channel without blocking — it's doing that send while holding the lock, so it cannot afford to wait around for this goroutine to be scheduled and ready to receive. A 1-slot buffer means the send always succeeds immediately and the pusher moves on, regardless of whether the waiter's `select` has even started running yet.

### Step 3 — wait for a value or a timeout, whichever comes first

```go
select {
case pop := <-ch:
    // someone pushed — done
case <-expch:
    // timeout fired first
}
```

`expch` comes from `time.After(duration)` when `timeout > 0`. When `timeout` is `0` (or negative), `expch` is never assigned, so it keeps its zero value: a nil channel. A receive on a nil channel never completes, so that `case` in the `select` simply never fires — "block forever" falls out of the zero value, with no separate `if timeout == 0` branch needed.

### Step 4 — the producer side: push first, then pop-and-feed

Every `RPUSH`/`LPUSH` call, after it finishes updating the list (`Count`, `NumNodes`, the listpack bytes — all of it, exactly as if no one were waiting), checks the waiter queue for that key and walks it in registration order — a FIFO queue. For as long as `list.Count > 0` and there are unserved waiters: call `list.LPOP(1)` to take the value at the head, send it into that waiter's channel, and count it as served. Stop early once the list runs dry. Afterward, trim the served waiters off the front of the slice, since they were served in the same front-to-back order they were stored — first client to call `BLPOP` is the first one woken.

**Why push into the list first, instead of handing a pushed value straight to a waiter's channel and skipping the list entirely:**

- **It reuses `List.LPOP` as-is.** `LPOP` already gets the hard parts right — listpack byte-splicing, node-draining, unlinking an emptied head node. A "straight to the channel" path would need a second, divergent way to decide and remove "the next value," largely duplicating what `LPOP` already does correctly.
- **It gets ordering right for free.** A waiter must receive whatever value would actually be at the head of the list after the push lands — the same value a plain `LPOP` would return — regardless of whether the push was `RPUSH` (tail) or `LPUSH` (head). Storing then popping from the head reuses that logic instead of re-deriving "what's the head now" separately for `RPUSH` versus `LPUSH`.
- **It handles a batch with mixed fates without extra bookkeeping.** `RPUSH key a b c` might satisfy zero, one, or all three waiters while the remainder stays queued. Push everything, then pop once per satisfied waiter — no need to decide, per value, before it's even inserted, whether it's "for a waiter" or "for the list."
- The cost is real but small: a value handed to a waiter gets encoded into listpack bytes and immediately decoded back out. That only happens when a client is already blocked and waiting, not on a hot path, so it isn't worth optimizing away.

### Step 5 — resolving the push-vs-timeout race

The subtle part: what if the timeout and a push happen at almost the same moment? The `select`'s timeout branch re-acquires the lock and checks whether this waiter's channel is *still* sitting in `listch[key]`:

- **Still there** — no push ever claimed it. Remove it from the queue and reply with a null array: genuinely timed out.
- **Not there** — some push already removed it from the queue, which only happens in the same step where it sends the value via `LPOP`. A value is therefore guaranteed to already be sitting in the buffered channel, so read it (`<-ch`, which returns instantly — no blocking) and reply with that value instead of timing out.

Both the producer step and this recheck run under the same lock, so "did anyone claim this waiter" is answered atomically with respect to any concurrent push. No lost wakeups, and no value ever gets handed to two different waiters.

### Scope, for now

Real `BLPOP key1 key2 ... timeout` waits on several keys at once and returns from whichever becomes ready first. Supporting only one key at a time is the deliberate scope right now, not a gap to chase yet — multi-key would mean registering the same waiter's channel under every key named in the call, and cleaning it up from all of them once any one fires, on top of everything above.

## Edge Cases

- **Head insert shifts bytes; tail insert does not.** A listpack is one flat byte array. Inserting at the tail is a plain append. Inserting at the head means moving every existing byte in that listpack forward to make room after the header. This cost is bounded by the node's size cap, so it stays inside the "O(1), amortized" claim in the performance table above, but it is not free the way a head insert in a real linked list would be.
- **An entry larger than the node's byte cap.** A single large value (say, a multi-megabyte string) can exceed the node cap by itself. Real Redis handles this with a "plain" node that holds exactly one oversized element outside the normal listpack packing. Decide up front whether to support this case or assume all values stay under the cap.
- **A batch push spans a node boundary.** `RPUSH key v1 v2 v3` where `v1` and `v2` fit in the current tail node but `v3` does not: `v3` needs a new node mid-command, and `NumNodes`/`Tail` must update partway through processing a single command.
- **Integer vs. string encoding changes size, not identity.** `RPUSH key 42` stores `42` as a 1-byte integer entry, not as the 2-byte string `"42"`. `LRANGE`/`LINDEX` must decode the tag and convert back to the string `"42"` for the client — the RESP reply is always a bulk string, regardless of how the entry is packed internally.
- **`Count` is a 2-byte field per listpack.** As written, `Listpack.Count` is `[2]byte`, so one listpack maxes out at 65535 entries before the count field itself overflows. The byte cap (8KB default) triggers a new node long before this limit in most cases, but a node cap set very high, or entries small enough (1-byte integers), could reach it.
- **`backlen` is not always 1 byte.** The worked example above uses entries small enough that `backlen` fits in 1 byte. A longer entry needs more backlen bytes to record its own length. Decoding (walking the listpack backward) must handle a variable-width `backlen`, not assume a fixed 1 byte per entry.
- **Growing `Entries []byte` can move the backing array.** Any code that holds a raw offset or sub-slice into a node's `Entries` before an insert must not reuse it after the insert — a `append`-driven grow can reallocate and copy, invalidating old pointers/offsets into the old array.
- **Concurrent push and pop on the same key.** Splicing bytes into a listpack, and updating `Count`/`NumNodes`/`Head`/`Tail`, are multiple separate writes. Two goroutines pushing to the same key at once need a lock around the whole per-key sequence, not just around each individual field write.

## Sources

- Redis source: `src/t_list.c`, `src/quicklist.c`, `src/listpack.c`
- Matt Stancliff, [Redis Quicklist — Adventures in Encodings](https://matt.sh/redis-quicklist)
- OneUptime, [How Redis Quicklist Data Structure Works](https://oneuptime.com/blog/post/2026-03-31-redis-how-redis-quicklist-data-structure-works/view)
- OneUptime, [How Redis Listpack Data Structure Works](https://oneuptime.com/blog/post/2026-03-31-redis-how-redis-listpack-data-structure-works/view)
