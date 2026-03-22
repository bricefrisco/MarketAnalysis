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
	"time"
)

// ── Internal recipe types ──────────────────────────────────────────────────

type recipe struct {
	ItemTypeID  string
	Tier        int
	Category    string // "weapon" | "armor" | "offhand"
	SubCategory string // e.g. "sword", "cloth_armor"
	Resources   []recipeRes
}

type recipeRes struct {
	ItemTypeID string
	Count      int
	NoReturn   bool // true for artefacts (@maxreturnamount="0")
}

// ── API response types ─────────────────────────────────────────────────────

type CraftingResource struct {
	ItemTypeID string  `json:"item_type_id"`
	Name       string  `json:"name"`
	Count      int     `json:"count"`
	NoReturn   bool    `json:"no_return"`
	AvgPrice   float64 `json:"avg_price"`
}

type CraftingItem struct {
	ItemTypeID   string             `json:"item_type_id"`
	Name         string             `json:"name"`
	Tier         int                `json:"tier"`
	Category     string             `json:"category"`
	SubCategory  string             `json:"sub_category"`
	Resources    []CraftingResource `json:"resources"`
	AvgSellPrice float64            `json:"avg_sell_price"`
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
	}

	type rawItem struct {
		Uniquename string          `json:"@uniquename"`
		Tier       string          `json:"@tier"`
		ShopCat    string          `json:"@shopcategory"`
		ShopSub    string          `json:"@shopsubcategory1"`
		CraftReqs  json.RawMessage `json:"craftingrequirements"`
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
			ItemTypeID:  item.Uniquename,
			Tier:        tier,
			Category:    canonicalCat(item.ShopCat),
			SubCategory: item.ShopSub,
			Resources:   resources,
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
				ItemTypeID:  fmt.Sprintf("%s@%d", r.ItemTypeID, enc),
				Tier:        r.Tier,
				Category:    r.Category,
				SubCategory: r.SubCategory,
				Resources:   encResources,
			})
		}
	}

	return recipes, nil
}

// ── DB price lookup ────────────────────────────────────────────────────────

// getAvgPrices fetches 4-week average prices for a set of albion_ids in one city.
// Queries in batches to stay within SQLite's variable limit.
func getAvgPrices(db *sql.DB, albionIDs []int32, locationID string) (map[int32]float64, error) {
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
			SELECT albion_id, AVG(per_item)
			FROM market_histories
			WHERE albion_id IN (%s)
			  AND location_id = ?
			  AND quality_level BETWEEN 1 AND 4
			  AND timescale IN (1, 2)
			  AND timestamp >= strftime('%%s', 'now', '-28 days')
			  AND per_item > 0
			GROUP BY albion_id
		`, strings.Join(ph, ","))

		rows, err := db.QueryContext(context.Background(), query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int32
			var avg float64
			if err := rows.Scan(&id, &avg); err == nil {
				prices[id] = avg
			}
		}
		rows.Close()
	}
	return prices, nil
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

	// Collect every unique item_type_id we need (outputs + resources)
	idSet := make(map[string]bool)
	for _, rec := range recipes {
		idSet[rec.ItemTypeID] = true
		for _, res := range rec.Resources {
			idSet[res.ItemTypeID] = true
		}
	}

	// Map item_type_id → albion_id (using the global cache populated by loadItemsCache)
	idToAlbion := make(map[string]int32)
	var albionIDs []int32
	for id := range idSet {
		if aid, ok := albionIDCache[id]; ok {
			idToAlbion[id] = aid
			albionIDs = append(albionIDs, aid)
		}
	}

	priceMap, err := getAvgPrices(db, albionIDs, locationID)
	if err != nil {
		http.Error(w, "Failed to fetch prices: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Build response items
	result := make([]CraftingItem, 0, len(recipes))
	for _, rec := range recipes {
		sellAlbID := idToAlbion[rec.ItemTypeID]
		sellPrice := priceMap[sellAlbID]

		resources := make([]CraftingResource, 0, len(rec.Resources))
		for _, res := range rec.Resources {
			resAlbID := idToAlbion[res.ItemTypeID]
			resources = append(resources, CraftingResource{
				ItemTypeID: res.ItemTypeID,
				Name:       items[res.ItemTypeID],
				Count:      res.Count,
				NoReturn:   res.NoReturn,
				AvgPrice:   priceMap[resAlbID],
			})
		}

		name := items[rec.ItemTypeID]
		if name == "" {
			name = rec.ItemTypeID
		}

		result = append(result, CraftingItem{
			ItemTypeID:   rec.ItemTypeID,
			Name:         name,
			Tier:         rec.Tier,
			Category:     rec.Category,
			SubCategory:  rec.SubCategory,
			Resources:    resources,
			AvgSellPrice: sellPrice,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items":       result,
		"location_id": locationID,
	})
}
