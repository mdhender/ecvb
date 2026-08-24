# Distance Map

With only **100 stellia** per game, I would not optimize this for performance yet.
The squared-distance comparison is already the right mathematical approach because it avoids `sqrt()`.

I’d keep the calculation unless distance queries become a particularly important part of the game.

### 1. Writing the filter

Your expression:

```sql
and (s1.x - s2.x) * (s1.x - s2.x)
  + (s1.y - s2.y) * (s1.y - s2.y)
  + (s1.z - s2.z) * (s1.z - s2.z) > ?
```

with `? = d * d` is good.
In particular, **don't replace it with `sqrt(...) > ?`**.
Squared distance is both simpler and cheaper.

For readability, you could calculate the deltas once in a CTE/subquery:

```sql
WITH pairs AS (
    SELECT
        s1.id AS s1_id,
        s2.id AS s2_id,
        s1.x - s2.x AS dx,
        s1.y - s2.y AS dy,
        s1.z - s2.z AS dz
    FROM stellia s1
    JOIN stellia s2 ON ...
)
SELECT ...
FROM pairs
WHERE dx * dx + dy * dy + dz * dz > ?;
```

But I wouldn't necessarily call that *better*.
Your original expression keeps everything in one place and is immediately recognizable as squared Euclidean distance.

If this appears in lots of queries, I would be more inclined to encapsulate it in your Go query-building code than make the SQL more elaborate.

### 2. Precomputed distances

With 100 stellia, there are only:

[
{100 \choose 2} = 4,950
]

unique pairs.

That's tiny.

If stellium coordinates are effectively immutable after the cluster is generated, a distance table is therefore quite reasonable.
But I'd store **squared distance**, not distance:

```sql
CREATE TABLE stellium_distance (
    game_id     INTEGER NOT NULL,
    s1_id       INTEGER NOT NULL,
    s2_id       INTEGER NOT NULL,
    distance_sq INTEGER NOT NULL,

    PRIMARY KEY (game_id, s1_id, s2_id),

    FOREIGN KEY (s1_id) REFERENCES stellia(id),
    FOREIGN KEY (s2_id) REFERENCES stellia(id),

    CHECK (s1_id < s2_id)
);
```

Then:

```sql
WHERE sd.distance_sq > ?
```

Again, `?` is `d * d`.

That avoids floating-point values entirely.

There is one wrinkle: normalizing pairs with `s1_id < s2_id` makes lookups from a particular stellium slightly awkward:

```sql
WHERE s1_id = ? OR s2_id = ?
```

For EC, I'd seriously consider storing the **directed relation** instead:

```sql
CREATE TABLE stellium_distance (
    game_id     INTEGER NOT NULL,
    from_id     INTEGER NOT NULL,
    to_id       INTEGER NOT NULL,
    distance_sq INTEGER NOT NULL,

    PRIMARY KEY (game_id, from_id, to_id)
);
```

You'd have 9,900 rows rather than 4,950:

```text
100 × 99 = 9,900
```

Still essentially nothing to SQLite.

Then queries become very pleasant:

```sql
SELECT s.*
FROM stellium_distance d
JOIN stellia s ON s.game_id = :game_id and s.id = d.to_id
WHERE d.from_id = :from_id
  AND d.distance_sq <= :distance_squared
ORDER BY d.distance_sq;
```

And the primary-key index immediately narrows it to the 99 possible destinations from one system.

### Would I actually do it?

I'd base the decision more on the **domain model** than performance.

If EC frequently asks questions like:

```text
systems within jump range
systems outside sensor range
nearest systems
systems reachable with fuel X
systems within communication range
```

then I like the precomputed table.
Distance between two fixed stellia is an immutable property of the map, so calculating it repeatedly is unnecessary.
The table becomes a useful map primitive.

I'd probably call it something like:

```text
stellium_distances
```

and use the directed, 9,900-row version with:

```sql
(from_id, to_id, distance_sq)
```

You could also add:

```sql
CREATE INDEX stellium_distances_from_distance
ON stellium_distances (game_id, from_id, distance_sq);
```

That makes this particularly nice:

```sql
WHERE game_id = ?
  AND from_id = ?
  AND distance_sq <= ?
```

SQLite can walk exactly the relevant portion of the index.

**For performance alone:** no, it isn't worth worrying about with 100 systems.
Even a naïve comparison over all 10,000 combinations is trivial.

**For simplifying EC's queries and making distance a first-class property of the generated map:** yes, I think the 9,900-row `stellium_distances` table is attractive.
That's the reason I'd do it.
