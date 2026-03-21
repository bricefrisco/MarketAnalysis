package client

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ao-data/albiondata-client/lib"
	"github.com/ao-data/albiondata-client/log"
	_ "modernc.org/sqlite"
)

type sqliteUploader struct {
	db *sql.DB
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

	return &sqliteUploader{db: db}, nil
}

// initSchema creates all tables if they don't exist
func initSchema(db *sql.DB) error {
	schemas := []string{
		`CREATE TABLE IF NOT EXISTS market_orders (
			id INTEGER, item_type_id TEXT, item_group_type_id TEXT,
			location_id TEXT, quality_level INTEGER, enchantment_level INTEGER,
			unit_price_silver INTEGER, amount INTEGER, auction_type TEXT,
			expires TEXT, upload_identifier TEXT, captured_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS market_histories (
			albion_id INTEGER, location_id TEXT, quality_level INTEGER,
			timescale INTEGER, item_amount INTEGER, silver_amount INTEGER,
			timestamp INTEGER, upload_identifier TEXT,
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

	return nil
}

func (u *sqliteUploader) sendToIngest(body []byte, topic string, state *albionState, identifier string) {
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

func (u *sqliteUploader) insertMarketOrders(tx *sql.Tx, body []byte, identifier string) error {
	var upload lib.MarketUpload
	if err := json.Unmarshal(body, &upload); err != nil {
		return fmt.Errorf("failed to unmarshal MarketUpload: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO market_orders (
			id, item_type_id, item_group_type_id, location_id, quality_level,
			enchantment_level, unit_price_silver, amount, auction_type, expires, upload_identifier
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, order := range upload.Orders {
		_, err := stmt.Exec(
			order.ID, order.ItemID, order.GroupTypeId, order.LocationID, order.QualityLevel,
			order.EnchantmentLevel, order.Price, order.Amount, order.AuctionType, order.Expires, identifier,
		)
		if err != nil {
			return fmt.Errorf("failed to insert market order: %w", err)
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
			silver_amount, timestamp, upload_identifier
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, history := range upload.Histories {
		_, err := stmt.Exec(
			upload.AlbionId, upload.LocationId, upload.QualityLevel, upload.Timescale,
			history.ItemAmount, history.SilverAmount, history.Timestamp, identifier,
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

	for i, price := range upload.Prices {
		_, err := stmt.Exec(price, upload.TimeStamps[i], identifier)
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
		price = notification.Price
		totalAfterTaxes.Float64 = float64(notification.TotalAfterTaxes)
		totalAfterTaxes.Valid = true

	case *lib.MarketExpiryNotification:
		mailID = notification.MailID
		itemID = notification.ItemID
		locationID = notification.LocationID
		amount = notification.Amount
		expires = notification.Expires
		price = notification.Price
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
