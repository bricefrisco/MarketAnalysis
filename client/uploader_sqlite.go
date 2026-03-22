package client

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ao-data/albiondata-client/lib"
	"github.com/ao-data/albiondata-client/log"
	_ "modernc.org/sqlite"
)

// writeRequest represents a single database write operation
type writeRequest struct {
	body       []byte
	topic      string
	identifier string
}

// ScanNotify is called when the player requests 7-day market history for an item.
// Set by the main package to notify the dashboard scanner in real-time.
var ScanNotify func(albionId int32, qualityLevel int)

// Location mapping for Royal Continent cities
var locationMap = map[string]string{
	"0007": "Thetford",
	"1002": "Lymhurst",
	"2004": "Bridgewatch",
	"3008": "Martlock",
	"4002": "Fort Sterling",
}

type sqliteUploader struct {
	db          *sql.DB
	writeQueue  chan writeRequest
	albionIDMap map[string]int32 // item_type_id -> albion_id
}

// newSQLiteUploader creates a new SQLite uploader
func newSQLiteUploader(dbPath string) (uploader, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	if err := cleanupExpiredData(db); err != nil {
		log.Warnf("Warning: failed to cleanup expired data: %v", err)
	}

	uploader := &sqliteUploader{
		db:          db,
		writeQueue:  make(chan writeRequest, 100),
		albionIDMap: loadAlbionIDMap(),
	}

	// Start the write queue processor goroutine
	go uploader.processWriteQueue()

	return uploader, nil
}

