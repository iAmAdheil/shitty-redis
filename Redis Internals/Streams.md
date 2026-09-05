# Redis Streams: The Underlying Data Structure

A stream stores entries in two layers: a **radix tree** (Redis names it `rax`) and a set of **listpacks**. Each entry has an ID with the form `<ms>-<seq>`. The `ms` part is a millisecond timestamp. The `seq` part is a counter. It breaks ties for entries added in the same millisecond.

## Why not one listpack per entry?

A separate node for each entry would work, but it wastes memory. Every radix tree node has fixed overhead. Redis groups many entries under one radix tree key instead. Each key holds one listpack with up to ~100 entries. Two settings control the limit: `stream-node-max-entries` and `stream-node-max-bytes`. This grouping cuts the per-entry overhead by a large amount.

## The three parts of a stream object

1. **The `stream` struct**, defined in `stream.h`. It holds:
   - `rax *rax` — the radix tree described below.
   - `length` — the count of entries still present. Deleted entries do not count here.
   - `last_id` — the ID of the most recent entry added.
   - `max_deleted_entry_id` and `entries_added` — bookkeeping fields, used by `XINFO` and replication.
   - `cgroups` — a second radix tree, one entry per consumer group.
2. **The radix tree (`rax`)**. Each key is the 16-byte big-endian ID of the *first* entry in a listpack node. Redis calls this first entry the **master entry**. The radix tree lets Redis find the correct listpack node for a given ID quickly, without a scan of every entry. `XRANGE` uses this for fast range scans.
3. **The listpack**. This is a compact, flat byte array, defined in `listpack.c`. One listpack holds one master entry, then a run of regular entries.

## How one listpack node is packed

The master entry stores data shared by the whole node:
- Its own full field names and values.
- The count of items in the node.
- The count of deleted items in the node.

Every entry after the master stores only the **difference** from the master:
- `ms` and `seq` are stored as a **delta** from the master ID, not as the full ID.
- If an entry has the same field names as the master, Redis sets a `SAMEFIELDS` flag. Redis then stores only the values, not the names again.
- A flags byte marks whether the entry is deleted (a **tombstone**, see below).
- A back-pointer at the end of each entry lets Redis walk the listpack backward. `XREVRANGE` needs this.

This design saves memory. The pattern used before Streams existed — a sorted set plus one hash per entry — needs much more space for the same data. Antirez measured about 13 times less memory for one benchmark of one million entries.

## Deletion is lazy

`XDEL` does not shrink the listpack right away. It sets the deleted flag on the entry (a tombstone) and adds one to the node's deleted count. The entry's bytes stay in place until Redis merges or drops that node later. This keeps `XDEL` fast. The cost is some wasted space until cleanup.

## Consumer groups reuse the same pattern

A consumer group (`streamCG`) also stores a radix tree, called the **PEL** (Pending Entries List). The PEL maps each pending entry ID to the consumer that holds it, and the time it was delivered. Each consumer (`streamConsumer`) keeps its own smaller radix tree of the entries it currently holds. The whole feature reuses two building blocks: a radix tree for ordered lookup by ID, and compact listpack records for the payload.

## Summary

| Concept | Structure |
|---|---|
| The stream itself | radix tree (`rax`), keyed by entry ID |
| One radix tree node's value | a listpack holding ~100 entries |
| Entry storage inside a listpack | one master entry, then delta-encoded, field-deduplicated entries |
| A deleted entry | a tombstone flag, removed later during a merge |
| Consumer group state | its own radix tree (the PEL), same pattern as the stream |

## Sources

- Redis source: `src/t_stream.c`, `src/stream.h`, `src/rax.c`, `src/listpack.c`
- Antirez, [Redis streams as a pure data structure](https://antirez.com/news/128)
- [zpoint/Redis-Internals — streams.md](https://github.com/zpoint/Redis-Internals/blob/5.0/Object/streams/streams.md)
