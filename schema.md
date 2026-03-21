# SQLite Database Schema

This document describes the database schema used by `albiondata-client` to store Albion Online game data locally.

## Overview

The client collects game data from network traffic inspection and stores it in a local SQLite database. All data is grouped by upload batches using a `upload_identifier` UUID, allowing you to track which records arrived together.

Database file: `albiondata.db` (default, configurable with `-db` flag)

## Automatic Cleanup

On startup, the client automatically removes expired records from:
- **`market_orders`**: Orders where the `expires` timestamp has passed
- **`market_notifications`**: Notifications where the `expires` timestamp has passed

The cleanup process:
1. Runs once when the client starts
2. Compares the RFC3339 `expires` timestamp with the current time
3. Deletes all expired records
4. Logs the number of records removed

This keeps the database clean of stale auction orders and market mail notifications.

---

## Tables

### `market_orders`

Market buy and sell orders from the auction house.

| Column | Type | Description |
|--------|------|-------------|
| `id` | INTEGER | Order ID from game |
| `item_type_id` | TEXT | Item identifier (e.g., "T4_BAG") |
| `item_group_type_id` | TEXT | Item group/category identifier |
| `location_id` | TEXT | Location code (e.g., "3005", "BLACKBANK-1") |
| `quality_level` | INTEGER | Item quality: 1-5 |
| `enchantment_level` | INTEGER | Enchantment level: 0-3 |
| `unit_price_silver` | INTEGER | Price per unit in silver |
| `amount` | INTEGER | Number of items in order |
| `auction_type` | TEXT | Order type: "offer" (selling) or "request" (buying) |
| `expires` | TEXT | Expiration timestamp (RFC3339 format) |
| `upload_identifier` | TEXT | UUID batch grouping (correlation ID) |
| `captured_at` | DATETIME | Server timestamp when record was inserted |

**Key insights:**
- Both buy and sell orders use the same table with `auction_type` distinguishing them
- `location_id` can be numeric (e.g., "3005" for Royal Continent) or named (e.g., "BLACKBANK-1")
- All records in a single market update share the same `upload_identifier`

---

### `market_histories`

Historical price data for items over different time periods.

| Column | Type | Description |
|--------|------|-------------|
| `albion_id` | INTEGER | Numeric item ID from Albion's internal data |
| `location_id` | TEXT | Location code (e.g., "3005") |
| `quality_level` | INTEGER | Item quality: 1-5 |
| `timescale` | INTEGER | Period grouping: 0=Hours, 1=Days, 2=Weeks |
| `item_amount` | INTEGER | Total units traded in period |
| `silver_amount` | INTEGER | Total silver transacted in period |
| `timestamp` | INTEGER | Unix epoch seconds (end of period) |
| `upload_identifier` | TEXT | UUID batch grouping |
| `captured_at` | DATETIME | Server timestamp when record was inserted |

**Key insights:**
- Each row represents aggregated trading data for one item at one location over one time period
- `timescale` values: 0 (hourly), 1 (daily), 2 (weekly)
- `timestamp` marks the end of the period, not the start
- Query example: Find all hourly data for item ID 123456 in location 3005:
  ```sql
  SELECT * FROM market_histories
  WHERE albion_id = 123456 AND location_id = '3005' AND timescale = 0
  ORDER BY timestamp DESC;
  ```

---

### `gold_prices`

Gold-to-silver conversion rates (for premium currency trading).

| Column | Type | Description |
|--------|------|-------------|
| `price` | INTEGER | Price of 1 gold in silver |
| `timestamp` | INTEGER | Unix epoch seconds when price was recorded |
| `upload_identifier` | TEXT | UUID batch grouping |
| `captured_at` | DATETIME | Server timestamp when record was inserted |

**Key insights:**
- Simple time-series table tracking the in-game exchange rate
- `timestamp` is the record time, not necessarily aligned to the hour/day boundary
- Example: "At timestamp X, 1 gold cost Y silver"

---

### `map_data`

World map zone information including buildings, fees, and player ownership.

| Column | Type | Description |
|--------|------|-------------|
| `zone_id` | INTEGER | Zone/cluster ID from game |
| `building_type` | INTEGER | Type code for the building (varies by zone) |
| `available_food` | INTEGER | Available food resource amount |
| `reward` | INTEGER | Reward/tax amount collected |
| `available_silver` | INTEGER | Available silver in zone treasury |
| `owner` | TEXT | Guild/player owner name |
| `public_fee` | INTEGER | Tax rate for public use (basis points) |
| `associate_fee` | INTEGER | Tax rate for guild associates (basis points) |
| `coordinate_x` | INTEGER | X coordinate within zone |
| `coordinate_y` | INTEGER | Y coordinate within zone |
| `durability` | INTEGER | Building structural durability |
| `permission` | INTEGER | Permission level/flags |
| `upload_identifier` | TEXT | UUID batch grouping |
| `captured_at` | DATETIME | Server timestamp when record was inserted |

**Key insights:**
- One row per building/plot in a zone
- All fields for a single zone update share the same `zone_id` and `upload_identifier`
- Coordinates allow reconstruction of zone maps
- `public_fee` and `associate_fee` are in basis points (1 bp = 0.01%)

---

### `bandit_events`

Redzone bandit phase events (PvP raid phases).

| Column | Type | Description |
|--------|------|-------------|
| `event_time` | INTEGER | Tick-based timestamp when the phase ended |
| `phase` | INTEGER | Phase number: 1, 2, or 3 |
| `upload_identifier` | TEXT | UUID batch grouping |
| `captured_at` | DATETIME | Server timestamp when record was inserted |

**Key insights:**
- Minimal event tracking for redzone PvP phases
- `event_time` is tick-based from Albion's game clock, not Unix timestamp
- Each phase progression is a separate record

