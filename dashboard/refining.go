package dashboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

// ── Constants ──────────────────────────────────────────────────────────────

const (
	refineReturnRate = 0.367 // city bonus, no focus
	mammothCapacity  = 25735.0
	refineMarketTax  = 0.065
)

// Raw material inputs per refining step, from items_full.json craftingrequirements.
// All resource types (fiber/ore/wood/hide/rock) share the same counts per tier.
//   T2: 1x raw                  → 1x T2 refined
//   T3: 2x raw + 1x T2 refined  → 1x T3 refined
//   T4: 2x raw + 1x T3 refined  → 1x T4 refined
const (
	rawInputT2 = 1.0
	rawInputT3 = 2.0
	rawInputT4 = 2.0
	rawInputT5 = 3.0
)

// keep is the fraction of each input that is net consumed (1 − return rate).
// The return rate applies to ALL inputs in a refining step — both the raw materials
// and the lower-tier refined input — so costs are computed recursively:
//
//	t2Cost = keep × rawInputT2 × t2Price
//	t3Cost = keep × rawInputT3 × t3Price  +  keep × t2Cost
//	t4Cost = keep × rawInputT4 × t4Price  +  keep × t3Cost
const keep = 1 - refineReturnRate // 0.633

// Weight per unit by tier. Raw and refined share the same weight at each tier.
// Source: items_full.json @weight field.
var itemWeightByTier = map[int]float64{
	2: 0.23,
	3: 0.34,
	4: 0.51,
	5: 0.82,
}

// Base item value of refined resources by tier.
// Source: items_full.json @itemvalue field (consistent across all resource types).
var refinedItemValueByTier = map[int]float64{
	2: 4,
	3: 8,
	4: 16,
	5: 32,
}

// ── Resource definitions ───────────────────────────────────────────────────

type refineResourceDef struct {
	Name          string
	RawID         string // item type suffix, e.g. "FIBER"
	RefinedID     string // e.g. "CLOTH"
	BonusCityID   string
	BonusCityName string
	BuyCityIDs    []string // if non-nil, restrict raw buy cities to this set
}

var refineResources = []refineResourceDef{
	{"Fiber", "FIBER", "CLOTH", "1002", "Lymhurst", []string{"0007", "3008", "2004"}}, // Thetford, Fort Sterling, Bridgewatch
	{"Ore", "ORE", "METALBAR", "0007", "Thetford", []string{"3008", "2004", "4002"}}, // Fort Sterling, Bridgewatch, Martlock
	{"Wood", "WOOD", "PLANKS", "3008", "Fort Sterling", []string{"0007", "1002", "4002"}}, // Thetford, Lymhurst, Martlock
	{"Hide", "HIDE", "LEATHER", "4002", "Martlock", []string{"2004", "0007", "1002"}}, // Bridgewatch, Thetford, Lymhurst
	{"Rock", "ROCK", "STONEBLOCK", "2004", "Bridgewatch", []string{"1002", "4002", "3008"}}, // Lymhurst, Martlock, Fort Sterling
}

var royalCities = []struct{ ID, Name string }{
	{"0007", "Thetford"},
	{"1002", "Lymhurst"},
	{"2004", "Bridgewatch"},
	{"3008", "Fort Sterling"},
	{"4002", "Martlock"},
}

// ── API response type ──────────────────────────────────────────────────────

// RefiningRow is one row: one (resource type, output tier, source city) combination.
type RefiningRow struct {
	Tier             int     `json:"tier"`
	ResourceType     string  `json:"resource_type"`
	BonusCityID      string  `json:"bonus_city_id"`
	BonusCityName    string  `json:"bonus_city_name"`
	RawCityID        string  `json:"raw_city_id"`
	RawCityName      string  `json:"raw_city_name"`
	RawT2BuyPrice    float64 `json:"raw_t2_buy_price"`   // 0 = no data
	RawT3BuyPrice    float64 `json:"raw_t3_buy_price"`
	RawT4BuyPrice    float64 `json:"raw_t4_buy_price"`
	RawT5BuyPrice    float64 `json:"raw_t5_buy_price"`   // only populated for tier=5
	RefinedSellPrice float64 `json:"refined_sell_price"` // 0 = no data
	RawCost          float64 `json:"raw_cost"`            // 0 if any price missing
	RefinedItemValue float64 `json:"refined_item_value"`  // base item value from items.json
	ProfitPerItem    float64 `json:"profit_per_item"`
	BatchesPerTrip   int     `json:"batches_per_trip"`
	ProfitPerTrip    float64 `json:"profit_per_trip"`
}

// ── HTTP handler ───────────────────────────────────────────────────────────

