package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Collection goals decide which cards count toward set completion:
//
//	complete = base cards only; master = base + first parallel (_p1);
//	wizard   = everything (all parallels).
const (
	goalComplete = "complete"
	goalMaster   = "master"
	goalWizard   = "wizard"
)

// parallelLevel returns 0 for a base card (id == code), else the parallel
// number parsed from the id suffix ("OP16-001_p2" -> 2; non-numeric -> 1).
func parallelLevel(cardID, code string) int {
	if cardID == code || !strings.HasPrefix(cardID, code) {
		return 0
	}
	suffix := cardID[len(code):]
	digits := ""
	for i := len(suffix) - 1; i >= 0 && suffix[i] >= '0' && suffix[i] <= '9'; i-- {
		digits = string(suffix[i]) + digits
	}
	if digits == "" {
		return 1
	}
	n, _ := strconv.Atoi(digits)
	return n
}

func validGoal(g string) bool {
	return g == goalComplete || g == goalMaster || g == goalWizard
}

// inGoal reports whether a card at the given parallel level counts toward goal.
func inGoal(level int, goal string) bool {
	switch goal {
	case goalComplete:
		return level == 0
	case goalMaster:
		return level <= 1
	default: // wizard
		return true
	}
}

// Card is the normalized view of a One Piece card. Raw holds the extra fields
// scraped from the official card list so nothing is lost; it is what gets
// stored in catalogue.json.
type Card struct {
	CardID     string          `json:"cardId"`
	Name       string          `json:"name"`
	Code       string          `json:"code"`
	Rarity     string          `json:"rarity"`
	SetName    string          `json:"setName"`
	ImageSmall string          `json:"imageSmall"`
	ImageLarge string          `json:"imageLarge"`
	Raw        json.RawMessage `json:"raw,omitempty"`
}

// SearchResult is the normalized search response served from the catalogue.
type SearchResult struct {
	Page       int    `json:"page"`
	Limit      int    `json:"limit"`
	Total      int    `json:"total"`
	TotalPages int    `json:"totalPages"`
	Cards      []Card `json:"cards"`
}

// catalogueFile is the on-disk shape of catalogue.json. The sync timestamp
// lives in the file itself, so a shared/committed catalogue carries its own
// freshness info — no database needed.
type catalogueFile struct {
	SyncedAt string `json:"syncedAt"`
	Count    int    `json:"count"`
	Cards    []Card `json:"cards"`
}

// Catalogue is the in-memory One Piece card catalogue, backed by a JSON file.
// It is read-mostly: search/get hit memory; a sync replaces it wholesale and
// rewrites the file. Concurrency is guarded by a single RWMutex.
type Catalogue struct {
	path string

	mu       sync.RWMutex
	cards    []Card          // sorted by code, then name
	byID     map[string]Card // card_id -> card
	sets     map[string]*setAgg
	setOrder []string // set prefixes in display order
	syncedAt string
}

// setAgg groups a set's cards by printed code (parallels collapse onto their
// base code, with the base card kept as the representative when present).
type setAgg struct {
	prefix string
	name   string
	cards  []Card // every card in the set, incl. parallels/alt-arts
}

// SetMeta is a set's completion summary for the overview screen.
type SetMeta struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Family string `json:"family"` // stable key: main/eb/prb/deck/promo
	Group  string `json:"group"`  // display label of the family
	Total  int    `json:"total"`
}

// Set families: the collection goal can scope to any subset of these.
var familyLabels = map[string]string{
	"main":  "Séries principales",
	"eb":    "Extra Boosters",
	"prb":   "Premium Boosters",
	"deck":  "Decks",
	"promo": "Promos & autres",
}

// donSetPrefixCode is the synthetic set that gathers promo/side-product DON!!
// cards (alternate arts, Double Pack Sets, Special/Tournament packs, film
// promos, and loose "P" promos). It falls into the "promo" family below.
const donSetPrefixCode = "DON"

// donSideProductMarkers flag DON!! cards that ship in a side product rather than
// as a set's own DON: the standalone Double Pack Sets, the Special DON!! Card
// Packs, Tournament packs, and film promos. These are matched against the
// (lower-cased) card name.
var donSideProductMarkers = []string{
	"double pack set",
	"special don!! card pack",
	"tournament pack",
	"promo",
}

