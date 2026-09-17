# Redis Streams: The Underlying Data Structure

A stream stores entries in two layers: a **radix tree** (Redis names it `rax`) and a set of **listpacks**. Each entry has an ID with the form `<ms>-<seq>`. The `ms` part is a millisecond timestamp. The `seq` part is a counter that breaks ties for entries added in the same millisecond.

These two layers solve two different problems:
- The radix tree answers "which listpack holds this ID?" in roughly the number of bytes in the ID, not the number of entries in the stream.
- The listpack packs many entries into one flat, cheap-to-allocate byte array, the same structure documented in [Lists.md](Lists.md).

## Structure, by example

```
XADD mystream * temp 90
XADD mystream * temp 92 hum 45
XADD mystream * temp 91 hum 44
... 98 more XADD calls, all still under one node's byte cap ...
XADD mystream * temp 89 hum 30   (this one overflows the node)
```

```
mystream → Stream { rax, length: 101, last_id: 1700000009000-0, entries_added: 101, cgroups }
              │
              ├─ rax key 1700000000000-0 → listpack Node A (master: 1700000000000-0, ~100 entries)
              └─ rax key 1700000009000-0 → listpack Node B (master: 1700000009000-0, 1 entry so far)
```

- **Key** (`mystream`) lives in the keyspace dict, one level above the stream object — same relationship a list's key has to its quicklist.
- **Stream** — one per key. Tracks `length`, `last_id`, and points at the `rax`.
- **Rax key** — the full ID of the *first* entry stored in that node (the **master entry**), stored as 16 fixed-width bytes so byte order equals numeric order.
- **Rax value** — a pointer to one listpack node holding that master entry plus a run of regular entries after it.

**Where an entry lands:** always the most recently created listpack node (the "open" node), same as a quicklist's tail. Once that node's entry count or byte size crosses `stream-node-max-entries` / `stream-node-max-bytes`, close it and open a new node, keyed by the entry that triggered the overflow. A stream never inserts into the middle of an old, closed node — only appends to the open one — which is what keeps `XADD` fast.

## Every data structure a Stream needs

### 1. The `Stream` object — one per key

Fields, and why each exists:

