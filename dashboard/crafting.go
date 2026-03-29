package dashboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ── Internal recipe types ──────────────────────────────────────────────────

type recipe struct {
	ItemTypeID    string
	Tier          int
	Category      string // "weapon" | "armor" | "offhand"
	SubCategory   string // e.g. "sword", "cloth_armor"
	Resources     []recipeRes
	CraftingFocus float64
}

type recipeRes struct {
	ItemTypeID string
	Count      int
	NoReturn   bool // true for artefacts (@maxreturnamount="0")
}

// ── API response types ─────────────────────────────────────────────────────

type CraftingResource struct {
	ItemTypeID   string  `json:"item_type_id"`
	Name         string  `json:"name"`
	Count        int     `json:"count"`
	NoReturn     bool    `json:"no_return"`
	CurrentPrice float64 `json:"avg_price"` // current MIN ask from market_orders
}

type CraftingItem struct {
	ItemTypeID       string             `json:"item_type_id"`
	Name             string             `json:"name"`
	Tier             int                `json:"tier"`
	Quality          int                `json:"quality"`
	Category         string             `json:"category"`
	SubCategory      string             `json:"sub_category"`
	Resources        []CraftingResource `json:"resources"`
	AvgSellPrice     float64            `json:"avg_sell_price"`     // 4-week avg for this quality
	CurrentSellPrice float64            `json:"current_sell_price"` // current lowest ask from market_orders
	CraftingFocus    float64            `json:"crafting_focus"`     // focus/nutrition consumed per craft
}

// ── Recipe cache ───────────────────────────────────────────────────────────

var recipesCache []recipe
var recipesCacheTime time.Time

func loadRecipes() ([]recipe, error) {
	if recipesCache != nil && time.Since(recipesCacheTime) < 24*time.Hour {
		return recipesCache, nil
	}

	cacheDir := ".cache"
	cachePath := filepath.Join(cacheDir, "items_full.json")

	var data []byte
	if info, err := os.Stat(cachePath); err == nil && time.Since(info.ModTime()) < 24*time.Hour {
		data, _ = os.ReadFile(cachePath)
	}

	if data == nil {
		fmt.Println("  [crafting] fetching items.json from GitHub (~17 MB)…")
		resp, err := http.Get("https://raw.githubusercontent.com/ao-data/ao-bin-dumps/refs/heads/master/items.json")
		if err != nil {
			return nil, fmt.Errorf("fetch items.json: %w", err)
		}
		defer resp.Body.Close()
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read items.json: %w", err)
		}
		os.MkdirAll(cacheDir, 0755)
		os.WriteFile(cachePath, data, 0644)
	}

	recipes, err := parseRecipes(data)
	if err != nil {
		return nil, err
	}
	fmt.Printf("  [crafting] parsed %d craftable items\n", len(recipes))
	recipesCache = recipes
	recipesCacheTime = time.Now()
	return recipes, nil
}

// ── items.json parser ──────────────────────────────────────────────────────

// Items.json is an XML-to-JSON dump; all XML attributes are @-prefixed.
// craftingrequirements can be a dict or a list (multiple recipe variants).
// craftresource can be a dict or a list (single vs multiple resources).