func handleRefining(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	// Build item_type_id → albion_id mapping for all needed items.
	typeToAlbID := make(map[string]int32)
	var rawAlbIDs []int32
	var refinedAlbIDs []int32

	for _, res := range refineResources {
		for _, tier := range []int{2, 3, 4, 5} {
			id := fmt.Sprintf("T%d_%s", tier, res.RawID)
			if aid, ok := albionIDCache[id]; ok {
				typeToAlbID[id] = aid
				rawAlbIDs = append(rawAlbIDs, aid)
			}
		}
		for _, tier := range []int{4, 5} {
			id := fmt.Sprintf("T%d_%s", tier, res.RefinedID)
			if aid, ok := albionIDCache[id]; ok {
				typeToAlbID[id] = aid
				refinedAlbIDs = append(refinedAlbIDs, aid)
			}
		}
	}

	cityIDs := make([]string, len(royalCities))
	for i, c := range royalCities {
		cityIDs[i] = c.ID
	}

	// Raw prices: MAX buy order (auction_type='request') per item per city.
	rawBuyMap, err := getMaxBuyPrices(db, rawAlbIDs, cityIDs)
	if err != nil {
		http.Error(w, "failed to fetch raw prices: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Refined sell prices: MIN ask (auction_type='offer') per item per city.
	refinedSellMap, err := getMinSellPrices(db, refinedAlbIDs, cityIDs)
	if err != nil {
		http.Error(w, "failed to fetch refined prices: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// T4 chain: net raw consumed per T4 output.
	// Returns offset lower-tier raw needs: T4=keep×2, T3=keep²×2, T2=keep³×1.
	netT4chain := keep * rawInputT4
	netT3chain := keep * keep * rawInputT3
	netT2chain := keep * keep * keep * rawInputT2
	batchWeightT4 := netT4chain*itemWeightByTier[4] + netT3chain*itemWeightByTier[3] + netT2chain*itemWeightByTier[2]
	batchesPerTripT4 := int(math.Floor(mammothCapacity / batchWeightT4))

	// T5 chain: net raw consumed per T5 output.
	// T5=keep×3, T4=keep²×2, T3=keep³×2, T2=keep⁴×1.
	netT5chain := keep * rawInputT5
	netT4forT5 := keep * keep * rawInputT4
	netT3forT5 := keep * keep * keep * rawInputT3
	netT2forT5 := keep * keep * keep * keep * rawInputT2
	batchWeightT5 := netT5chain*itemWeightByTier[5] + netT4forT5*itemWeightByTier[4] + netT3forT5*itemWeightByTier[3] + netT2forT5*itemWeightByTier[2]
	batchesPerTripT5 := int(math.Floor(mammothCapacity / batchWeightT5))

	var rows []RefiningRow
	for _, res := range refineResources {
		t2AlbID := typeToAlbID[fmt.Sprintf("T2_%s", res.RawID)]
		t3AlbID := typeToAlbID[fmt.Sprintf("T3_%s", res.RawID)]
		t4AlbID := typeToAlbID[fmt.Sprintf("T4_%s", res.RawID)]
		t5AlbID := typeToAlbID[fmt.Sprintf("T5_%s", res.RawID)]
		t4RefinedAlbID := typeToAlbID[fmt.Sprintf("T4_%s", res.RefinedID)]
		t5RefinedAlbID := typeToAlbID[fmt.Sprintf("T5_%s", res.RefinedID)]

		t4RefinedSell := refinedSellMap[t4RefinedAlbID][res.BonusCityID]
		t5RefinedSell := refinedSellMap[t5RefinedAlbID][res.BonusCityID]

		buyCities := royalCities
		if len(res.BuyCityIDs) > 0 {
			buyCities = nil
			for _, c := range royalCities {
				for _, id := range res.BuyCityIDs {
					if c.ID == id {
						buyCities = append(buyCities, c)
						break
					}
				}
			}
		}

		for _, city := range buyCities {
			t2Buy := rawBuyMap[t2AlbID][city.ID]
			t3Buy := rawBuyMap[t3AlbID][city.ID]
			t4Buy := rawBuyMap[t4AlbID][city.ID]
			t5Buy := rawBuyMap[t5AlbID][city.ID]

			// T4 chain cost
			rawCostT4 := 0.0
			if t2Buy > 0 && t3Buy > 0 && t4Buy > 0 {
				t2Cost := keep * rawInputT2 * t2Buy
				t3Cost := keep*rawInputT3*t3Buy + keep*t2Cost
				t4Cost := keep*rawInputT4*t4Buy + keep*t3Cost
				rawCostT4 = t4Cost
			}
			profitT4 := 0.0
			if rawCostT4 > 0 && t4RefinedSell > 0 {
				profitT4 = t4RefinedSell*(1-refineMarketTax) - rawCostT4
			}

			rows = append(rows, RefiningRow{
				Tier:             4,
				ResourceType:     res.Name,
				BonusCityID:      res.BonusCityID,
				BonusCityName:    res.BonusCityName,
				RawCityID:        city.ID,
				RawCityName:      city.Name,
				RawT2BuyPrice:    t2Buy,
				RawT3BuyPrice:    t3Buy,
				RawT4BuyPrice:    t4Buy,
				RefinedSellPrice: t4RefinedSell,
				RawCost:          rawCostT4,
				RefinedItemValue: refinedItemValueByTier[4],
				ProfitPerItem:    profitT4,
				BatchesPerTrip:   batchesPerTripT4,
				ProfitPerTrip:    profitT4 * float64(batchesPerTripT4),
			})

			// T5 chain cost
			rawCostT5 := 0.0
			if t2Buy > 0 && t3Buy > 0 && t4Buy > 0 && t5Buy > 0 {
				t2Cost := keep * rawInputT2 * t2Buy
				t3Cost := keep*rawInputT3*t3Buy + keep*t2Cost
				t4Cost := keep*rawInputT4*t4Buy + keep*t3Cost
				t5Cost := keep*rawInputT5*t5Buy + keep*t4Cost
				rawCostT5 = t5Cost
			}
			profitT5 := 0.0
			if rawCostT5 > 0 && t5RefinedSell > 0 {
				profitT5 = t5RefinedSell*(1-refineMarketTax) - rawCostT5
			}

			rows = append(rows, RefiningRow{
				Tier:             5,
				ResourceType:     res.Name,
				BonusCityID:      res.BonusCityID,
				BonusCityName:    res.BonusCityName,
				RawCityID:        city.ID,
				RawCityName:      city.Name,
				RawT2BuyPrice:    t2Buy,
				RawT3BuyPrice:    t3Buy,
				RawT4BuyPrice:    t4Buy,
				RawT5BuyPrice:    t5Buy,
				RefinedSellPrice: t5RefinedSell,
				RawCost:          rawCostT5,
				RefinedItemValue: refinedItemValueByTier[5],
				ProfitPerItem:    profitT5,
				BatchesPerTrip:   batchesPerTripT5,
				ProfitPerTrip:    profitT5 * float64(batchesPerTripT5),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"rows": rows})
}

// ── DB helpers ─────────────────────────────────────────────────────────────

// getMaxBuyPrices returns map[albion_id][location_id] = MAX buy-order price.
func getMaxBuyPrices(db *sql.DB, albionIDs []int32, locationIDs []string) (map[int32]map[string]float64, error) {
	result := make(map[int32]map[string]float64)
	if len(albionIDs) == 0 {
		return result, nil
	}

	ph := make([]string, len(albionIDs))
	args := make([]interface{}, 0, len(albionIDs)+len(locationIDs))
	for i, id := range albionIDs {
		ph[i] = "?"
		args = append(args, id)
	}
	lph := make([]string, len(locationIDs))
	for i, loc := range locationIDs {
		lph[i] = "?"
		args = append(args, loc)
	}

	query := fmt.Sprintf(`
		SELECT albion_id, location_id, MAX(unit_price_silver)
		FROM market_orders
		WHERE albion_id IN (%s)
		  AND location_id IN (%s)
		  AND auction_type = 'request'
		GROUP BY albion_id, location_id
	`, strings.Join(ph, ","), strings.Join(lph, ","))

	rows, err := db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id int32
		var loc string
		var price float64
		if err := rows.Scan(&id, &loc, &price); err == nil {
			if result[id] == nil {
				result[id] = make(map[string]float64)
			}
			result[id][loc] = price
		}
	}
	return result, nil
}

// getMinSellPrices returns map[albion_id][location_id] = MIN sell-order price.
func getMinSellPrices(db *sql.DB, albionIDs []int32, locationIDs []string) (map[int32]map[string]float64, error) {
	result := make(map[int32]map[string]float64)
	if len(albionIDs) == 0 {
		return result, nil
	}

	ph := make([]string, len(albionIDs))
	args := make([]interface{}, 0, len(albionIDs)+len(locationIDs))
	for i, id := range albionIDs {
		ph[i] = "?"
		args = append(args, id)
	}
	lph := make([]string, len(locationIDs))
	for i, loc := range locationIDs {
		lph[i] = "?"
		args = append(args, loc)
	}

	query := fmt.Sprintf(`
		SELECT albion_id, location_id, MIN(unit_price_silver)
		FROM market_orders
		WHERE albion_id IN (%s)
		  AND location_id IN (%s)
		  AND auction_type = 'offer'
		GROUP BY albion_id, location_id
	`, strings.Join(ph, ","), strings.Join(lph, ","))

	rows, err := db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id int32
		var loc string
		var price float64
		if err := rows.Scan(&id, &loc, &price); err == nil {
			if result[id] == nil {
				result[id] = make(map[string]float64)
			}
			result[id][loc] = price
		}
	}
	return result, nil
}

// ── RefiningHub ────────────────────────────────────────────────────────────

type RefiningEvent struct {
	LocationID string `json:"location_id"`
}

// RefiningHub manages WebSocket connections for the refining page.
type RefiningHub struct {
	mu       sync.Mutex
	clients  map[*websocket.Conn]bool
	upgrader websocket.Upgrader
}

func NewRefiningHub() *RefiningHub {
	return &RefiningHub{
		clients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Notify broadcasts a price-change event to all refining WebSocket clients.
func (h *RefiningHub) Notify(locationID string) {
	msg, _ := json.Marshal(RefiningEvent{LocationID: locationID})
	h.mu.Lock()
	defer h.mu.Unlock()
	for conn := range h.clients {
		conn.WriteMessage(websocket.TextMessage, msg)
	}
}

// ServeWS upgrades an HTTP connection to WebSocket and keeps it registered until closed.
func (h *RefiningHub) ServeWS(w http.ResponseWriter, r *http.Request) {
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
