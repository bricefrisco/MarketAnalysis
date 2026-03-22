package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type zvzWeapon struct {
	Name  string
	ID    string
	Tiers []int // effective tiers, e.g. [7, 8]
}

var zvzWeapons = []zvzWeapon{
	{"Great Hammer", "2H_HAMMER", []int{7, 8}},
	{"Heavy Mace", "2H_MACE", []int{7, 8}},
	{"Staff of Balance", "2H_ROCKSTAFF_KEEPER", []int{7, 8}},
	{"Oathkeepers", "2H_DUALMACE_AVALON", []int{7, 8}},
	{"Lifecurse Staff", "MAIN_CURSEDSTAFF_UNDEAD", []int{7, 8}},
	{"Taproot", "OFF_TOTEM_KEEPER", []int{7, 8}},
	{"Rotcaller Staff", "MAIN_CURSEDSTAFF_CRYSTAL", []int{7, 8}},
	{"Rootbound Staff", "2H_SHAPESHIFTER_SET2", []int{7, 8}},
	{"Carving Sword", "2H_CLEAVER_HELL", []int{7, 8, 9}},
	{"Realmbreaker", "2H_AXE_AVALON", []int{7, 8, 9}},
	{"Spiked Gauntlets", "2H_KNUCKLES_SET3", []int{7, 8, 9}},
	{"Battle Bracers", "2H_KNUCKLES_SET2", []int{7, 8, 9}},
	{"Hellfire Hands", "2H_KNUCKLES_HELL", []int{7, 8, 9}},
	{"Permafrost Prism", "2H_ICECRYSTAL_UNDEAD", []int{7, 8, 9}},
	{"Bloodletter", "MAIN_RAPIER_MORGANA", []int{7, 8, 9}},
	{"Facebreaker", "OFF_SPIKEDSHIELD_MORGANA", []int{7, 8, 9}},
}

// zvzKey uniquely identifies a (base item_type_id, enchantment) pair we care about.
// Effective tier = base_tier + enchantment (e.g. T4@3 = effective T7).
type zvzKey struct {
	ItemTypeID  string
	Enchantment int
}

var zvzItemSet map[zvzKey]bool

func init() {
	zvzItemSet = make(map[zvzKey]bool)
	for _, w := range zvzWeapons {
		for _, eff := range w.Tiers {
			for enc := 0; enc <= 4; enc++ {
				base := eff - enc
				if base >= 4 && base <= 8 {
					zvzItemSet[zvzKey{fmt.Sprintf("T%d_%s", base, w.ID), enc}] = true
				}
			}
		}
	}
}

// ScanEvent is the JSON message pushed to scanner WebSocket clients.
type ScanEvent struct {
	ItemTypeID  string `json:"item_type_id"`
	Enchantment int    `json:"enchantment_level"`
	Quality     int    `json:"quality_level"`
}

// ScanHub manages WebSocket connections for the scanner page.
type ScanHub struct {
	mu             sync.Mutex
	clients        map[*websocket.Conn]bool
	upgrader       websocket.Upgrader
	albionIDToItem map[int32]string // albion_id -> item_type_id (may include @N suffix)
}

func NewScanHub() *ScanHub {
	return &ScanHub{
		clients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// SetItemMap provides the hub with the reverse albion_id -> item_type_id mapping,
// built from the items.txt cache after it is loaded.
func (h *ScanHub) SetItemMap(m map[int32]string) {
	h.mu.Lock()
	h.albionIDToItem = m
	h.mu.Unlock()
}

// Notify resolves albionId to an item_type_id, checks if it is a tracked ZVZ item,
// and broadcasts to all connected scanner clients.
// Only qualities 2–5 (Good, Outstanding, Excellent, Masterpiece) are broadcast.
func (h *ScanHub) Notify(albionId int32, quality int) {
	if quality < 1 || quality > 5 {
		return
	}

	h.mu.Lock()
	itemTypeID, ok := h.albionIDToItem[albionId]
	h.mu.Unlock()
	if !ok {
		return
	}

	// Items with enchantment have @N in their item_type_id (e.g. "T4_2H_HAMMER@3").
	// Strip the suffix to get the bare ID and parse the enchantment level.
	bareID := itemTypeID
	enc := 0
	if i := strings.LastIndex(itemTypeID, "@"); i != -1 {
		bareID = itemTypeID[:i]
		if n, err := strconv.Atoi(itemTypeID[i+1:]); err == nil {
			enc = n
		}
	}

	if !zvzItemSet[zvzKey{bareID, enc}] {
		return
	}

	msg, _ := json.Marshal(ScanEvent{ItemTypeID: bareID, Enchantment: enc, Quality: quality})
	h.mu.Lock()
	defer h.mu.Unlock()
	for conn := range h.clients {
		conn.WriteMessage(websocket.TextMessage, msg)
	}
}

// ServeWS upgrades an HTTP connection to WebSocket and keeps it registered until closed.
func (h *ScanHub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	// Block until client disconnects; discard any incoming messages.
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