| Field                  | Purpose                                                                                                                                              |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| `rax`                  | The index described above: ID → listpack node.                                                                                                       |
| `length`               | Count of entries still present (deleted entries don't count). Answers `XLEN` in O(1) instead of walking the whole rax.                               |
| `last_id`              | The most recently assigned ID. `XADD *` and ID-ordering validation both need this without scanning anything.                                         |
| `max_deleted_entry_id` | The highest ID ever removed by `XDEL`/trimming. Exists purely for `XINFO` reporting — it is not needed to make `XADD`/`XRANGE` correct.              |
| `entries_added`        | A monotonically increasing counter of every entry ever added, deleted or not. Used by replication and `XINFO` to detect how much history has passed. |
| `cgroups`              | A second radix tree, one entry per consumer group name → that group's state (see below).                                                             |

### 2. The radix tree (`rax`) — the index

A radix tree (also called a "compressed trie" or "patricia trie") is a tree where a chain of nodes that each have exactly one child gets collapsed into a single edge holding several bytes at once, instead of one byte per edge. Two properties make it the right fit here, and both come from a plain trie, not from anything stream-specific:

- **Lookup and insert cost scale with key length, not with how many keys are stored.** A stream ID is a fixed 16 bytes, so every lookup does a bounded amount of work regardless of whether the stream holds a thousand or a billion entries.
- **Keys come back out in sorted order for free**, because traversal always walks the smaller-byte-value child branch before the larger one. Since IDs are stored big-endian, byte order *is* numeric order — no separate sort step is needed to answer `XRANGE` or find `last_id`.

Compare this to the flat `map[string]map[string]string` you'd reach for first: a hash map answers "does this exact ID exist?" in O(1), but it has no order at all, so a range query (`XRANGE 100 200`) has no better option than scanning every key and checking whether it falls in range. The radix tree exists specifically to make that a targeted walk instead of a full scan.

**What you actually need to build one:** each rax node holds a set of children, indexed by the next byte (or run of bytes, when compressed) of the key. To look up or insert an ID, walk down from the root, consuming bytes of the 16-byte ID as you go, until you either find an existing key or fall off the tree at the point where a new branch needs to be created. This is standard trie mechanics — nothing about streams changes it. Antirez's design notes (linked below) go into the exact node layout Redis uses; for a first implementation, any structure that supports "insert key → value" and "find the largest key ≤ X" satisfies what a stream needs from this layer.

### 3. The listpack node — the payload

This is the exact same byte-packed structure [Lists.md](Lists.md) documents byte-by-byte for quicklist nodes — same header, same tag bytes, same `backlen` trick for walking backward. What's stream-specific is *what* gets packed into it:

- **The master entry** (the node's first entry, the one the rax key points at) stores data shared by the whole node in full: its own complete field names and values, the count of entries in the node, and the count of deleted entries in the node.
- **Every entry after the master** stores only the *difference* from the master:
  - `ms` and `seq` are stored as a delta from the master ID, not as the full 16-byte ID — usually 1-2 bytes instead of 16.
  - If an entry has the exact same field names as the master, a `SAMEFIELDS` flag is set and only the values are stored, not the names again.
  - A flags byte marks whether the entry is deleted (a tombstone — see below).
  - A back-pointer at the end of each entry (the same `backlen` field from the listpack format) lets Redis walk the node backward, which `XREVRANGE` needs.

This is why one listpack node is capped at roughly 100 entries by default (`stream-node-max-entries`): every entry after the master is only cheap to store *because* it can point back at a nearby master's full field data. Left uncapped, a node stays efficient; splitting on a size/count cap is what keeps any single node from growing large enough to make an insert (which may still need to shift bytes, per the listpack format) expensive.

### 4. Tombstones — deletion is lazy

`XDEL` does not shrink the listpack right away. It sets the deleted flag on the entry and increments the node's deleted count. The entry's bytes stay in place until Redis merges or drops that node later. This keeps `XDEL` a fast, bounded operation — find the entry, flip one flag — instead of a byte-shifting removal. The cost is wasted space until cleanup, the same trade `Lists.md`'s lazy-node-drain avoids on the pop path, just deferred further here since a stream (unlike a list) needs the space to stay addressable by ID even after logical deletion, for tools like `XINFO`.

### 5. Consumer groups — the same two-layer pattern, reused

A consumer group (`cgroups` entry) needs its own index of "which entries are still unacknowledged," and Redis builds it out of the exact same two building blocks:

- **The PEL (Pending Entries List)** is a radix tree keyed by entry ID, same as the stream itself. Its value per ID is a small record: which consumer holds it, and when it was delivered.
- **Each consumer** keeps its *own* smaller radix tree — the subset of the group PEL it personally holds — so "what does consumer X still need to ack" doesn't require scanning the whole group's PEL.

Nothing new to design here: it's "radix tree keyed by ID" applied twice more, at the group level and the per-consumer level.

## What exists in this codebase today, and what's still needed

The current implementation (`app/stream.go`, `app/handlers.go`) is a working stand-in for the storage layer above, the same relationship the old `lists map[string]*[]string` had to the eventual quicklist in `Lists.md`:

- `entries *map[string]map[string]string` (ID → field → value) plays the role the rax + listpack layers play together. It gets `XADD`/`XRANGE` correct, just via a full scan instead of a targeted tree walk.
- The ID logic — `splitStreamEntryId`, `getMaxEntry`, `validateStreamEntryId`, `generateStreamEntryId`, `formatBoundXRange` — operates purely on ID strings and comparisons. None of it needs to change when the flat map is replaced by a rax: it doesn't know or care how entries are stored, only how IDs compare and how the next ID gets picked. That logic carries over unchanged.
- What the rax specifically buys you, that the flat map cannot: `getEntriesInRange` today (`app/stream.go`) does `for id, pairs := range *(st.entries)` — a full O(n) walk of every entry in the stream, checking each one against the range. A radix tree replaces that with a seek to the first relevant node, then a bounded walk forward only through entries that matter. Same for `getMaxEntry`, which currently also scans every entry to find the maximum ID; with the ID space held in sorted order, this becomes "read the rightmost key."

## XADD, step by step (structural, not code)

1. **Resolve the ID.** Already implemented: `*` generates a full `ms-seq` ID from the current time; `ms-*` generates just the sequence part for a caller-supplied millisecond. Both need `last_id` (or a scan standing in for it today).
2. **Validate ordering.** The new ID must be strictly greater than `last_id`. Already implemented in `validateStreamEntryId`.
3. **Find the target node.** In the target design: this is always the *open* (most recently created) node — the rax's largest key. Check whether that node's entry count/byte size still has room under `stream-node-max-entries`/`stream-node-max-bytes`.
   - **Room available:** append the new entry to that node's listpack, encoded as a delta from the node's master entry (per the listpack section above).
   - **No room:** close the current node, open a new one, insert a rax key for the new node using the new entry's ID as its master. This new entry becomes a master entry, so it stores its full field data rather than a delta.
4. **Update the `Stream` object.** `length += 1`, `last_id = new id`, `entries_added += 1`.
5. **Reply** with the new entry's ID — this part is already implemented and doesn't change.

## XRANGE, step by step (structural, not code)

1. **Resolve the boundary IDs.** Already implemented in `formatBoundXRange` — an incomplete boundary like `5` expands to `5-0` on the low end and `5-<max seq seen for ms 5>` on the high end.
2. **Seek, don't scan.** Find the rax node whose master ID is the largest ID ≤ the low boundary — that's the first node that can possibly contain a matching entry. This is the step a flat map cannot do without a full scan.
3. **Walk forward, node by node.** Starting from that node, decode entries (master, then each delta) in order. Stop as soon as a decoded ID exceeds the high boundary, or the rax runs out of nodes.
4. **Filter and collect.** Skip tombstoned entries. Everything else in `[low, high]` is already being visited in ID order (storage order matches ID order by construction), so no separate sort step is needed before returning the result.

## Edge Cases

- **Concurrent `XADD` on a brand-new stream key.** `validateStreamEntryId` and `generateStreamEntryId` read the `streams` map under `smu.RLock()`, but creating a *new* `Stream` and inserting it into the `streams` map needs to happen under `smu.Lock()` (a write lock) to be safe against two clients racing to create the same new key at once — whoever's `XADD` runs the create-and-insert step needs exclusive access to that map write, not just to the per-stream data once it exists.
- **Sequence overflow within one millisecond.** If enough entries land in the same millisecond to run the `seq` counter past its max value, ID generation needs a defined behavior (real Redis rolls to the next millisecond) rather than silently wrapping or erroring.
- **A batch of fields larger than a node's byte cap.** Same issue `Lists.md` flags for an oversized list element — one entry's field data can, in principle, exceed `stream-node-max-bytes` by itself, and needs a defined fallback (real Redis still packs it into its own node, cap notwithstanding).
- **Master entry becomes a tombstone.** If `XDEL` removes the *master* entry of a node, later delta-encoded entries in that node still need the master's original field names/ID to decode correctly — the master's bytes stay in place (flagged deleted) rather than being reclaimed immediately, precisely because deltas depend on it.
- **`XRANGE` across the tombstone-then-merge boundary.** Once lazy deletion eventually triggers a node merge or drop, any in-progress range walk holding a reference into the old node layout needs to not have that layout change out from under it — the same "don't hold a stale offset across a mutation" hazard `Lists.md` calls out for `Entries []byte` growth.
- **Consumer group PEL entries outliving their consumer.** If a consumer group's PEL tracks an entry as pending, and the consumer that was tracking it disconnects, the entry needs to stay claimable by another consumer (`XCLAIM`) rather than being lost — the PEL's radix tree is the source of truth for "still unacknowledged," independent of any single consumer's connection state.

## Summary

| Concept | Structure | Status in this codebase |
|---|---|---|
| The stream itself | radix tree (`rax`), keyed by entry ID | flat `map[string]map[string]string`, correct but O(n) range scans |
| One radix tree node's value | a listpack holding ~100 entries | not yet built — entries are stored whole, no master/delta packing |
| Entry storage inside a listpack | one master entry, then delta-encoded, field-deduplicated entries | not yet built |
| A deleted entry | a tombstone flag, removed later during a merge | not yet built — `XDEL` isn't implemented yet |
| Consumer group state | its own radix tree (the PEL), same pattern as the stream | not yet built |
| ID generation & ordering rules | pure ID comparisons, storage-agnostic | implemented, and reusable as-is once the storage layer above changes |

## Sources

- Redis source: `src/t_stream.c`, `src/stream.h`, `src/rax.c`, `src/listpack.c`
- Antirez, [Redis streams as a pure data structure](https://antirez.com/news/128)
- [zpoint/Redis-Internals — streams.md](https://github.com/zpoint/Redis-Internals/blob/5.0/Object/streams/streams.md)
