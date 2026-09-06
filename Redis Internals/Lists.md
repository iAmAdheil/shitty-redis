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

## Sources

- Redis source: `src/t_list.c`, `src/quicklist.c`, `src/listpack.c`
- Matt Stancliff, [Redis Quicklist — Adventures in Encodings](https://matt.sh/redis-quicklist)
- OneUptime, [How Redis Quicklist Data Structure Works](https://oneuptime.com/blog/post/2026-03-31-redis-how-redis-quicklist-data-structure-works/view)
- OneUptime, [How Redis Listpack Data Structure Works](https://oneuptime.com/blog/post/2026-03-31-redis-how-redis-listpack-data-structure-works/view)
