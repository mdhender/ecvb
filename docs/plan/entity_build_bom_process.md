# Entity Construction Process — BOM-Driven Build System

## Overview

The `using` assets represent the **Bill of Materials (BOM)** for an entity build.

The BOM is both:

1. The recipe for what the target entity must ultimately contain.
2. The player's construction priority after the structure is complete.

The target entity is created immediately when the build order is accepted, even if none of the required BOM assets are currently available.

Construction then proceeds over time according to:

- build seniority,
- material availability,
- transport availability,
- CWRK availability,
- BOM priority,
- life-support constraints for population,
- and orbital decay while the target lacks functioning SDRV units.

---

## Terminology

- **Builder** — the entity creating another entity.
- **Target** — the entity being created.
- **BOM** — the ordered list of `using` assets required to complete the target.
- **CWRK** — construction workers used to assemble components.
- **SDRV** — drive units. Until the target has at least one functioning SDRV, it falls toward the planet.
- **Structural components** — the BOM components required to create the target's structure.
- **Seniority** — priority among multiple builds competing for the same builder resources.

---

## Core Rules

### 1. The target is created immediately

When a build order is accepted:

```text
create target
target.status = IN_PROGRESS
target.builder = builder
target.controller = builder.controller
target.ring = 99
target.BOM = ordered BOM
```

The target exists even if the builder currently has none of the required BOM components.

Issuing the build therefore starts the orbital-decay clock immediately.

---

### 2. The BOM is a persistent construction queue

The builder does **not** need to possess the entire BOM when the order is issued.

Instead, the target attempts to acquire, transport, and assemble BOM assets as they become available.

The BOM remains attached to the target for the entire build.

A useful BOM line model is:

```text
BOMLine
    unit_type
    quantity_required
    quantity_claimed
    quantity_on_site
    quantity_completed
    sequence
```

Where:

- `quantity_required` — total amount required by the design.
- `quantity_claimed` — assets owned by the builder and committed to this target but not yet transported.
- `quantity_on_site` — assets delivered to the target but not yet completed.
- `quantity_completed` — assets installed, assembled, or otherwise incorporated into the target.
- `sequence` — the player's explicit BOM priority.

For every BOM line:

```text
quantity_required =
    quantity_needed
    + quantity_claimed
    + quantity_on_site
    + quantity_completed
```

---

### 3. Senior builds have priority

A builder may have multiple active targets.

Those builds compete for:

- available BOM components,
- transports,
- CWRK,
- and any other shared construction resources.

The senior target gets first access.

A deterministic seniority rule should be used, for example:

```text
oldest build order first
then build/order ID as tie-breaker
```

Resources consumed or committed to a senior build during a turn are not available to junior builds during that turn.

Resources that a target is not able to use (for example, transports were unavailable) are available for use in later phases.

---

## Construction Phase 1 — Build the Structure

The structure is the mandatory first phase.

Before the structure is complete, only structural BOM entries are eligible for transfer and assembly.

```text
while assembled_structural < required_structural:
    process structural BOM entries only
```

Within the structural subset, BOM sequence determines priority.

Each turn, the build attempts to:

1. Claim available structural components.
2. Use transports to move claimed structural components from the builder to the target.
3. Use transports to move CWRK from the builder to the target.
4. Assemble structural components already on site.
5. Return CWRK to the builder.

Construction progress can therefore be limited independently by:

```text
materials
transports
CWRK
```

Transport capacity is especially important because it can constrain both:

- component delivery, and
- worker movement.

Once every required structural component has been completed:

```text
target.structure_complete = true
```

the remainder of the BOM becomes eligible.

---

## Construction Phase 2 — Process the Remaining BOM

After the structure is complete, the target processes the remaining BOM in the player's specified order.

Example:

```text
1. SDRV-4 x 10
2. LIFE-3 x 20
3. POWR-3 x 15
4. LASR-4 x 30
5. POPU x 500
6. CARG-3 x 10
```

This ordering is strategically meaningful.

A smart player will usually place SDRV near the top of the BOM because every turn before the first functioning drive is completed causes the target to fall one ring toward the planet.

---

## BOM Order Is Priority, Not a Hard Dependency Chain

An unavailable BOM line should **not** freeze the entire build.

Instead, each turn the target scans eligible BOM lines from top to bottom.

If a BOM entry cannot currently make progress because its required material is unavailable or it is otherwise ineligible, the target may continue to lower-priority BOM entries.

For example:

```text
1. SDRV-4
2. SHLD-4
3. LIFE-4
4. POPU
5. LASR-4
```

If no SDRV-4 units are available, the build may work on SHLD-4 or LIFE-4 instead.

As soon as SDRV-4 becomes available, however, that higher BOM line gets first access to available materials, transports, and CWRK.

The rule is:

> BOM order determines priority among work that can currently be performed. An unavailable or ineligible BOM line may be skipped.

---

## Claiming Materials

For each builder, process active targets in seniority order.

For each target, scan currently eligible BOM entries in BOM order and claim available components.

Conceptually:

```text
for target in builder.targets_by_seniority:

    for bom_line in target.eligible_BOM_lines_in_order:

        needed =
            required
            - claimed
            - onsite
            - completed

        claim = min(
            needed,
            builder.available_unclaimed(unit_type)
        )

        builder.claim(unit_type, claim, target)
```

Claiming does not require transport.

Claimed assets remain physically aboard the builder until they are moved, but they are reserved for the target and may not be used by a junior build.

This ensures that a junior build cannot consume components required by a senior build merely because the senior build lacks transport that turn.

---

## Transporting BOM Components

Claimed assets do not teleport.

The target attempts to move claimed components using the builder's available transports.

Within a build, transfer priority follows BOM order.

```text
for eligible BOM line in priority order:
    transfer as much claimed material as transport capacity permits
```

