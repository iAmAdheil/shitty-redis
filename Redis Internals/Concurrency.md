# Concurrency Models: From One Lock to Beating Redis

`Keyspace.md` lands on a single global mutex (`kmu`) over one unified map, for now. This note picks up where that leaves off: what the ceiling on that model is, and what a design that could genuinely out-throughput real Redis looks like.

## The ceiling of a single lock

One mutex around the whole keyspace means exactly one goroutine can be touching *any* key at *any* moment, no matter how many CPU cores the machine has. A `SET foo bar` and an unrelated `RPUSH mylist x` from two different clients still queue up behind each other. Adding more cores does nothing for command throughput under this model — it only helps things that happen outside the lock (parsing, network I/O).

This isn't a flaw unique to this project. It's the same ceiling real, single-threaded Redis has always had: one command executes at a time, full stop. Real Redis has historically stayed fast *despite* this, not because of clever locking — because in-memory operations are fast enough that one core, doing nothing else, keeps up with a lot of traffic. But it does mean single-threaded Redis (and a single-`kmu` clone of it) has a hard per-core throughput ceiling that more cores cannot move.

## What actually beats that ceiling: shared-nothing sharding

The real answer isn't a smarter lock — [as covered before](Keyspace.md), finer-grained locking (per-key, or striped) adds real correctness hazards (stale references, ordering discipline across every future command) for a throughput gain that's still capped by lock contention under the hood. The approach that gets genuinely different numbers is to **stop sharing memory between the pieces that need to run in parallel, so there's nothing left to lock.**

This is exactly Dragonfly's design, and it's real, working, and benchmarked — not a proposal:

> "Its philosophy is dead simple: making each thread as independent as possible. The first problem, exclusive access to keys, is solved by dividing the entire dataset into smaller sections called **shards**. Keys in a single shard are managed exclusively by one dedicated thread, which is known as a **shard-thread**."
> — [Dragonfly: Ensuring Atomicity](https://www.dragonflydb.io/blog/transactions-in-dragonfly)

> "Since a single key in a shard is managed exclusively by one dedicated thread, the shared-nothing architecture minimizes the need for complex locking and synchronization mechanisms."
> — [Redis and Dragonfly Architecture Comparison](https://www.dragonflydb.io/blog/redis-and-dragonfly-architecture-comparison)

Measured result, not a projection: [6.43 million ops/sec on a 64-core machine](https://www.dragonflydb.io/blog/dragonfly-achieves-6-million-rps-on-64-core-graviton3), scaling from ~300K ops/sec on 1 thread to 6.43M on 64 — close to linear with core count, something a single-threaded (or single-lock) design structurally cannot do.

### What this looks like in Go: the actor model, not translated mutexes

Go already has the right primitive for this — it just isn't a mutex. "Share memory by communicating" (goroutines + channels) *is* the shared-nothing model:

1. **Partition the keyspace into N shards.** `shard := hash(key) % N` (N around the number of CPU cores is a reasonable starting point, matching Dragonfly's thread-per-core approach).
2. **Each shard is owned by exactly one goroutine**, holding its own private map (or its own `vars`/`lists`/`streams` slice of the keyspace) that *no other goroutine ever touches directly*.
3. **Client-handling goroutines never touch a shard's data directly.** They build a command message (key, args, a reply channel) and send it down that shard's request channel — its "mailbox."
4. **Each shard goroutine runs a simple loop**: `for msg := range shardChan { handle(msg); msg.reply <- result }`. Because exactly one goroutine ever reads from and mutates that shard's map, there is no data race to prevent — correctness is structural, not a matter of remembering to lock the right thing in the right order. Every single-key command becomes automatically linearizable for free, the same property Dragonfly gets from this design.

### `BLPOP` gets dramatically simpler under this model

Every hazard worked through in `Keyspace.md` and the conversation before it — the stale-pointer race, the get-or-create race, the lock-ordering inversion between `RPUSH` and `Expire` — exists *because* multiple goroutines can touch the same key's state concurrently. Under sharding, they can't. A key's `List` and its `Listeners` both live inside one shard, touched only by that shard's single goroutine, processing one message at a time. `BLPOP`'s "check data, else register a waiter" and `RPUSH`'s "push, then feed waiters" both just become sequential steps inside that one goroutine's handling of one message — no lock, no race, no revalidation logic, because there's nothing else running concurrently against that state to race with.

### The part that's still genuinely hard: multi-key operations

A key's data lives on exactly one shard, so a command touching two keys that hash to *different* shards (`RENAME src dst`, `LMOVE`, a future `MULTI`/`EXEC`) can't just be "one message to one goroutine." Dragonfly's answer is a lightweight, academically-grounded locking protocol — VLL, "very lightweight locking" — applied *only* to the minority of commands that span shards. Its actual mechanism, from [Dragonfly's own transaction model doc](https://github.com/dragonflydb/dragonfly/blob/master/docs/transaction.md), is worth being precise about, because it's not what "locking" usually implies:

- **A lock is two integers, not a mutex.** Each shard keeps a table mapping each key it owns to a pair of counters — pending SHARED (read) intents and pending EXCLUSIVE (write) intents. Scheduling a transaction just increments the relevant counter for each key it touches. Nothing here is an OS mutex or spinlock, and nothing needs to be: that table lives inside one shard, and only that shard's own thread ever reads or writes it — the same shared-nothing rule that protects the actual data protects the lock bookkeeping too, recursively.
- **Most transactions never even allocate a transaction ID.** A single-shard command on an uncontended key runs inline the moment its shard receives it — increment the counter, do the work, decrement it, done. No queue, no cross-shard coordination, no global counter touched. The heavier machinery (a monotonic transaction ID, a per-shard ordered queue) only activates for the minority of commands that either span shards or land on a key another pending transaction already holds a conflicting intent on.
- **Two different lock granularities exist, for two different situations — not one mechanism doing both jobs.** The per-key intent locks above handle ordinary multi-key commands (`MSET`, `RENAME`). A small set of commands that must touch the *entire* keyspace at once (`FLUSHDB`, `FLUSHALL`, `MOVE`, `SAVE`) instead take a **shard-level lock** on every shard — a much heavier operation, and one Dragonfly's own docs explicitly flag as something to avoid on a hot path, since it serializes all shard activity while held.
- **`BLPOP` gets a specific carve-out**, not just "treated as a read that blocks": it schedules normally and checks its keys; if they're all empty, it registers a watch on each one, is removed from the ordering queue (so it stops competing for a turn) while *keeping* its intent locks (so nothing else can start a conflicting write on those exact keys mid-wait), and only the connection's own coordinator suspends — the shard threads keep serving every other client the entire time. A later push to a watched key wakes the coordinator directly and lets it jump the queue to claim the value, bypassing the normal ordering entirely.

The full protocol is real engineering effort — this note isn't proposing you reimplement VLL from scratch, just naming what the hard 20% actually is, so it's not a surprise later: single-key commands (the overwhelming majority — `GET`, `SET`, `RPUSH`, `LPOP`, `BLPOP`, `LRANGE`) get this architecture almost for free; commands spanning keys need a deliberate cross-shard protocol on top.

## Where this leaves the project, practically

This is a foundational rewrite — every command handler's relationship to the keyspace changes, from "acquire a lock, touch a map" to "send a message, await a reply." It's not something to bolt onto the current single-`kmu` design incrementally; it's a distinct architectural phase.

The practical path: keep `kmu` for now (per `Keyspace.md`) while the command set and correctness (the kind of bugs already hunted down in `Lists.md`) are still being built out. Treat the shared-nothing rewrite as a deliberate later milestone — worth doing specifically *because* it's the one design in this note that has a real, measured shot at beating Redis's own throughput, not because the current design is wrong for where the project is now.

## Sources

- Dragonfly: [Transaction Model (docs/transaction.md)](https://github.com/dragonflydb/dragonfly/blob/master/docs/transaction.md) — the precise intent-lock/shard-lock/BLPOP mechanism cited above
- Dragonfly: [Ensuring Atomicity — A Tale of Dragonfly Transactions](https://www.dragonflydb.io/blog/transactions-in-dragonfly)
- Dragonfly: [Redis and Dragonfly Architecture Comparison](https://www.dragonflydb.io/blog/redis-and-dragonfly-architecture-comparison)
- Dragonfly: [Achieves 6.43 Million RPS on a 64-Core Graviton3 Instance](https://www.dragonflydb.io/blog/dragonfly-achieves-6-million-rps-on-64-core-graviton3)
- [github.com/dragonflydb/dragonfly](https://github.com/dragonflydb/dragonfly) — background section on the shared-nothing/VLL design