// setPrefix maps a card to the set it's displayed under. For most cards this is
// just the printed code prefix ("OP16-001" -> "OP16"). DON!! cards are special:
// a series' own DON — whatever its art (Alternate Art, character, …) — and its
// Gold stay in that series, but side-product DONs (Double Pack Sets, Special
// packs, Tournament packs, film promos) and loose "P" promos are consolidated
// into the dedicated "DON" set (filed under Promos & autres). The card's
// code/id is never rewritten, so ownership stays intact — this only changes
// which bucket the card is grouped into.
func setPrefix(code, rarity, name string) string {
	p := codePrefix(code)
	if rarity != "DON!!" {
		return p
	}
	if p == "P" {
		return donSetPrefixCode // loose promo DONs
	}
	n := strings.ToLower(name)
	for _, m := range donSideProductMarkers {
		if strings.Contains(n, m) {
			return donSetPrefixCode
		}
	}
	return p // the series' own DON (any art) and its Gold
}

func setFamily(prefix string) string {
	switch strings.TrimRight(prefix, "0123456789") {
	case "OP":
		return "main"
	case "EB":
		return "eb"
	case "PRB":
		return "prb"
	case "ST":
		return "deck"
	default:
		return "promo"
	}
}

// setGroup is the display label of a set's family.
func setGroup(prefix string) string { return familyLabels[setFamily(prefix)] }

// LoadCatalogue reads path into memory. If path is missing and seed is set and
// readable, it copies seed -> path first (so a fresh deploy works offline from
// a bundled catalogue with no API key). A missing file is not an error: the
// catalogue is simply empty until the first sync.
func LoadCatalogue(path, seed string) (*Catalogue, error) {
	c := &Catalogue{path: path, byID: map[string]Card{}, cards: []Card{}}

	if _, err := os.Stat(path); os.IsNotExist(err) && seed != "" {
		if data, err := os.ReadFile(seed); err == nil {
			if dir := filepath.Dir(path); dir != "" && dir != "." {
				_ = os.MkdirAll(dir, 0o755)
			}
			_ = os.WriteFile(path, data, 0o644)
		}
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read catalogue: %w", err)
	}
	var f catalogueFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse catalogue %s: %w", path, err)
	}
	c.set(f.Cards, f.SyncedAt)
	return c, nil
}

// set replaces the in-memory state (caller must hold no lock).
func (c *Catalogue) set(cards []Card, syncedAt string) {
	byID := make(map[string]Card, len(cards))
	for _, card := range cards {
		if card.CardID == "" {
			continue
		}
		byID[card.CardID] = card
	}
	deduped := make([]Card, 0, len(byID))
	for _, card := range byID {
		deduped = append(deduped, card)
	}
	sort.Slice(deduped, func(i, j int) bool {
		if deduped[i].Code != deduped[j].Code {
			return deduped[i].Code < deduped[j].Code
		}
		return deduped[i].Name < deduped[j].Name
	})

	// Group into sets by code prefix; keep one representative card per printed
	// code (prefer the base card whose id equals the code over its parallels).
	sets := map[string]*setAgg{}
	for _, card := range deduped {
		prefix := setPrefix(card.Code, card.Rarity, card.Name)
		agg := sets[prefix]
		if agg == nil {
			agg = &setAgg{prefix: prefix}
			if prefix == donSetPrefixCode {
				agg.name = "DON!!" // synthetic set; its cards carry no SetName
			}
			sets[prefix] = agg
		}
		if agg.name == "" && card.SetName != "" {
			agg.name = card.SetName
		}
		agg.cards = append(agg.cards, card) // every version, incl. parallels
	}
	order := make([]string, 0, len(sets))
	for p := range sets {
		order = append(order, p)
	}
	sort.Slice(order, func(i, j int) bool { return setLess(order[i], order[j]) })

	c.mu.Lock()
	c.cards = deduped
	c.byID = byID
	c.sets = sets
	c.setOrder = order
	c.syncedAt = syncedAt
	c.mu.Unlock()
}

// setLess orders set prefixes for display: families in a fixed priority
// (OP, EB, PRB, ST, then anything else like P/promos), newest number first.
func setLess(a, b string) bool {
	ra, na := setRank(a)
	rb, nb := setRank(b)
	if ra != rb {
		return ra < rb
	}
	if na != nb {
		return na > nb // higher number (newer) first
	}
	return a < b
}