// initSchema creates all tables if they don't exist
func initSchema(db *sql.DB) error {
	schemas := []string{
		`CREATE TABLE IF NOT EXISTS locations (
			location_id TEXT PRIMARY KEY, location_name TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS market_orders (
			id INTEGER, item_type_id TEXT, item_group_type_id TEXT,
			location_id TEXT, quality_level INTEGER, enchantment_level INTEGER,
			albion_id INTEGER,
			unit_price_silver INTEGER, amount INTEGER, auction_type TEXT,
			expires TEXT, upload_identifier TEXT, captured_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(item_type_id, enchantment_level, location_id, quality_level, auction_type)
		)`,
		`CREATE TABLE IF NOT EXISTS market_histories (
			albion_id INTEGER, location_id TEXT, quality_level INTEGER,
			timescale INTEGER, item_amount INTEGER, silver_amount REAL,
			per_item REAL, timestamp INTEGER, upload_identifier TEXT,
			captured_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS gold_prices (
			price INTEGER, timestamp INTEGER, upload_identifier TEXT,
			captured_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS map_data (
			zone_id INTEGER, building_type INTEGER, available_food INTEGER,
			reward INTEGER, available_silver INTEGER, owner TEXT,
			public_fee INTEGER, associate_fee INTEGER,
			coordinate_x INTEGER, coordinate_y INTEGER,
			durability INTEGER, permission INTEGER,
			upload_identifier TEXT, captured_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS bandit_events (
			event_time INTEGER, phase INTEGER, upload_identifier TEXT,
			captured_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS skills (
			character_id TEXT, character_name TEXT,
			skill_id INTEGER, level INTEGER, percent_next_level REAL, fame INTEGER,
			upload_identifier TEXT, captured_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS market_notifications (
			character_id TEXT, character_name TEXT, notification_type TEXT,
			mail_id INTEGER, item_type_id TEXT, location_id TEXT,
			amount INTEGER, expires TEXT, unit_price_silver INTEGER,
			total_after_taxes REAL, sold INTEGER,
			upload_identifier TEXT, captured_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, schema := range schemas {
		if _, err := db.Exec(schema); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	// Populate locations table
	if err := populateLocations(db); err != nil {
		return fmt.Errorf("failed to populate locations: %w", err)
	}

	// Run migrations to add new columns to existing tables
	if err := runMigrations(db); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// runMigrations removes location_name columns from tables if they exist
func runMigrations(db *sql.DB) error {
	tables := []string{"market_orders", "market_histories"}

	for _, table := range tables {
		// Check if location_name column exists
		if hasColumn(db, table, "location_name") {
			// Drop the column
			_, err := db.Exec(fmt.Sprintf(`ALTER TABLE %s DROP COLUMN location_name`, table))
			if err != nil {
				log.Warnf("Failed to drop location_name from %s: %v", table, err)
				// Continue with other migrations even if one fails
			} else {
				log.Infof("Dropped location_name column from %s", table)
			}
		}
	}

	// Rebuild market_orders with UNIQUE constraint if it doesn't already have one
	if !hasUniqueConstraint(db, "market_orders") {
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS market_orders_new (
				id INTEGER, item_type_id TEXT, item_group_type_id TEXT,
				location_id TEXT, quality_level INTEGER, enchantment_level INTEGER,
				albion_id INTEGER,
				unit_price_silver INTEGER, amount INTEGER, auction_type TEXT,
				expires TEXT, upload_identifier TEXT, captured_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				UNIQUE(item_type_id, enchantment_level, location_id, quality_level, auction_type)
			)
		`)
		if err != nil {
			log.Warnf("Failed to create market_orders_new: %v", err)
		} else {
			// Copy keeping only the latest row per unique key
			_, err = db.Exec(`
				INSERT OR REPLACE INTO market_orders_new
				SELECT id, item_type_id, item_group_type_id, location_id, quality_level,
					enchantment_level, NULL, unit_price_silver, amount, auction_type,
					expires, upload_identifier, captured_at
				FROM market_orders
				ORDER BY captured_at ASC
			`)
			if err != nil {
				log.Warnf("Failed to migrate market_orders data: %v", err)
			} else {
				db.Exec(`DROP TABLE market_orders`)
				db.Exec(`ALTER TABLE market_orders_new RENAME TO market_orders`)
				log.Infof("Rebuilt market_orders with UNIQUE constraint")
			}
		}
	}

	// Add albion_id column to market_orders if it doesn't already have one
	if !hasColumn(db, "market_orders", "albion_id") {
		_, err := db.Exec(`ALTER TABLE market_orders ADD COLUMN albion_id INTEGER`)
		if err != nil {
			log.Warnf("Failed to add albion_id column to market_orders: %v", err)
		} else {
			log.Infof("Added albion_id column to market_orders")
		}
	}

	// Add per_item column to market_histories if it doesn't already have one
	if !hasColumn(db, "market_histories", "per_item") {
		_, err := db.Exec(`ALTER TABLE market_histories ADD COLUMN per_item REAL`)
		if err != nil {
			log.Warnf("Failed to add per_item column to market_histories: %v", err)
		} else {
			log.Infof("Added per_item column to market_histories")
		}
	}

	return nil
}

