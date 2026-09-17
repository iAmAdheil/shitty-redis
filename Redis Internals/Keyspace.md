# The Keyspace: One Map, or Three?

Every Redis key lives somewhere. Today, "somewhere" means three separate, unrelated Go maps, one per type. This note covers what exists now, why that's getting cramped, and the unified design being considered to replace it.

## What exists today

```go
var vars    = make(map[string]string)         // strings
var vmu     sync.RWMutex

var lists   = make(map[string]*[]string)      // lists (older, naive representation)
var listch  = make(map[string][]chan string)   // BLPOP waiters, kept in a separate map
var lmu     sync.RWMutex

var streams = make(map[string]*Stream)        // streams
var smu     sync.RWMutex
```

- **Strings** (`vars`) are the trivial case: a key either maps to a string value, or the key isn't in the map. `GET` reads `vars[key]` directly; `SET` writes `vars[key] = val` directly. No wrapper struct, no internal structure — unlike `List`, there's no listpack-style byte layout underneath a string value.
- **Lists** (`lists`) currently use the older `map[string]*[]string` representation (the `List`/`Node`/`Listpack` quicklist described in `Lists.md` is the newer structure being built to replace this, not yet wired into the command handlers).
- **Streams** (`streams`) have their own map and their own mutex.
- **TTL** exists only for strings today (`SetupExpiry`/`Expire` in `app/utils.go`), because it was built against `vars` specifically — a list or stream key can't currently expire.

## The problem with three maps

- **`TYPE key` and `WRONGTYPE` checks need three lookups.** `handleType()` checks `vars`, then `streams`, in sequence, to figure out what a key is (see `app/handlers.go`). Every new type added means one more map to check, in every place that needs to know "does this key exist, and as what."
- **The list/waiter split can drift.** `lists` and `listch` are two maps, kept in sync only by convention. Nothing stops a key from existing in one but not the other.
- **TTL doesn't generalize.** Because expiry is wired directly against `vars`, giving a list or stream a TTL means duplicating the expiry mechanism per map, not writing it once.

## The proposed design: one map, a type tag, a listener slice

```go
type DataType int

const (
	TypeString DataType = iota
	TypeList
	TypeStream
)

type Entry struct {
	Type      DataType
	Data      any            // string | *list.List | *stream.Stream
	Listeners []chan string  // waiters blocked on this key (BLPOP, XREAD BLOCK)
}

var keyspace = make(map[string]*Entry)
var kmu sync.RWMutex
```

One map, one mutex, for every type. A command handler checks the tag once, right after the lookup, then type-asserts to the concrete type it expects:

```go
entry, ok := keyspace[key]
if ok && entry.Type != TypeList {
	return WrongTypeReply
}
var l *list.List
if ok {
	l = entry.Data.(*list.List)
}
```

### Why not a shared interface across types

`LPUSH`/`LRANGE` and `XADD`/`XREAD` share essentially no behavior — a common interface across all data types would end up either empty (a marker interface, no real methods) or bloated (most types leaving most methods unimplemented). That's not what solves this. What's needed is a **tagged union**: one field says which type this is, a second field holds the type-specific data as `any`. Each command handler already knows which concrete type it expects; it just needs the tag check and a type assertion, not a shared interface.

### Why `Listeners` lives on `Entry`, not in a separate map

Folding `Listeners` into the same struct as `Data` — rather than keeping a second `map[string][]chan string` alongside it — removes the drift risk described above: the data and its waiters can't fall out of sync, because they're not two separate pieces of state to keep in sync.

**Plain slice, not `*[]chan string`.** Since `keyspace` stores `*Entry` (a pointer), `entry.Listeners = append(entry.Listeners, ch)` already mutates the real struct in place — no double indirection needed. A `nil` slice already means "no listeners," for free, as the zero value: `len(nil) == 0`, ranging over it does nothing, and `append(nil, ch)` allocates on first use exactly as wanted. A `*[]chan string` would only double the nil-checking (`entry.Listeners != nil && len(*entry.Listeners) > 0`) without solving a problem that still exists.

A string-typed `Entry` simply never touches `Listeners` — nothing in Redis blocks waiting on a string key, so the field just sits at its zero value for every string key. The cost of that unused field (an always-nil slice header, 24 bytes) is not worth avoiding with a second, type-specific struct per type.

## When a key actually gets removed

Real Redis removes a key in five situations: `DEL`/`UNLINK`, TTL expiry (lazy on access, or an active background sweep), `FLUSHDB`/`FLUSHALL`, memory-pressure eviction (not relevant at this project's scale yet) — and one rule that's easy to miss:

> **Draining an aggregate type to empty deletes the key.** If the last element leaves a list, hash, set, or sorted set, Redis deletes the key entirely. `RPUSH k a; LPOP k; EXISTS k` returns `0` — an empty list is not a thing that persists in the keyspace.

That rule is the reason `Listeners` living on the same `Entry` needs a specific rule of its own, once a list drains to empty:

| # | `Count` | `Listeners` | Map entry | What a client sees | When |
|---|---|---|---|---|---|
| 1 | > 0 | 0 | exists | Normal key. `EXISTS`=1. | Ordinary push, nobody waiting. |
| 2 | 0 | 0 | **deleted** | `EXISTS`=0. | Last element popped; no one was blocked on this key. |
| 3 | 0 | > 0 | **kept, but hidden** | Must still act like case 2 — `EXISTS`=0, `TYPE`=none — even though the entry is still allocated. | Last element popped, but a `BLPOP` client is still parked waiting for the next push. |
| 4 | > 0 | > 0 | — transient only | — | Only exists mid-push, before the pop-and-feed step (see `Lists.md`) finishes; never at rest. |

Case 3 is the one that needs deliberate handling: every *read* command (`EXISTS`, `TYPE`, `LRANGE`, …) must check `Count > 0`, not just "does the map entry exist," before answering. The entry can be technically present in `keyspace` while still being logically absent to every client but the ones already parked waiting on it.

### Exceptions to the drain-to-empty rule

- **Strings never drain.** A string holds a value, or the key is absent — there's no in-between "present but empty" state the way a collection has. `""` is a valid, distinct value from the key not existing at all.
- **Streams never auto-delete on empty**, even with every entry `XDEL`'d — a stream key is exempt from the rule real lists/hashes/sets/zsets follow, because stream metadata (consumer groups, last-delivered ID) is worth keeping regardless of entry count.

## What this means for `vars` specifically

Under the unified design, a `SET`/`GET` key becomes `Entry{Type: TypeString, Data: "the value"}` — `Data` holds the string directly (not a pointer, not a wrapper), `Listeners` stays permanently unused, and none of the case-3 "kept but hidden" logic ever applies to it. It's the simplest instance of the general shape, not a special case requiring its own rules — which is exactly why today's plain `map[string]string` already works fine for it. The only thing folding it into `keyspace` actually changes for strings is where the TTL logic lives: instead of `SetupExpiry`/`Expire` reaching into `vars` specifically, a generic "delete this key" works for a string, a list, or a stream, without three copies of the same expiry code.

## Sources

- Redis source: `src/db.c` (key expiry, `dbAdd`/`dbDelete`), `src/t_list.c`/`src/t_hash.c`/`src/t_set.c`/`src/t_zset.c` (delete-on-empty for aggregate types), `src/t_stream.c` (the exception)
