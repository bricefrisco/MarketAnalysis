package dashboard

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// staticFiles will be set by the main package via SetStaticFiles
var staticFiles embed.FS

// SetStaticFiles allows the main package to inject the embedded files
func SetStaticFiles(fs embed.FS) {
	staticFiles = fs
}

// locationMap matches the main client's city IDs
var locationMap = map[string]string{
	"0007": "Thetford",
	"1002": "Lymhurst",
	"2004": "Bridgewatch",
	"3008": "Martlock",
	"4002": "Fort Sterling",
}

var itemCache map[string]string
var itemCacheTime time.Time
var albionIDCache map[string]int32 // item_type_id -> albion_id

type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Order struct {
	ItemTypeID  string   `json:"item_type_id"`
	City        string   `json:"city"`
	Quality     int      `json:"quality_level"`
	Price       int64    `json:"unit_price_silver"`
	Supply      int64    `json:"amount"`
	AuctionType string   `json:"auction_type"`
	CapturedAt  string   `json:"captured_at"`
	WeeklyAvg   *float64 `json:"weekly_avg"`
}

type MarketPrice struct {
	City       string `json:"City"`
	Quality    int    `json:"Quality"`
	MinPrice   int64  `json:"MinPrice"`
	AvgPrice   int64  `json:"AvgPrice"`
	Supply     int64  `json:"Supply"`
	NumOrders  int    `json:"NumOrders"`
	PriceClass string `json:"PriceClass"`
}

type MarketHistoryPoint struct {
	Timestamp int64   `json:"timestamp"`
	City      string  `json:"city"`
	PerItem   float64 `json:"per_item"`
	Timescale int     `json:"timescale"`
}

func loadItemsCache() (map[string]string, error) {
	if itemCache != nil && time.Since(itemCacheTime) < time.Hour {
		return itemCache, nil
	}

	cacheDir := ".cache"
	cachePath := filepath.Join(cacheDir, "items.txt")

	if info, err := os.Stat(cachePath); err == nil {
		if time.Since(info.ModTime()) < time.Hour {
			return loadItemsFromFile(cachePath)
		}
	}

	fmt.Println("  [dashboard] fetching items from GitHub...")
	resp, err := http.Get("https://raw.githubusercontent.com/ao-data/ao-bin-dumps/refs/heads/master/formatted/items.txt")
	if err != nil {
		if cached, err := loadItemsFromFile(cachePath); err == nil {
			fmt.Println("  [dashboard] using stale items cache (fetch failed)")
			return cached, nil
		}
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to fetch items: HTTP %d", resp.StatusCode)
	}

	os.MkdirAll(cacheDir, 0755)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	os.WriteFile(cachePath, body, 0644)

	itemCache = parseItems(body)
	itemCacheTime = time.Now()
	return itemCache, nil
}

func loadItemsFromFile(path string) (map[string]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	itemCache = parseItems(body)
	itemCacheTime = time.Now()
	return itemCache, nil
}

func parseItems(data []byte) map[string]string {
	items := make(map[string]string)
	albionIDs := make(map[string]int32)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Format: "albion_id : ItemID (padded) : Display Name"
		parts := strings.Split(line, ":")
		if len(parts) >= 3 {
			numStr := strings.TrimSpace(parts[0])
			itemID := strings.TrimSpace(parts[1])
			// Join the rest in case display name has colons
			displayName := strings.TrimSpace(strings.Join(parts[2:], ":"))
			if itemID != "" && displayName != "" {
				items[itemID] = displayName
				if n, err := strconv.ParseInt(numStr, 10, 32); err == nil {
					albionIDs[itemID] = int32(n)
				}
			}
		}
	}
	albionIDCache = albionIDs
	fmt.Printf("  [dashboard] parsed %d items\n", len(items))
	return items
}

func findItemsByName(query string, items map[string]string) []Item {
	query = strings.ToLower(query)
	var results []Item
	for name, id := range items {
		if strings.Contains(name, query) {
			results = append(results, Item{id, name})
		}
	}
	return results
}

// splitItemID parses "T4_MAIN_HOLYSTAFF_AVALON@3" into ("T4_MAIN_HOLYSTAFF_AVALON", 3).
// Items without an enchantment suffix return enchantment 0.
func splitItemID(id string) (baseID string, enchantment int) {
	if i := strings.LastIndex(id, "@"); i != -1 {
		enc, err := strconv.Atoi(id[i+1:])
		if err == nil {
			return id[:i], enc
		}
	}
	return id, 0
}