// hasUniqueConstraint checks if a table has any UNIQUE constraint defined
func hasUniqueConstraint(db *sql.DB, tableName string) bool {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA index_list(%s)`, tableName))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			continue
		}
		if unique == 1 {
			return true
		}
	}
	return false
}

// hasColumn checks if a table has a specific column
func hasColumn(db *sql.DB, tableName, columnName string) bool {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, tableName))
	if err != nil {
		log.Debugf("Failed to query table info for %s: %v", tableName, err)
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notnull int
		var dfltValue interface{}
		var pk int

		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			log.Debugf("Failed to scan table info: %v", err)
			continue
		}

		if name == columnName {
			return true
		}
	}

	return false
}


// loadAlbionIDMap parses .cache/items.txt to build item_type_id -> albion_id mapping.
// items.txt format: "albion_id : item_type_id (padded) : Display Name"
func loadAlbionIDMap() map[string]int32 {
	m := make(map[string]int32)
	data, err := os.ReadFile(".cache/items.txt")
	if err != nil {
		return m
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		parts := strings.Split(sc.Text(), ":")
		if len(parts) < 3 {
			continue
		}
		numStr := strings.TrimSpace(parts[0])
		itemID := strings.TrimSpace(parts[1])
		if numStr == "" || itemID == "" {
			continue
		}
		n, err := strconv.ParseInt(numStr, 10, 32)
		if err != nil {
			continue
		}
		m[itemID] = int32(n)
	}
	return m
}

// populateLocations inserts or updates the location mappings
func populateLocations(db *sql.DB) error {
	for id, name := range locationMap {
		_, err := db.Exec(`
			INSERT OR REPLACE INTO locations (location_id, location_name)
			VALUES (?, ?)
		`, id, name)
		if err != nil {
			return fmt.Errorf("failed to insert location %s: %w", id, err)
		}
	}
	return nil
}

// processWriteQueue processes database writes serially from the queue
func (u *sqliteUploader) processWriteQueue() {
	for req := range u.writeQueue {
		u.executeWrite(req.body, req.topic, req.identifier)
	}
}

// executeWrite performs the actual database write operation
func (u *sqliteUploader) executeWrite(body []byte, topic string, identifier string) {
	tx, err := u.db.Begin()
	if err != nil {
		log.Errorf("Failed to begin transaction: %v", err)
		return
	}
	defer tx.Rollback()

	var insertErr error

	switch topic {
	case lib.NatsMarketOrdersIngest:
		insertErr = u.insertMarketOrders(tx, body, identifier)

	case lib.NatsMarketHistoriesIngest:
		insertErr = u.insertMarketHistories(tx, body, identifier)

	case lib.NatsGoldPricesIngest:
		insertErr = u.insertGoldPrices(tx, body, identifier)

	case lib.NatsMapDataIngest:
		insertErr = u.insertMapData(tx, body, identifier)

	case lib.NatsBanditEvent:
		insertErr = u.insertBanditEvent(tx, body, identifier)

	case lib.NatsSkillData:
		insertErr = u.insertSkills(tx, body, identifier)

	case lib.NatsMarketNotifications:
		insertErr = u.insertMarketNotifications(tx, body, identifier)

	default:
		log.Warnf("Unknown topic for SQLite insert: %v", topic)
		return
	}

	if insertErr != nil {
		log.Errorf("Error inserting data into %v: %v", topic, insertErr)
		return
	}

	if err := tx.Commit(); err != nil {
		log.Errorf("Failed to commit transaction: %v", err)
		return
	}

	log.Infof("Successfully inserted data for topic %v", topic)
}

// sendToIngest queues a write request to be processed serially
func (u *sqliteUploader) sendToIngest(body []byte, topic string, state *albionState, identifier string) {
	select {
	case u.writeQueue <- writeRequest{body: body, topic: topic, identifier: identifier}:
		// Request queued successfully
	default:
		log.Warnf("Write queue full, dropping request for topic %v", topic)
	}
}

func (u *sqliteUploader) insertMarketOrders(tx *sql.Tx, body []byte, identifier string) error {
	var upload lib.MarketUpload
	if err := json.Unmarshal(body, &upload); err != nil {
		return fmt.Errorf("failed to unmarshal MarketUpload: %w", err)
	}

	insertStmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO market_orders (
			id, item_type_id, item_group_type_id, location_id, quality_level,
			enchantment_level, albion_id, unit_price_silver, amount, auction_type, expires, upload_identifier
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer insertStmt.Close()

	updateStmt, err := tx.Prepare(`
		UPDATE market_orders SET
			id = ?, item_group_type_id = ?, albion_id = ?,
			unit_price_silver = ?, amount = ?, expires = ?, upload_identifier = ?
		WHERE item_type_id = ? AND enchantment_level = ? AND location_id = ?
			AND quality_level = ? AND auction_type = ?
			AND unit_price_silver > ?
	`)
	if err != nil {
		return err
	}
	defer updateStmt.Close()

	for _, order := range upload.Orders {
		albionID := u.albionIDMap[order.ItemID]
		price := order.Price / 10000
		_, err := insertStmt.Exec(
			order.ID, order.ItemID, order.GroupTypeId, order.LocationID, order.QualityLevel,
			order.EnchantmentLevel, albionID, price, order.Amount, order.AuctionType, order.Expires, identifier,
		)
		if err != nil {
			return fmt.Errorf("failed to insert market order: %w", err)
		}
		_, err = updateStmt.Exec(
			order.ID, order.GroupTypeId, albionID,
			price, order.Amount, order.Expires, identifier,
			order.ItemID, order.EnchantmentLevel, order.LocationID,
			order.QualityLevel, order.AuctionType,
			price,
		)
		if err != nil {
			return fmt.Errorf("failed to update market order: %w", err)
		}
	}

	return nil
}

func (u *sqliteUploader) insertMarketHistories(tx *sql.Tx, body []byte, identifier string) error {
	var upload lib.MarketHistoriesUpload
	if err := json.Unmarshal(body, &upload); err != nil {
		return fmt.Errorf("failed to unmarshal MarketHistoriesUpload: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO market_histories (
			albion_id, location_id, quality_level, timescale, item_amount,
			silver_amount, per_item, timestamp, upload_identifier
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	const dotNetTicksPerSecond = 10_000_000
	const dotNetEpochOffset = 621_355_968_000_000_000

	for _, history := range upload.Histories {
		silverAmount := float64(history.SilverAmount) / 10000.0
		var perItem float64
		if history.ItemAmount > 0 {
			perItem = silverAmount / float64(history.ItemAmount)
		}
		epochTimestamp := int64((history.Timestamp - dotNetEpochOffset) / dotNetTicksPerSecond)

		_, err := stmt.Exec(
			upload.AlbionId, upload.LocationId, upload.QualityLevel, upload.Timescale,
			history.ItemAmount, silverAmount, perItem, epochTimestamp, identifier,
		)
		if err != nil {
			return fmt.Errorf("failed to insert market history: %w", err)
		}
	}

	return nil
}

func (u *sqliteUploader) insertGoldPrices(tx *sql.Tx, body []byte, identifier string) error {
	var upload lib.GoldPricesUpload
	if err := json.Unmarshal(body, &upload); err != nil {
		return fmt.Errorf("failed to unmarshal GoldPricesUpload: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO gold_prices (price, timestamp, upload_identifier)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	const dotNetTicksPerSecond = 10_000_000
	const dotNetEpochOffset = 621_355_968_000_000_000

	for i, price := range upload.Prices {
		epochTimestamp := (upload.TimeStamps[i] - dotNetEpochOffset) / dotNetTicksPerSecond
		_, err := stmt.Exec(price/10000, epochTimestamp, identifier)
		if err != nil {
			return fmt.Errorf("failed to insert gold price: %w", err)
		}
	}

	return nil
}

func (u *sqliteUploader) insertMapData(tx *sql.Tx, body []byte, identifier string) error {
	var upload lib.MapDataUpload
	if err := json.Unmarshal(body, &upload); err != nil {
		return fmt.Errorf("failed to unmarshal MapDataUpload: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO map_data (
			zone_id, building_type, available_food, reward, available_silver,
			owner, public_fee, associate_fee, coordinate_x, coordinate_y,
			durability, permission, upload_identifier
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// All fields are parallel arrays indexed by building
	for i := range upload.BuildingType {
		x, y := 0, 0
		if i < len(upload.Coordinates) {
			x = upload.Coordinates[i][0]
			y = upload.Coordinates[i][1]
		}

		_, err := stmt.Exec(
			upload.ZoneID, upload.BuildingType[i], upload.AvailableFood[i],
			upload.Reward[i], upload.AvailableSilver[i],
			upload.Owners[i], upload.PublicFee[i], upload.AssociateFee[i],
			x, y, upload.Durability[i], upload.Permission[i], identifier,
		)
		if err != nil {
			return fmt.Errorf("failed to insert map data: %w", err)
		}
	}

	return nil
}

func (u *sqliteUploader) insertBanditEvent(tx *sql.Tx, body []byte, identifier string) error {
	var event lib.BanditEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("failed to unmarshal BanditEvent: %w", err)
	}

	_, err := tx.Exec(`
		INSERT INTO bandit_events (event_time, phase, upload_identifier)
		VALUES (?, ?, ?)
	`, event.EventTime, event.Phase, identifier)

	return err
}

func (u *sqliteUploader) insertSkills(tx *sql.Tx, body []byte, identifier string) error {
	var upload lib.SkillsUpload
	if err := json.Unmarshal(body, &upload); err != nil {
		return fmt.Errorf("failed to unmarshal SkillsUpload: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO skills (
			character_id, character_name, skill_id, level, percent_next_level, fame, upload_identifier
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, skill := range upload.Skills {
		_, err := stmt.Exec(
			upload.CharacterId, upload.CharacterName, skill.ID, skill.Level,
			skill.PercentNextLevel, skill.Fame, identifier,
		)
		if err != nil {
			return fmt.Errorf("failed to insert skill: %w", err)
		}
	}

	return nil
}

func (u *sqliteUploader) insertMarketNotifications(tx *sql.Tx, body []byte, identifier string) error {
	var upload lib.MarketNotificationUpload
	if err := json.Unmarshal(body, &upload); err != nil {
		return fmt.Errorf("failed to unmarshal MarketNotificationUpload: %w", err)
	}

	notificationType := string(upload.Type)

	// Handle both notification types (SalesNotification and ExpiryNotification)
	var mailID int
	var itemID, locationID, expires string
	var amount, price int
	var totalAfterTaxes sql.NullFloat64
	var sold sql.NullInt64

	switch notification := upload.Notification.(type) {
	case *lib.MarketSellNotification:
		mailID = notification.MailID
		itemID = notification.ItemID
		locationID = notification.LocationID
		amount = notification.Amount
		expires = notification.Expires
		price = notification.Price / 10000
		totalAfterTaxes.Float64 = float64(notification.TotalAfterTaxes) / 10000
		totalAfterTaxes.Valid = true

	case *lib.MarketExpiryNotification:
		mailID = notification.MailID
		itemID = notification.ItemID
		locationID = notification.LocationID
		amount = notification.Amount
		expires = notification.Expires
		price = notification.Price / 10000
		sold.Int64 = int64(notification.Sold)
		sold.Valid = true

	default:
		return fmt.Errorf("unknown notification type in MarketNotificationUpload")
	}

	_, err := tx.Exec(`
		INSERT INTO market_notifications (
			character_id, character_name, notification_type, mail_id, item_type_id,
			location_id, amount, expires, unit_price_silver, total_after_taxes, sold, upload_identifier
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, upload.CharacterId, upload.CharacterName, notificationType, mailID, itemID,
		locationID, amount, expires, price, totalAfterTaxes, sold, identifier)

	return err
}

// cleanupExpiredData removes records that have exceeded their expiration time
func cleanupExpiredData(db *sql.DB) error {
	// Delete expired market orders (expires is RFC3339 timestamp string)
	result, err := db.Exec(`
		DELETE FROM market_orders
		WHERE expires IS NOT NULL AND expires < datetime('now')
	`)
	if err != nil {
		return fmt.Errorf("failed to cleanup market_orders: %w", err)
	}

	ordersDeleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected for market_orders: %w", err)
	}

	// Delete expired market notifications (expires is RFC3339 timestamp string)
	result, err = db.Exec(`
		DELETE FROM market_notifications
		WHERE expires IS NOT NULL AND expires < datetime('now')
	`)
	if err != nil {
		return fmt.Errorf("failed to cleanup market_notifications: %w", err)
	}

	notificationsDeleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected for market_notifications: %w", err)
	}

	if ordersDeleted+notificationsDeleted > 0 {
		log.Infof("Cleaned up %d expired market orders and %d expired notifications", ordersDeleted, notificationsDeleted)
	}

	return nil
}
