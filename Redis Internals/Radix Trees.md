# Radix Trees: Building the Stream Index

[Streams.md](Streams.md) names the two layers a stream needs: a radix tree (`rax`) for the ID index, and a listpack for the payload. That note treats the rax as a known quantity and moves on. This note stays on the rax itself — the part this codebase has started (`app/structures/radix/radix.go`) but not finished.

This note does not write the insert/lookup code for you. It maps out the concept, the case analysis an insert must handle, and how each piece connects to the stub already in `radix.go`, so the remaining work is choosing and typing the logic, not discovering what the logic needs to do.

## 1. From a plain trie to a radix tree

A plain trie stores one byte per edge. To store a 16-byte stream ID, a plain trie walks 16 levels deep, one byte at a time, even when a whole run of those bytes is identical across every key in the tree — which happens often here, since stream IDs are millisecond timestamps that rise together.

A radix tree (also called a **compressed trie** or **Patricia trie**) collapses any chain of single-child nodes into one edge holding several bytes at once. Two keys that share a 6-byte prefix still cost 6 bytes of storage for that shared span, but only one comparison step to walk it, not six.

## 2. The four fields in `RaxNode`, and the rule that ties them together

```go
type RaxNode struct {
	Prefix   string
	Value    *listpack.Listpack
	HasValue bool
	Children map[byte]*RaxNode
}
```