func getMarketData(db *sql.DB, itemID string) ([]MarketPrice, error) {
	baseID, enchantment := splitItemID(itemID)

	query := `
		SELECT
			COALESCE(l.location_name, m.location_id) AS city,
			m.quality_level,
			MIN(m.unit_price_silver) AS min_price,
			AVG(m.unit_price_silver) AS avg_price,
			SUM(m.amount) AS total_supply,
			COUNT(*) AS num_orders
		FROM market_orders m
		LEFT JOIN locations l ON m.location_id = l.location_id
		WHERE m.item_type_id = ?
			AND m.enchantment_level = ?
			AND m.auction_type = 'offer'
			AND m.location_id IN ('0007','1002','2004','3008','4002')
		GROUP BY m.location_id, m.quality_level
		ORDER BY m.location_id, m.quality_level
	`

	fmt.Printf("  [dashboard] querying market data for item: %s (enchantment: %d)\n", baseID, enchantment)
	rows, err := db.QueryContext(context.Background(), query, baseID, enchantment)
	if err != nil {
		fmt.Printf("  [dashboard] query error: %v\n", err)
		return nil, err
	}
	defer rows.Close()

	var results []MarketPrice
	for rows.Next() {
		var mp MarketPrice
		var city interface{}
		var avgPrice float64
		err := rows.Scan(&city, &mp.Quality, &mp.MinPrice, &avgPrice, &mp.Supply, &mp.NumOrders)
		if err != nil {
			return nil, err
		}
		mp.AvgPrice = int64(avgPrice)
		if city != nil {
			mp.City = fmt.Sprintf("%s", city)
		}
		results = append(results, mp)
	}

	cheapestByQuality := make(map[int]int64)
	expensiveByQuality := make(map[int]int64)

	for i := range results {
		q := results[i].Quality
		if _, ok := cheapestByQuality[q]; !ok {
			cheapestByQuality[q] = results[i].MinPrice
			expensiveByQuality[q] = results[i].MinPrice
		} else {
			if results[i].MinPrice < cheapestByQuality[q] {
				cheapestByQuality[q] = results[i].MinPrice
			}
			if results[i].MinPrice > expensiveByQuality[q] {
				expensiveByQuality[q] = results[i].MinPrice
			}
		}
	}

	for i := range results {
		q := results[i].Quality
		if results[i].MinPrice == cheapestByQuality[q] {
			results[i].PriceClass = "cheapest"
		} else if results[i].MinPrice == expensiveByQuality[q] {
			results[i].PriceClass = "expensive"
		}
	}

	return results, nil
}

func getRecentOrders(db *sql.DB, limit int) ([]Order, error) {
	query := `
		SELECT
			m.item_type_id,
			COALESCE(l.location_name, m.location_id) AS city,
			m.quality_level,
			m.unit_price_silver,
			m.amount,
			m.auction_type,
			m.captured_at,
			CASE WHEN m.albion_id > 0 THEN (
				SELECT AVG(h.per_item)
				FROM market_histories h
				WHERE h.albion_id = m.albion_id
					AND h.location_id = m.location_id
					AND h.quality_level = m.quality_level
					AND h.timescale = 1
					AND h.timestamp >= strftime('%s', 'now', '-7 days')
					AND h.per_item > 0
			) ELSE NULL END AS weekly_avg
		FROM market_orders m
		LEFT JOIN locations l ON m.location_id = l.location_id
		WHERE m.auction_type = 'offer'
			AND m.location_id IN ('0007','1002','2004','3008','4002')
		ORDER BY m.captured_at DESC
		LIMIT ?
	`

	rows, err := db.QueryContext(context.Background(), query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		var weeklyAvg sql.NullFloat64
		err := rows.Scan(&o.ItemTypeID, &o.City, &o.Quality, &o.Price, &o.Supply, &o.AuctionType, &o.CapturedAt, &weeklyAvg)
		if err != nil {
			return nil, err
		}
		if weeklyAvg.Valid {
			o.WeeklyAvg = &weeklyAvg.Float64
		}
		orders = append(orders, o)
	}

	return orders, nil
}