func parseRecipes(data []byte) ([]recipe, error) {
	type rawRes struct {
		Uniquename      string `json:"@uniquename"`
		Count           string `json:"@count"`
		MaxReturnAmount string `json:"@maxreturnamount"`
	}

	type rawReqs struct {
		CraftResource json.RawMessage `json:"craftresource"`
		CraftingFocus string          `json:"@craftingfocus"`
	}

	type rawEnchantment struct {
		Level    string   `json:"@enchantmentlevel"`
		CraftReqs rawReqs `json:"craftingrequirements"`
	}

	type rawItem struct {
		Uniquename   string          `json:"@uniquename"`
		Tier         string          `json:"@tier"`
		ShopCat      string          `json:"@shopcategory"`
		ShopSub      string          `json:"@shopsubcategory1"`
		CraftReqs    json.RawMessage `json:"craftingrequirements"`
		Enchantments json.RawMessage `json:"enchantments"`
	}

	type rawRoot struct {
		Items struct {
			Weapons   []rawItem `json:"weapon"`
			Equipment []rawItem `json:"equipmentitem"`
		} `json:"items"`
	}

	var root rawRoot
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse items.json: %w", err)
	}

	relevantCats := map[string]bool{
		"weapons": true,
		"armors":  true,
		"head":    true,
		"shoes":   true,
		"offhands": true,
	}

	// normalise craftresource field (dict or array) into []rawRes
	normaliseResources := func(raw json.RawMessage) []rawRes {
		if len(raw) == 0 {
			return nil
		}
		if raw[0] == '[' {
			var arr []rawRes
			json.Unmarshal(raw, &arr)
			return arr
		}
		var single rawRes
		if json.Unmarshal(raw, &single) == nil && single.Uniquename != "" {
			return []rawRes{single}
		}
		return nil
	}

	// normalise craftingrequirements (dict or array) into rawReqs
	normaliseReqs := func(raw json.RawMessage) *rawReqs {
		if len(raw) == 0 {
			return nil
		}
		if raw[0] == '[' {
			var arr []rawReqs
			if json.Unmarshal(raw, &arr) == nil && len(arr) > 0 {
				return &arr[0]
			}
			return nil
		}
		var r rawReqs
		if json.Unmarshal(raw, &r) == nil {
			return &r
		}
		return nil
	}

	canonicalCat := func(shopCat string) string {
		switch shopCat {
		case "weapons":
			return "weapon"
		case "armors", "head", "shoes":
			return "armor"
		case "offhands":
			return "offhand"
		}
		return shopCat
	}

	// focusMap: uniquename -> craftingfocus (base and enchanted variants)
	focusMap := make(map[string]float64)

	var recipes []recipe
	process := func(item rawItem) {
		if !relevantCats[item.ShopCat] {
			return
		}
		tier, err := strconv.Atoi(item.Tier)
		if err != nil || tier < 4 || tier > 8 {
			return
		}
		reqs := normaliseReqs(item.CraftReqs)
		if reqs == nil {
			return
		}
		rawResources := normaliseResources(reqs.CraftResource)
		if len(rawResources) == 0 {
			return
		}

		// Parse base crafting focus
		baseFocus, _ := strconv.ParseFloat(reqs.CraftingFocus, 64)
		if baseFocus > 0 {
			focusMap[item.Uniquename] = baseFocus
		}

		// Parse enchanted variant focus values
		if len(item.Enchantments) > 0 {
			var encWrapper struct {
				Enchantment json.RawMessage `json:"enchantment"`
			}
			if json.Unmarshal(item.Enchantments, &encWrapper) == nil {
				var encs []rawEnchantment
				if len(encWrapper.Enchantment) > 0 && encWrapper.Enchantment[0] == '[' {
					json.Unmarshal(encWrapper.Enchantment, &encs)
				} else {
					var single rawEnchantment
					if json.Unmarshal(encWrapper.Enchantment, &single) == nil {
						encs = []rawEnchantment{single}
					}
				}
				for _, e := range encs {
					f, _ := strconv.ParseFloat(e.CraftReqs.CraftingFocus, 64)
					if f > 0 && e.Level != "" {
						focusMap[fmt.Sprintf("%s@%s", item.Uniquename, e.Level)] = f
					}
				}
			}
		}

		var resources []recipeRes
		for _, r := range rawResources {
			count, _ := strconv.Atoi(r.Count)
			if count <= 0 || r.Uniquename == "" {
				continue
			}
			resources = append(resources, recipeRes{
				ItemTypeID: r.Uniquename,
				Count:      count,
				NoReturn:   r.MaxReturnAmount == "0",
			})
		}
		if len(resources) == 0 {
			return
		}

		recipes = append(recipes, recipe{
			ItemTypeID:    item.Uniquename,
			Tier:          tier,
			Category:      canonicalCat(item.ShopCat),
			SubCategory:   item.ShopSub,
			Resources:     resources,
			CraftingFocus: baseFocus,
		})
	}

	for _, w := range root.Items.Weapons {
		process(w)
	}
	for _, e := range root.Items.Equipment {
		process(e)
	}

	// Derive .1–.4 enchanted variants from base recipes.
	// Artefact resources (no_return) stay the same across enchantment levels;
	// only the regular materials get their enchanted (_LEVEL{N}@{N}) equivalents.
	base := recipes
	for _, r := range base {
		for enc := 1; enc <= 4; enc++ {
			encID := fmt.Sprintf("%s@%d", r.ItemTypeID, enc)
			encResources := make([]recipeRes, len(r.Resources))
			for i, res := range r.Resources {
				if res.NoReturn {
					encResources[i] = res // artefact: same across all enchantment levels
				} else {
					encResources[i] = recipeRes{
						ItemTypeID: fmt.Sprintf("%s_LEVEL%d@%d", res.ItemTypeID, enc, enc),
						Count:      res.Count,
						NoReturn:   false,
					}
				}
			}
			recipes = append(recipes, recipe{
				ItemTypeID:    encID,
				Tier:          r.Tier,
				Category:      r.Category,
				SubCategory:   r.SubCategory,
				Resources:     encResources,
				CraftingFocus: focusMap[encID],
			})
		}
	}

	return recipes, nil
}

// ── DB price lookups ────────────────────────────────────────────────────────