func setRank(prefix string) (int, int) {
	letters := strings.TrimRight(prefix, "0123456789")
	num := 0
	if s := strings.TrimLeft(prefix, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"); s != "" {
		fmt.Sscanf(s, "%d", &num)
	}
	rank := map[string]int{"OP": 0, "EB": 1, "PRB": 2, "ST": 3}
	r, ok := rank[letters]
	if !ok {
		r = 9
	}
	return r, num
}

// SetList returns each set's completion metadata in display order.
func (c *Catalogue) SetList() []SetMeta {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]SetMeta, 0, len(c.setOrder))
	for _, p := range c.setOrder {
		agg := c.sets[p]
		out = append(out, SetMeta{Code: p, Name: agg.name, Family: setFamily(p), Group: setGroup(p), Total: len(agg.cards)})
	}
	return out
}

// DonCards returns the cards currently in the catalogue that are DON!! cards
// (sourced from the community API). Used to preserve them when an official
// resync can't reach the DON source.
func (c *Catalogue) DonCards() []Card {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []Card
	for _, card := range c.cards {
		if card.Rarity == "DON!!" {
			out = append(out, card)
		}
	}
	return out
}

// SetGoalTotals returns, per set prefix, the number of cards that count toward
// the given collection goal.
func (c *Catalogue) SetGoalTotals(goal string) map[string]int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]int, len(c.sets))
	for p, agg := range c.sets {
		n := 0
		for _, card := range agg.cards {
			if inGoal(parallelLevel(card.CardID, card.Code), goal) {
				n++
			}
		}
		out[p] = n
	}
	return out
}

// SetCards returns every card in a set (incl. parallels), sorted by code then
// card id (base before its parallels), plus the set name. ok is false if the
// set prefix is unknown.
func (c *Catalogue) SetCards(prefix string) ([]Card, string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	agg, ok := c.sets[prefix]
	if !ok {
		return nil, "", false
	}
	cards := make([]Card, len(agg.cards))
	copy(cards, agg.cards)
	// Inline order (base card immediately followed by its parallels). The
	// frontend applies optional display ordering/filters (parallels-at-end,
	// owned/missing, goal-only).
	sort.Slice(cards, func(i, j int) bool {
		if cards[i].Code != cards[j].Code {
			return cards[i].Code < cards[j].Code
		}
		return cards[i].CardID < cards[j].CardID
	})
	return cards, agg.name, true
}

// Replace swaps in a freshly-synced set of cards and persists it to disk.
func (c *Catalogue) Replace(cards []Card, syncedAt string) error {
	c.set(cards, syncedAt)
	return c.save()
}

func (c *Catalogue) save() error {
	c.mu.RLock()
	f := catalogueFile{SyncedAt: c.syncedAt, Count: len(c.cards), Cards: c.cards}
	c.mu.RUnlock()

	data, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(c.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	// Write to a temp file then rename, so a crash mid-write can't corrupt it.
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

func (c *Catalogue) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cards)
}

func (c *Catalogue) SyncedAt() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.syncedAt
}

// Get returns a cached card by id, or (zero, false) if absent.
func (c *Catalogue) Get(id string) (Card, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	card, ok := c.byID[id]
	return card, ok
}

// Search filters the catalogue in memory by name or code (case-insensitive),
// returning one page plus the total match count.
func (c *Catalogue) Search(q string, page, limit int) ([]Card, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	q = strings.ToLower(strings.TrimSpace(q))

	c.mu.RLock()
	defer c.mu.RUnlock()

	var matches []Card
	if q == "" {
		matches = c.cards
	} else {
		matches = make([]Card, 0, 64)
		for _, card := range c.cards {
			if strings.Contains(strings.ToLower(card.Name), q) ||
				strings.Contains(strings.ToLower(card.Code), q) {
				matches = append(matches, card)
			}
		}
	}

	total := len(matches)
	start := (page - 1) * limit
	if start >= total {
		return []Card{}, total
	}
	end := start + limit
	if end > total {
		end = total
	}
	out := make([]Card, end-start)
	copy(out, matches[start:end])
	return out, total
}
