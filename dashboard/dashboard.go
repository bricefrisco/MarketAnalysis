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

type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Order struct {
	ItemTypeID  string `json:"item_type_id"`
	City        string `json:"city"`
	Price       int64  `json:"unit_price_silver"`
	Supply      int64  `json:"amount"`
	AuctionType string `json:"auction_type"`
	CapturedAt  string `json:"captured_at"`
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
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Format: "Number : ItemID (padded) : Display Name"
		// Split by colon
		parts := strings.Split(line, ":")
		if len(parts) >= 3 {
			itemID := strings.TrimSpace(parts[1])
			// Join the rest in case display name has colons
			displayName := strings.TrimSpace(strings.Join(parts[2:], ":"))
			// Map ItemID -> Display Name
			if itemID != "" && displayName != "" {
				items[itemID] = displayName
			}
		}
	}
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

func getMarketData(db *sql.DB, itemID string) ([]MarketPrice, error) {
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
			AND m.auction_type = 'offer'
			AND m.location_id IN ('0007','1002','2004','3008','4002')
		GROUP BY m.location_id, m.quality_level
		ORDER BY m.location_id, m.quality_level
	`

	fmt.Printf("  [dashboard] querying market data for item: %s\n", itemID)
	rows, err := db.QueryContext(context.Background(), query, itemID)
	if err != nil {
		fmt.Printf("  [dashboard] query error: %v\n", err)
		return nil, err
	}
	defer rows.Close()

	var results []MarketPrice
	for rows.Next() {
		var mp MarketPrice
		var city interface{}
		err := rows.Scan(&city, &mp.Quality, &mp.MinPrice, &mp.AvgPrice, &mp.Supply, &mp.NumOrders)
		if err != nil {
			return nil, err
		}
		if city != nil {
			mp.City = city.(string)
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
			m.unit_price_silver,
			m.amount,
			m.auction_type,
			m.captured_at
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
		err := rows.Scan(&o.ItemTypeID, &o.City, &o.Price, &o.Supply, &o.AuctionType, &o.CapturedAt)
		if err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}

	return orders, nil
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