// getCurrentPrices fetches the current lowest ask price for a set of albion_ids in one city.
// Uses market_orders (live order data) rather than historical averages.
func getCurrentPrices(db *sql.DB, albionIDs []int32, locationID string) (map[int32]float64, error) {
	prices := make(map[int32]float64)
	const batchSize = 450

	for i := 0; i < len(albionIDs); i += batchSize {
		end := i + batchSize
		if end > len(albionIDs) {
			end = len(albionIDs)
		}
		batch := albionIDs[i:end]

		ph := make([]string, len(batch))
		args := make([]interface{}, 0, len(batch)+1)
		for j, id := range batch {
			ph[j] = "?"
			args = append(args, id)
		}
		args = append(args, locationID)

		query := fmt.Sprintf(`
			SELECT albion_id, MIN(unit_price_silver)
			FROM market_orders
			WHERE albion_id IN (%s)
			  AND location_id = ?
			  AND auction_type = 'offer'
			GROUP BY albion_id
		`, strings.Join(ph, ","))

		rows, err := db.QueryContext(context.Background(), query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int32
			var price float64
			if err := rows.Scan(&id, &price); err == nil {
				prices[id] = price
			}
		}
		rows.Close()
	}
	return prices, nil
}

// getSellAvgsByQuality fetches 4-week average sell prices per quality for a set of albion_ids.
// Returns map[albion_id]map[quality]avg_price.
func getSellAvgsByQuality(db *sql.DB, albionIDs []int32, locationID string) (map[int32]map[int]float64, error) {
	result := make(map[int32]map[int]float64)
	const batchSize = 450

	for i := 0; i < len(albionIDs); i += batchSize {
		end := i + batchSize
		if end > len(albionIDs) {
			end = len(albionIDs)
		}
		batch := albionIDs[i:end]

		ph := make([]string, len(batch))
		args := make([]interface{}, 0, len(batch)+1)
		for j, id := range batch {
			ph[j] = "?"
			args = append(args, id)
		}
		args = append(args, locationID)

		query := fmt.Sprintf(`
			SELECT albion_id, quality_level, AVG(per_item)
			FROM market_histories
			WHERE albion_id IN (%s)
			  AND location_id = ?
			  AND quality_level BETWEEN 1 AND 4
			  AND timescale IN (1, 2)
			  AND timestamp >= strftime('%%s', 'now', '-28 days')
			  AND per_item > 0
			GROUP BY albion_id, quality_level
		`, strings.Join(ph, ","))

		rows, err := db.QueryContext(context.Background(), query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int32
			var quality int
			var avg float64
			if err := rows.Scan(&id, &quality, &avg); err == nil {
				if result[id] == nil {
					result[id] = make(map[int]float64)
				}
				result[id][quality] = avg
			}
		}
		rows.Close()
	}
	return result, nil
}

// getCurrentSellPricesByQuality fetches the lowest ask price per quality for a set of albion_ids.
// Returns map[albion_id]map[quality]min_price using live market_orders data.
func getCurrentSellPricesByQuality(db *sql.DB, albionIDs []int32, locationID string) (map[int32]map[int]float64, error) {
	result := make(map[int32]map[int]float64)
	const batchSize = 450

	for i := 0; i < len(albionIDs); i += batchSize {
		end := i + batchSize
		if end > len(albionIDs) {
			end = len(albionIDs)
		}
		batch := albionIDs[i:end]

		ph := make([]string, len(batch))
		args := make([]interface{}, 0, len(batch)+1)
		for j, id := range batch {
			ph[j] = "?"
			args = append(args, id)
		}
		args = append(args, locationID)

		query := fmt.Sprintf(`
			SELECT albion_id, quality_level, MIN(unit_price_silver)
			FROM market_orders
			WHERE albion_id IN (%s)
			  AND location_id = ?
			  AND auction_type = 'offer'
			  AND quality_level BETWEEN 1 AND 4
			GROUP BY albion_id, quality_level
		`, strings.Join(ph, ","))

		rows, err := db.QueryContext(context.Background(), query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int32
			var quality int
			var price float64
			if err := rows.Scan(&id, &quality, &price); err == nil {
				if result[id] == nil {
					result[id] = make(map[int]float64)
				}
				result[id][quality] = price
			}
		}
		rows.Close()
	}
	return result, nil
}

// ── HTTP handler ───────────────────────────────────────────────────────────