---

### `skills`

Player character skill progression data (private).

| Column | Type | Description |
|--------|------|-------------|
| `character_id` | TEXT | UUID of the player character |
| `character_name` | TEXT | Name of the player character |
| `skill_id` | INTEGER | Skill ID from game |
| `level` | INTEGER | Current skill level |
| `percent_next_level` | REAL | Progress toward next level (0.0-100.0) |
| `fame` | INTEGER | Cumulative fame earned in skill |
| `upload_identifier` | TEXT | UUID batch grouping |
| `captured_at` | DATETIME | Server timestamp when record was inserted |

**Key insights:**
- Each row is one skill for one character at one point in time
- `character_id` and `character_name` identify the player (character personalization)
- `percent_next_level` is a floating-point percentage
- All skills for a character in a single upload share the same `upload_identifier`
- Private data: only visible to the character owner

---

### `market_notifications`

Market mail notifications (sales and expiries) (private).

| Column | Type | Description |
|--------|------|-------------|
| `character_id` | TEXT | UUID of the player character |
| `character_name` | TEXT | Name of the player character |
| `notification_type` | TEXT | "SalesNotification" or "ExpiryNotification" |
| `mail_id` | INTEGER | Mail/notification ID |
| `item_type_id` | TEXT | Item identifier (e.g., "T4_BAG") |
| `location_id` | TEXT | Location code |
| `amount` | INTEGER | Number of items |
| `expires` | TEXT | Expiration timestamp (RFC3339 format) |
| `unit_price_silver` | INTEGER | Price per unit in silver |
| `total_after_taxes` | REAL | Total revenue after marketplace taxes (NULL for ExpiryNotification) |
| `sold` | INTEGER | Units sold (NULL for SalesNotification, only in ExpiryNotification) |
| `upload_identifier` | TEXT | UUID batch grouping |
| `captured_at` | DATETIME | Server timestamp when record was inserted |

**Key insights:**
- Two notification types in one table:
  - **SalesNotification**: Item sold; `total_after_taxes` has value, `sold` is NULL
  - **ExpiryNotification**: Order expired; `sold` has value, `total_after_taxes` is NULL
- `character_id` and `character_name` identify the player (character personalization)
- Private data: only visible to the character owner
- Query example: Get all sales notifications for a character:
  ```sql
  SELECT * FROM market_notifications
  WHERE character_id = 'uuid-here' AND notification_type = 'SalesNotification'
  ORDER BY captured_at DESC;
  ```

---

## Data Types

- **INTEGER** — 64-bit signed integers (Unix timestamps, prices, quantities)
- **REAL** — 64-bit floating-point (percentages, taxes)
- **TEXT** — UTF-8 strings (UUIDs, names, item IDs, locations)
- **DATETIME** — ISO8601 timestamps (SQLite `CURRENT_TIMESTAMP`)

---

## Batch Grouping & Deduplication

Every record includes an `upload_identifier` field—a UUIDv4 generated at the time the data is sent. This allows you to:

1. **Group records by upload:** All records from a single market snapshot share one UUID
2. **Detect duplicates:** If the same UUID appears twice, the data is a duplicate
3. **Audit trail:** Correlate which records arrived together across all tables

Example query to find all records from a specific batch:
```sql
SELECT 'market_orders' as table_name, COUNT(*) as count FROM market_orders WHERE upload_identifier = 'abc-123'
UNION ALL
SELECT 'market_histories', COUNT(*) FROM market_histories WHERE upload_identifier = 'abc-123'
UNION ALL
SELECT 'gold_prices', COUNT(*) FROM gold_prices WHERE upload_identifier = 'abc-123'
UNION ALL
SELECT 'map_data', COUNT(*) FROM map_data WHERE upload_identifier = 'abc-123'
UNION ALL
SELECT 'bandit_events', COUNT(*) FROM bandit_events WHERE upload_identifier = 'abc-123'
UNION ALL
SELECT 'skills', COUNT(*) FROM skills WHERE upload_identifier = 'abc-123'
UNION ALL
SELECT 'market_notifications', COUNT(*) FROM market_notifications WHERE upload_identifier = 'abc-123';
```

---

## Example Queries

### Latest market prices for an item
```sql
SELECT location_id, unit_price_silver, auction_type, amount, captured_at
FROM market_orders
WHERE item_type_id = 'T4_ARMOR_PLATE' AND auction_type = 'offer'
ORDER BY captured_at DESC
LIMIT 100;
```

### Gold price trend
```sql
SELECT timestamp, price, captured_at
FROM gold_prices
ORDER BY timestamp DESC
LIMIT 24;  -- Last 24 records
```

### Player skill snapshot
```sql
SELECT skill_id, level, percent_next_level, fame
FROM skills
WHERE character_id = 'player-uuid-here'
ORDER BY captured_at DESC
LIMIT 1;  -- Most recent
```

### Map ownership changes
```sql
SELECT DISTINCT owner, zone_id, captured_at
FROM map_data
ORDER BY captured_at DESC, zone_id;
```

### Player's market activity
```sql
SELECT notification_type, item_type_id, amount, unit_price_silver,
       total_after_taxes, sold, captured_at
FROM market_notifications
WHERE character_id = 'player-uuid-here'
ORDER BY captured_at DESC;
```

---

## Notes

- All timestamps in the `captured_at` column are in the server's local timezone
- `upload_identifier` is generated client-side and should be globally unique per upload batch
- Private tables (`skills`, `market_notifications`) contain character-identifying information
- No foreign key constraints are defined; relationships are implicit through field values
- Database is created automatically on first run; schema is initialized via `CREATE TABLE IF NOT EXISTS`
