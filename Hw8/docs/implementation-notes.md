# HW8 — MySQL Integration: Implementation Notes

## Database Schema Design

I chose a **two-table normalized design**: `shopping_carts` and `cart_items`.

**Why two tables instead of one?**
A single denormalized table (one row per cart-item pair, repeating customer_id on every row) would simplify reads but make "count all carts for a customer" expensive and create update anomalies. The normalized approach keeps cart metadata (customer, timestamps) separate from line items, so a customer history query (`WHERE customer_id = ?`) is a clean single-table index scan with no duplicates.

**Trade-off considered:** I could have used a JSON column on `shopping_carts` to store items as an embedded array (like MongoDB). This would give single-row reads but makes item-level updates harder and wastes space on every read even when you only need cart metadata. The relational approach is more natural for MySQL and gives the query planner better optimization options.

## Index Strategy

Three indexes beyond primary keys:
- `idx_customer_id` on `shopping_carts(customer_id)` — supports "all carts by customer" queries without full table scan.
- `idx_cart_id` on `cart_items(cart_id)` — the JOIN key for cart retrieval. Without this, every `GET /shopping-carts/{id}` does a full scan of `cart_items`.
- `uk_cart_product` UNIQUE on `cart_items(cart_id, product_id)` — prevents duplicate product entries and enables the `INSERT ... ON DUPLICATE KEY UPDATE` upsert pattern for idempotent add-item operations.

## Cart-Item Relationship

The `cart_items` table has a foreign key `fk_cart_items_cart` referencing `shopping_carts(cart_id)` with `ON DELETE CASCADE`. This means deleting a cart automatically removes all its items, preventing orphaned rows. InnoDB's row-level locking means concurrent modifications to *different* carts don't block each other — only two transactions touching the *same* cart will serialize.

## Connection Pool Configuration

- **MaxOpenConns = 20**: With `db.t3.micro` allowing 150 total connections, this supports up to 7 ECS tasks before pool exhaustion. For this assignment with 1 task, 20 is generous headroom.
- **MaxIdleConns = 10**: Keeps 10 connections warm to avoid TCP handshake overhead on the hot path (cart retrievals).
- **ConnMaxLifetime = 5min**: Recycles connections to handle RDS DNS changes and maintenance events.

## Key Challenges

1. **RDS startup delay**: The ECS task boots faster than RDS becomes available. Solved with a retry loop (30 attempts × 2s backoff) in `InitDB()`.
2. **MySQL driver multi-statement**: The `go-sql-driver/mysql` driver doesn't support multi-statement execution by default. I had to split the schema migration into individual `Exec()` calls instead of one big SQL string.
3. **NULL handling in LEFT JOIN**: When a cart has no items, the item columns come back as NULL. Required using `sql.NullInt64` / `sql.NullTime` in the scan and checking `.Valid` before appending.

## Performance Observations vs HW5

- **HW5 (in-memory)**: Sub-millisecond response times for all product operations. No persistence — data lost on restart.
- **HW8 (MySQL)**: Cart operations average ~5-30ms depending on network hop to RDS. The dominant cost is the network round-trip to the RDS instance in a different subnet, not the query execution itself.
- **Cart retrieval with JOIN**: With the `idx_cart_id` index, the JOIN completes in <5ms at the database level. The ~20-50ms you see in the test includes network latency from ECS to RDS.
- **Connection pooling impact**: Without pooling (opening a new connection per request), each operation adds ~30ms for TCP handshake + TLS negotiation. The pool eliminates this for steady-state traffic.

## What I'd Do Differently

If I were optimizing for production, I'd add Redis as a read-through cache in front of MySQL for cart retrieval (the most frequent operation). The write-path would go to MySQL and invalidate the cache. This would bring read latency back to sub-millisecond while keeping MySQL as the durable source of truth — essentially combining HW5's speed with HW8's persistence.
