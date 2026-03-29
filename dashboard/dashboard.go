package dashboard

import (
	"bufio"
	"bytes"
	"database/sql"
	"embed"
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

var itemCache map[string]string
var itemCacheTime time.Time
var albionIDCache map[string]int32 // item_type_id -> albion_id

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

func serveSPA(w http.ResponseWriter, r *http.Request) {
	data, err := staticFiles.ReadFile("web/dist/index.html")
	if err != nil {
		data, err = staticFiles.ReadFile("dist/index.html")
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// Start launches the crafting dashboard web server
func Start(dbPath string, port string, craftingHub *CraftingHub, refiningHub *RefiningHub) {
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

	mux := http.NewServeMux()

	mux.HandleFunc("/ws/crafting", craftingHub.ServeWS)
	mux.HandleFunc("/ws/refining", refiningHub.ServeWS)

	mux.HandleFunc("/api/crafting", func(w http.ResponseWriter, r *http.Request) {
		handleCrafting(w, r, db, items)
	})

	mux.HandleFunc("/api/crafting/prices", func(w http.ResponseWriter, r *http.Request) {
		handleClearPrices(w, r, db)
	})

	mux.HandleFunc("/api/refining", func(w http.ResponseWriter, r *http.Request) {
		handleRefining(w, r, db)
	})

	mux.HandleFunc("/assets/", func(w http.ResponseWriter, r *http.Request) {
		filePath := r.URL.Path
		if strings.HasSuffix(filePath, ".js") {
			w.Header().Set("Content-Type", "application/javascript")
		} else if strings.HasSuffix(filePath, ".css") {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		} else if strings.HasSuffix(filePath, ".svg") {
			w.Header().Set("Content-Type", "image/svg+xml")
		}
		data, err := staticFiles.ReadFile("web/dist" + filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveSPA(w, r)
	})

	addr := ":" + port
	fmt.Printf("  [dashboard] 🚀 listening at http://localhost:%s\n", port)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Printf("  [dashboard] server error: %v\n", err)
	}
}