When transport capacity is exhausted, transfers stop for that turn.

Unmoved claimed assets remain aboard the builder and stay reserved for the target.

---

## Transporting CWRK

CWRK must also be transported from the builder to the target before it can perform assembly work.

After work is completed, CWRK returns to the builder.

If material transport and CWRK movement share the same transport pool, the build should avoid consuming all transport capacity moving components while leaving no capacity to move workers.

The implementation should prefer a useful balance between:

- delivering material, and
- keeping available CWRK productively working.

A lack of transport does not cancel the build. It simply reduces or prevents progress for that turn.

---

## Assembly Priority

Once structural construction is complete, CWRK assembles on-site BOM assets in BOM order.

Example:

```text
SDRV on site     4
LIFE on site    10
LASR on site    20

available work  12
```

If BOM priority is:

```text
SDRV
LIFE
LASR
```

then the turn would complete:

```text
4 SDRV
8 LIFE
0 LASR
```

assuming one unit of CWRK work completes one unit.

Higher-priority BOM lines therefore receive assembly effort before lower-priority lines.

---

## SDRV and Orbital Decay

The target begins in ring 99.

At the end of every turn:

```text
if functioning_SDRV_count(target) == 0:
    target.ring -= 1
```

The decay check happens **after construction for the turn**.

This matters because a drive completed during the current turn should be able to prevent that turn's fall.

The sequence is:

```text
transfer / construction
        ↓
recalculate functioning units
        ↓
end-of-turn orbital-decay check
```

The target stops falling as soon as it has at least one **functioning** SDRV.

The check should use the entity's actual operational-state rules, not merely the number of SDRV units physically assembled.

---

## Population and Life Support

Population requires special handling because unsupported population must not be transferred onto an unfinished target.

A population BOM line is eligible for transfer only when the target already has enough **functioning life-support capacity**.

Conceptually:

```text
available_population_capacity =
    functioning_life_support_capacity(target)
    - current_population(target)
```

Then:

```text
population_transfer =
    min(
        BOM_population_still_needed,
        available_population_capacity,
        available_population_at_builder,
        transport_capacity
    )
```

This is a **transfer restriction**, not merely an assembly restriction.

Unsupported population never leaves the builder.

---

### Population does not block lower BOM entries

A population entry that is temporarily unsafe to transfer is simply skipped.

Example:

```text
1. POPU x 500
2. LIFE x 10
3. SDRV x 5
```

Initially, POPU is ineligible because there is insufficient life support.

The target skips POPU and may work on LIFE.

As functioning LIFE capacity comes online, population becomes eligible for transfer.

The target may then return to the higher-priority POPU line.

This prevents a badly ordered BOM from creating an unrecoverable deadlock while still allowing poor BOM ordering to impose strategic costs.

---

### Life-support capacity must include population already aboard

The safety invariant is:

```text
functioning_population_capacity(target)
    >=
current_population(target)
    + population_about_to_be_transferred
```

Example:

```text
life-support capacity = 500
population already aboard = 450
requested transfer = 100
```

Only 50 population units may be transferred.

Life-support capacity is calculated from **functioning assembled units**, never merely delivered components.

---

## Generic BOM Completion

`quantity_completed` should be interpreted according to the asset type.

Examples:

- structural component → assembled into the structure
- SDRV → installed and completed
- LIFE → installed and completed
- weapon → installed and completed
- population → safely transferred aboard
- cargo-type asset → installed or stowed as defined by that asset's rules

Using `completed` rather than `assembled` avoids awkward semantics such as "assembling population."

---

## Build Completion

The entity is complete when every BOM line has been completed:

```text
for every BOM line:
    quantity_completed == quantity_required
```

At that point:

```text
target.status = COMPLETE
```

The entity stops participating in construction-specific processing and becomes a normal entity.

---

## No-Progress Turns

After the target has been created, lack of resources does not cause the build order to fail.

Instead, it causes reduced or zero progress.

Examples:

```text
required component unavailable
    -> skip that BOM line

no transports
    -> no transfers
    -> no CWRK movement
    -> little or no construction progress

no CWRK
    -> materials may be delivered
    -> assembly does not progress

population lacks life support
    -> population remains with builder
    -> skip that BOM line
```

If the target still lacks a functioning SDRV at the end of the turn, it falls one ring regardless of why construction was delayed.

---

## Overall Lifecycle

```text
BUILD ORDER ISSUED
        │
        ▼
TARGET CREATED AT RING 99
        │
        ▼
PROCESS STRUCTURAL BOM
        │
        ▼
STRUCTURE COMPLETE
        │
        ▼
PROCESS REMAINING BOM
IN PLAYER-SPECIFIED PRIORITY ORDER
        │
        ├── material unavailable → skip for now
        ├── no transport → progress constrained
        ├── no CWRK → assembly constrained
        └── population unsupported → skip for now
        │
        ▼
ALL BOM ITEMS COMPLETE
        │
        ▼
ENTITY COMPLETE
```

Independently, at the end of every turn:

```text
if target has no functioning SDRV:
    fall one ring
```

---

## Design Summary

The key design principle is:

> The BOM is both the entity's recipe and the player's construction policy.

The target is created immediately and begins falling from ring 99 until a functioning drive comes online.

Structure is always built first.

After structural completion, the remaining BOM is processed in player-specified priority order. Higher-priority items receive first access to available materials, transports, and CWRK, while temporarily unavailable or ineligible BOM entries may be skipped.

This makes BOM ordering strategically important without allowing temporary shortages to deadlock construction.

A well-designed BOM gets essential systems such as SDRV and LIFE online quickly.

A poorly designed BOM can leave a large unfinished entity falling toward the planet while workers spend scarce transport and construction capacity on less important systems.