func handleCrafting(w http.ResponseWriter, r *http.Request, db *sql.DB, items map[string]string) {
	locationID := r.URL.Query().Get("city")
	if locationID == "" {
		locationID = "0007"
	}

	recipes, err := loadRecipes()
	if err != nil {
		http.Error(w, "Failed to load recipes: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Collect unique resource IDs (for current prices) and output IDs (for sell avgs)
	resourceIDSet := make(map[string]bool)
	outputIDSet := make(map[string]bool)
	for _, rec := range recipes {
		outputIDSet[rec.ItemTypeID] = true
		for _, res := range rec.Resources {
			resourceIDSet[res.ItemTypeID] = true
		}
	}

	// Map item_type_id → albion_id for resources
	resourceToAlbion := make(map[string]int32)
	var resourceAlbionIDs []int32
	for id := range resourceIDSet {
		if aid, ok := albionIDCache[id]; ok {
			resourceToAlbion[id] = aid
			resourceAlbionIDs = append(resourceAlbionIDs, aid)
		}
	}

	// Map item_type_id → albion_id for outputs
	outputToAlbion := make(map[string]int32)
	var outputAlbionIDs []int32
	for id := range outputIDSet {
		if aid, ok := albionIDCache[id]; ok {
			outputToAlbion[id] = aid
			outputAlbionIDs = append(outputAlbionIDs, aid)
		}
	}

	// Current prices for resources (MIN ask from market_orders)
	resourcePriceMap, err := getCurrentPrices(db, resourceAlbionIDs, locationID)
	if err != nil {
		http.Error(w, "Failed to fetch resource prices: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 4-week sell averages per quality for output items
	sellAvgMap, err := getSellAvgsByQuality(db, outputAlbionIDs, locationID)
	if err != nil {
		http.Error(w, "Failed to fetch sell averages: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Current lowest ask per quality for output items
	currentSellMap, err := getCurrentSellPricesByQuality(db, outputAlbionIDs, locationID)
	if err != nil {
		http.Error(w, "Failed to fetch current sell prices: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Build response: one row per (recipe, quality)
	result := make([]CraftingItem, 0, len(recipes)*4)
	for _, rec := range recipes {
		name := items[rec.ItemTypeID]
		if name == "" {
			name = rec.ItemTypeID
		}

		resources := make([]CraftingResource, 0, len(rec.Resources))
		for _, res := range rec.Resources {
			resAlbID := resourceToAlbion[res.ItemTypeID]
			resources = append(resources, CraftingResource{
				ItemTypeID:   res.ItemTypeID,
				Name:         items[res.ItemTypeID],
				Count:        res.Count,
				NoReturn:     res.NoReturn,
				CurrentPrice: resourcePriceMap[resAlbID],
			})
		}

		outAlbID := outputToAlbion[rec.ItemTypeID]
		qualityAvgs := sellAvgMap[outAlbID]      // may be nil if no data
		qualityCurrent := currentSellMap[outAlbID] // may be nil if no live orders

		for quality := 1; quality <= 4; quality++ {
			sellPrice := 0.0
			if qualityAvgs != nil {
				sellPrice = qualityAvgs[quality]
			}
			currentSellPrice := 0.0
			if qualityCurrent != nil {
				currentSellPrice = qualityCurrent[quality]
			}
			result = append(result, CraftingItem{
				ItemTypeID:       rec.ItemTypeID,
				Name:             name,
				Tier:             rec.Tier,
				Quality:          quality,
				Category:         rec.Category,
				SubCategory:      rec.SubCategory,
				Resources:        resources,
				AvgSellPrice:     sellPrice,
				CurrentSellPrice: currentSellPrice,
				CraftingFocus:    rec.CraftingFocus,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items":       result,
		"location_id": locationID,
	})
}

// ── Clear prices handler ────────────────────────────────────────────────────

func handleClearPrices(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_, err := db.ExecContext(context.Background(), `DELETE FROM market_orders`)
	if err != nil {
		http.Error(w, "failed to clear prices: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── CraftingHub ─────────────────────────────────────────────────────────────

// CraftingEvent is sent to crafting WebSocket clients when prices change.
type CraftingEvent struct {
	LocationID string `json:"location_id"`
}

// CraftingHub manages WebSocket connections for the crafting page.
type CraftingHub struct {
	mu       sync.Mutex
	clients  map[*websocket.Conn]bool
	upgrader websocket.Upgrader
}

func NewCraftingHub() *CraftingHub {
	return &CraftingHub{
		clients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Notify broadcasts a price-change event for a city to all crafting WebSocket clients.
func (h *CraftingHub) Notify(locationID string) {
	msg, _ := json.Marshal(CraftingEvent{LocationID: locationID})
	h.mu.Lock()
	defer h.mu.Unlock()
	for conn := range h.clients {
		conn.WriteMessage(websocket.TextMessage, msg)
	}
}

// ServeWS upgrades an HTTP connection to WebSocket and keeps it registered until closed.
func (h *CraftingHub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()
	conn.Close()
}