| Field | Holds |
|---|---|
| `Prefix` | The bytes this node's incoming edge represents. Not the whole key from the root — only the span from the parent node to here. |
| `Value` | The listpack for this stream node, when this `RaxNode` is a stored key (the master entry's node). |
| `HasValue` | Whether `Value` is meaningful. A node can exist purely as a branch point with no stored key of its own. |
| `Children` | The next byte of every key that continues past this node, mapped to the child node that owns the rest of that key. |

One rule holds everywhere in the tree: **`Children[b]` always points at a node whose `Prefix` starts with byte `b`.** The map key is not an arbitrary label — it is always the first byte of the child's own `Prefix`. Every walk, insert, and split in this note leans on that rule.

## 3. What a stream needs this structure to do

Per [Streams.md](Streams.md)'s `XADD`/`XRANGE` walkthroughs, a stream's rax needs exactly four operations:

| Operation | Used by | What it returns |
|---|---|---|
| Insert(key, value) | `XADD`, when a node closes and a new one opens | Adds or updates one key |
| Exact Get(key) | Mostly a building block for the other three | The value at that exact key, if the key exists |
| Largest key overall | `XADD`, to find the open (tail) node | The rightmost value in the tree |
| Largest key ≤ X | `XRANGE`'s low boundary | The first node that can hold a matching entry |

A fifth capability, ordered forward walk from a starting key, is not a separate operation — it falls out of the tree shape once children are visited in ascending byte order (Section 8 covers the catch here).

## 4. Insertion into a compressed trie: the case analysis

This is the part a plain trie does not need and a compressed trie cannot avoid: inserting a key can require **splitting** an existing node's `Prefix` partway through.

At each step, you hold a current node and a remaining key (the part of the full key not yet consumed by the walk so far). Compute the **longest common prefix** (LCP) between the current node's `Prefix` and the remaining key. The LCP length, compared against both lengths, picks one of five cases:

1. **LCP equals both lengths — exact match.** The remaining key equals this node's `Prefix` exactly. Set `Value` and `HasValue = true` on this node. No structural change.
2. **LCP equals the node's `Prefix` length, remaining key is longer.** This node's whole prefix is consumed and the key continues. Look up `Children[nextByte]` on what is left.
   - A matching child exists → recurse into it with the leftover key.
   - No matching child → create one new leaf node holding the leftover key as its `Prefix`, `HasValue = true`, and attach it under that byte.
3. **LCP equals the remaining key's length, node's `Prefix` is longer.** The new key ends in the middle of this node's prefix. A split is required: a new node takes the shared LCP bytes and holds the value; the old node shrinks to the leftover suffix and becomes its child.
4. **LCP is shorter than both.** The two keys diverge partway through this node's prefix. A split is required: a new node takes the shared LCP bytes (no value of its own); the old node shrinks to its own leftover suffix and becomes one child; a fresh leaf for the new key's leftover suffix becomes the other child.
5. **The tree is empty.** No node exists yet at all — this is the base case that starts everything else.

Cases 3 and 4 both replace a node with a new one and re-attach the old node underneath it. That means an insert has to be able to **change what the parent points at**, not only mutate the node it is standing on. Plan for this before writing the walk: either the recursive step returns the (possibly new) node and the caller re-assigns it into its own `Children` map, or the walk tracks the parent node and the byte key alongside the current node the whole way down.

## 5. A simplification specific to streams: every key is the same length

The case analysis above is the general shape of radix-tree insertion, for keys of any length. Stream IDs are not general-purpose keys — every one of them encodes to exactly 16 bytes (an 8-byte `ms` plus an 8-byte `seq`, both big-endian, per `Id` in `app/structures/stream/stream.go`).

Fixed-length keys remove two of the five cases. Follow the consequence through: at any node in the tree, every key that could possibly reach it has consumed the exact same number of bytes to get there (the walk only descends by matching bytes exactly). So the remaining-key length and the node's `Prefix` length are **always equal** at every node except the root — meaning **Case 2 and Case 3 above can only ever fire at the very first step, against the root's own empty `Prefix`,** and never again deeper in the tree. Every insertion below the root is either an exact match (Case 1 — a duplicate ID, which should not happen if `validateStreamEntryId` ran first) or a genuine split (Case 4).

This gives the stream's rax a clean shape: a node either has `HasValue = true` and zero children (a full 16-byte key, nothing can extend past it), or `HasValue = false` and two or more children (a pure branch point). No node in this tree ever needs to hold both a value and children at once — that mixed case only arises when one stored key is a strict prefix of another, which fixed-length keys make impossible.

## 6. Worked example, byte by byte

Toy keys, to keep the diagram readable — read `"car"` as standing in for a fixed-length byte string, the same way a real key stands in for 16 ID bytes.

**Insert `"car"` → 99.** Tree is empty (Section 4, case 5). Root's `Prefix` is `""`, so the very first step is the one-time root exception from Section 5: `Children['c']` does not exist yet, so a new leaf is created directly.

```
root
 └─ 'c' → Prefix="car", HasValue=true, Value=99
```

**Insert `"cat"` → 100.** At root, `Children['c']` exists, so the walk descends into it with remaining key `"cat"`. At that node, `Prefix="car"` and remaining `"cat"` share LCP `"ca"` (length 2), shorter than both lengths (3 and 3) — Case 4, split:

```
root
 └─ 'c' → Prefix="ca", HasValue=false
           ├─ 'r' → Prefix="r", HasValue=true, Value=99   (full key "car")
           └─ 't' → Prefix="t", HasValue=true, Value=100  (full key "cat")
```

**Insert `"dog"` → 101.** At root, `Children['d']` does not exist — a fresh leaf attaches straight to the root, as a sibling of the `'c'` branch.

**Insert `"care"` → 102.** Descend `root → 'c' node ("ca")`. Here `Prefix="ca"` is fully consumed by the remaining key `"care"` (LCP 2 = `len(Prefix)`, remaining still has `"re"` left) — this is Case 2 from Section 4, continuing with `Children['r']`. That child exists (`Prefix="r"`, the `"car"` node) — descend again, remaining now `"re"`. There, `Prefix="r"` is again fully consumed (LCP 1 = `len(Prefix)`), remaining has `"e"` left, and `Children['e']` does not exist on that node — a new leaf is created.

```
root
 └─ 'c' → Prefix="ca", HasValue=false
           ├─ 'r' → Prefix="r", HasValue=true, Value=99   (full key "car")
           │         └─ 'e' → Prefix="e", HasValue=true, Value=102  (full key "care")
           └─ 't' → Prefix="t", HasValue=true, Value=100  (full key "cat")
```

Notice the `"car"` node now holds **both** a value and a child — exactly the mixed case Section 5 said cannot happen for stream IDs. It only happened here because `"car"` is a strict prefix of `"care"`, which needs keys of different lengths. Swap these toy strings for 16-byte fixed IDs and this particular shape cannot occur — every stream-rax node is cleanly a leaf-with-value or a branch-with-no-value, never both.

## 7. Finding the open node and the range boundary node

**Largest key overall** (the `XADD` open node): starting at the root, repeatedly step into the child keyed by the largest byte present in `Children`, until a node with no children is reached. Per Section 5, that node is guaranteed to have `HasValue = true` — there is nothing to check along the way except the empty-tree base case.

**Largest key ≤ X** (the `XRANGE` low boundary): walk down matching `X`'s bytes against each node's `Prefix` for as long as they agree.

- If the walk matches `X` all the way to an exact leaf, that leaf is the answer (equal counts as "≤").
- If at some node `X`'s next byte has no matching child, look among that node's children for the **largest byte strictly less than** `X`'s next byte.
  - Found one → the answer is the largest key anywhere in that child's subtree (the same rightmost walk as above, run from that child).
  - Found none → back up to the parent and repeat the same search one level higher, comparing against the byte that led to the current node.
- Reaching the root with nothing found means `X` is smaller than every key in the tree — no predecessor exists.

## 8. Ordered traversal, and a gotcha worth knowing before it bites

[Streams.md](Streams.md) states that a rax returns keys "in sorted order for free," because traversal visits the smaller-byte child before the larger one. That claim depends on visiting `Children` **in ascending byte order.**

`Children` here is a Go `map[byte]*RaxNode`. Iterating a Go map with `range` gives no order guarantee at all — two runs over the same map can visit keys in different sequences. The "sorted for free" property is a fact about the tree's *shape*, not about this particular Go type. Any code that walks `Children` for a range scan or a max-of-subtree search has to impose the byte order itself, every time it iterates, rather than relying on `range` to provide it. Worth deciding early which approach fits: sort the keys before each walk, or store children in a way that is sorted by construction. Either resolves it; forgetting the problem exists is what causes a range query to return entries out of order.

## 9. Wiring this into the `Stream` struct

`app/structures/stream/stream.go` currently has the rax field commented out:

```go
// Rax               map[Id]*listpack.Listpack
```

Two things need to change here, beyond just removing the comment marker:

- The field's type should be a `*radix.RaxNode` (the tree root), not a `map[Id]*listpack.Listpack`. A map was always a stand-in — Section 8 of [Streams.md](Streams.md) already names this as the piece a flat map cannot provide (sorted, range-friendly storage).
- `RaxNode.Children` is keyed by raw bytes, but `Id` is a `{ms uint64, seq uint64}` pair. Something needs to turn an `Id` into the 16-byte, big-endian key the tree actually walks — 8 bytes of `ms` followed by 8 bytes of `seq`, in that order, so that byte comparison and numeric comparison agree. `ms` has to come first: swapping the order would make two IDs with the same `ms` but different `seq` compare correctly, but would break ordering between two different `ms` values.

## 10. What's already broken in `radix.go` today

The current file ends with:

```go
) {

}
```

This is a dangling, unfinished function signature (a stray `)` followed by a body with no `func` line above it) — the package does not compile as it stands. Worth clearing out or completing before anything else here gets tested.

## 11. Edge cases to design for

- **Duplicate ID insert.** `validateStreamEntryId` (per [Streams.md](Streams.md)) should stop a duplicate or out-of-order ID before it reaches the rax, but the insert logic still has to decide what happens if an exact-match key (Case 1, Section 4) ever does arrive — overwrite, or reject as a bug signal.
- **Empty tree.** The very first `XADD` on a stream inserts into a rax with no nodes at all. `New()` already sets `Children` to an empty, non-nil map — insert logic should not assume at least one child exists anywhere.
- **Single-node tree.** "Largest key overall" and "largest key ≤ X" both need to behave correctly when the whole tree is just the root plus one leaf, with no branching yet.
- **Concurrent `XADD` on the same stream.** [Concurrency.md](Concurrency.md) covers the project's locking model in general — an insert that mutates node identity mid-tree (a split replaces a node, not just edits one) needs the same "don't let two goroutines restructure the tree at once" care that any in-place mutation needs, once this is wired under the stream's own lock.
- **Deletion.** Not needed for a first working `XADD`/`XRANGE` pair. [Streams.md](Streams.md) already notes that real Redis handles `XDEL` as a tombstone flag on the listpack entry, not a rax removal — the rax key for a node stays until that whole node is later merged or dropped.

## 12. Checklist

| Piece | Section | Status |
|---|---|---|
| Insert, with the 5-case split logic | 4, 5 | Not started — stub only has `New()` |
| Exact Get | 4 | Not started |
| Largest key overall (rightmost walk) | 7 | Not started |
| Largest key ≤ X (predecessor walk) | 7 | Not started |
| Deterministic child ordering for range scans | 8 | Not started — needs a decision, not just code |
| `Id` → 16-byte big-endian key encoding | 9 | Not started |
| `Stream.Rax` field, typed as `*radix.RaxNode` | 9 | Field is commented out |
| Broken trailing syntax in `radix.go` | 10 | Needs cleanup before the package builds |

## Sources

- [Streams.md](Streams.md) — the stream-level design this note assumes throughout
- Redis source: `src/rax.c`, `src/rax.h` — the real node layout (compressed vs single-byte nodes, sorted byte/pointer arrays instead of a map) this codebase's simpler single-struct design stands in for
- Antirez, [Redis streams as a pure data structure](https://antirez.com/news/128)
- Wikipedia, [Radix tree](https://en.wikipedia.org/wiki/Radix_tree)