func getItemHistory(db *sql.DB, itemID string, timescale int) ([]MarketHistoryPoint, error) {
	// market_histories stores a numeric albion_id from the game's internal ID system,
	// which does not directly correspond to the string item_type_id used in market_orders.
	// Until a mapping table is available, history lookup requires the raw numeric albion_id.
	albionID, err := strconv.ParseInt(itemID, 10, 64)
	if err != nil {
		// Non-numeric ID — no history available yet
		return nil, nil
	}

	query := `
		SELECT
			h.timestamp,
			COALESCE(l.location_name, h.location_id) AS city,
			h.per_item,
			h.timescale
		FROM market_histories h
		LEFT JOIN locations l ON h.location_id = l.location_id
		WHERE h.albion_id = ?
			AND h.timescale = ?
			AND h.location_id IN ('0007','1002','2004','3008','4002')
			AND h.per_item > 0
		ORDER BY h.timestamp ASC
	`

	rows, err := db.QueryContext(context.Background(), query, albionID, timescale)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []MarketHistoryPoint
	for rows.Next() {
		var p MarketHistoryPoint
		if err := rows.Scan(&p.Timestamp, &p.City, &p.PerItem, &p.Timescale); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, nil
}

// API Handlers

func handleItems(w http.ResponseWriter, items map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func handleSearch(w http.ResponseWriter, r *http.Request, items map[string]string) {
	query := r.URL.Query().Get("q")
	results := findItemsByName(query, items)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func handleItemPrices(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	itemID := r.URL.Query().Get("id")
	prices, err := getMarketData(db, itemID)
	if err != nil {
		fmt.Printf("  [dashboard] error fetching prices for %s: %v\n", itemID, err)
		http.Error(w, "Error fetching market data", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(prices)
}

func handleItemHistory(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	itemID := r.URL.Query().Get("id")
	timescale := 0 // default: hours
	if ts := r.URL.Query().Get("timescale"); ts != "" {
		if v, err := strconv.Atoi(ts); err == nil {
			timescale = v
		}
	}

	points, err := getItemHistory(db, itemID, timescale)
	if err != nil {
		http.Error(w, "Error fetching history", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(points)
}

func handleRecentOrders(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
			limit = l
		}
	}

	orders, err := getRecentOrders(db, limit)
	if err != nil {
		http.Error(w, "Error fetching orders", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}

func serveSPA(w http.ResponseWriter, r *http.Request) {
	data, err := staticFiles.ReadFile("web/dist/index.html")
	if err != nil {
		// Try without web/dist prefix (in case fs is mapped differently)
		data, err = staticFiles.ReadFile("dist/index.html")
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// createMux creates a router with proper API/SPA routing
func createMux(db *sql.DB, items map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API routes
		if strings.HasPrefix(r.URL.Path, "/api/") {
			switch r.URL.Path {
			case "/api/items":
				handleItems(w, items)
				return
			case "/api/search":
				handleSearch(w, r, items)
				return
			case "/api/item":
				handleItemPrices(w, r, db)
				return
			case "/api/item/history":
				handleItemHistory(w, r, db)
				return
			case "/api/orders/recent":
				handleRecentOrders(w, r, db)
				return
			default:
				http.NotFound(w, r)
				return
			}
		}

		// Asset routes
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			filePath := r.URL.Path

			// Set correct MIME type based on file extension
			if strings.HasSuffix(filePath, ".js") {
				w.Header().Set("Content-Type", "application/javascript")
			} else if strings.HasSuffix(filePath, ".css") {
				w.Header().Set("Content-Type", "text/css; charset=utf-8")
			} else if strings.HasSuffix(filePath, ".svg") {
				w.Header().Set("Content-Type", "image/svg+xml")
			} else if strings.HasSuffix(filePath, ".png") {
				w.Header().Set("Content-Type", "image/png")
			} else if strings.HasSuffix(filePath, ".jpg") || strings.HasSuffix(filePath, ".jpeg") {
				w.Header().Set("Content-Type", "image/jpeg")
			}

			// The embedded path includes web/dist/ prefix
			embeddedPath := "web/dist" + filePath
			data, err := staticFiles.ReadFile(embeddedPath)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Write(data)
			return
		}

		// Everything else is SPA
		serveSPA(w, r)
	})
}

// Start launches the market dashboard web server
func Start(dbPath string, port string) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Printf("  [dashboard] failed to connect to database: %v\n", err)
		return
	}
	defer db.Close()

	items, err := loadItemsCache()
	if err != nil {
		fmt.Printf("  [dashboard] warning: failed to load items cache: %v\n", err)
		items = make(map[string]string)
	}

	handler := createMux(db, items)

	addr := ":" + port
	fmt.Printf("  [dashboard] 🚀 listening at http://localhost:%s\n", port)
	if err := http.ListenAndServe(addr, handler); err != nil {
		fmt.Printf("  [dashboard] server error: %v\n", err)
	}
}
