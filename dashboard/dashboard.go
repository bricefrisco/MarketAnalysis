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
func createMux(db *sql.DB, items map[string]string, hub *ScanHub) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// WebSocket routes
		if r.URL.Path == "/ws/scanner" {
			hub.ServeWS(w, r)
			return
		}

		// API routes
		if strings.HasPrefix(r.URL.Path, "/api/") {
			switch r.URL.Path {
			case "/api/items":
				handleItems(w, items)
				return
			case "/api/search":
				handleSearch(w, r, items)
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
func Start(dbPath string, port string, hub *ScanHub) {
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

	// Build reverse map (albion_id -> item_type_id) for the scanner hub.
	if albionIDCache != nil {
		rev := make(map[int32]string, len(albionIDCache))
		for itemID, id := range albionIDCache {
			rev[id] = itemID
		}
		hub.SetItemMap(rev)
	}

	handler := createMux(db, items, hub)

	addr := ":" + port
	fmt.Printf("  [dashboard] 🚀 listening at http://localhost:%s\n", port)
	if err := http.ListenAndServe(addr, handler); err != nil {
		fmt.Printf("  [dashboard] server error: %v\n", err)
	}
}
