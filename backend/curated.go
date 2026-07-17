package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Curated catalogue entries: cards our automated sources don't expose —
// TCGplayer-only promo alternate arts / printings that the official One Piece
// card list (our source for every non-DON card) omits. Two origins are merged
// into the catalogue as "extra" cards (live, no resync needed):
//
//   - built-ins below, baked into the binary;
//   - user additions stored in the DB (added via Préférences -> "carte non
//     trouvée"), which fetch their details from a TCGplayer product URL.
type curatedCard struct {
	cardID    string
	code      string
	name      string
	rarity    string
	productID int64 // TCGplayer productId -> image URL
}

// Built-in curated cards (the "P-073/074/075 Tin Pack Set Vol. 2" alt arts).
var curatedCards = []curatedCard{
	{cardID: "P-073_p2", code: "P-073", name: "Sabo", rarity: "PR", productID: 669296},
	{cardID: "P-074_p2", code: "P-074", name: "Portgas.D.Ace", rarity: "PR", productID: 669293},
	{cardID: "P-075_p2", code: "P-075", name: "Monkey.D.Luffy", rarity: "PR", productID: 669279},
}

// curatedToCard materialises one curated entry into a catalogue Card. The image
// is served from TCGplayer's CDN, same as DON!! cards.
func curatedToCard(c curatedCard) Card {
	img := donImageURL(c.productID)
	raw, _ := json.Marshal(map[string]any{
		"id": c.cardID, "code": c.code, "name": c.name, "rarity": c.rarity,
		"source": "tcgplayer.com", "productId": c.productID, "curated": true,
	})
	return Card{
		CardID:     c.cardID,
		Code:       c.code,
		Name:       c.name,
		Rarity:     c.rarity,
		ImageSmall: img,
		ImageLarge: img,
		Raw:        raw,
	}
}

// builtinCuratedCards returns the baked-in curated cards as catalogue cards.
func builtinCuratedCards() []Card {
	out := make([]Card, 0, len(curatedCards))
	for _, c := range curatedCards {
		out = append(out, curatedToCard(c))
	}
	return out
}

// refreshCuratedExtra recomputes the catalogue's extra cards (built-ins + DB
// additions) and merges them into the live catalogue.
func (s *server) refreshCuratedExtra() error {
	db, err := s.st.ListCuratedCards()
	if err != nil {
		return err
	}
	extra := builtinCuratedCards()
	for _, c := range db {
		extra = append(extra, curatedToCard(c))
	}
	s.cat.SetExtra(extra)
	return nil
}

// resolveCuratedID picks a free catalogue id for a curated card: the bare code
// if nothing uses it yet, else the first free "_pN" parallel slot.
func (s *server) resolveCuratedID(code string) string {
	if _, ok := s.cat.Get(code); !ok {
		return code
	}
	for n := 1; n < 100; n++ {
		cand := fmt.Sprintf("%s_p%d", code, n)
		if _, ok := s.cat.Get(cand); !ok {
			return cand
		}
	}
	return code + "_px"
}

func curatedJSON(c curatedCard) map[string]any {
	return map[string]any{
		"cardId": c.cardID, "code": c.code, "name": c.name,
		"rarity": c.rarity, "productId": c.productID, "image": donImageURL(c.productID),
	}
}

func (s *server) handleListCurated(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListCuratedCards()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		out = append(out, curatedJSON(c))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAddCurated takes a TCGplayer product URL (or id), looks up the card's
// number/name/rarity, stores it and merges it into the catalogue immediately.
func (s *server) handleAddCurated(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	pid, ok := parseTCGProductID(req.URL)
	if !ok {
		writeErr(w, http.StatusBadRequest, "URL ou identifiant produit TCGplayer invalide")
		return
	}
	prod, err := fetchTCGProduct(r.Context(), pid)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "TCGplayer: "+err.Error())
		return
	}
	c := curatedCard{
		cardID:    s.resolveCuratedID(prod.Number),
		code:      prod.Number,
		name:      prod.Name,
		rarity:    prod.Rarity,
		productID: prod.ProductID,
	}
	if err := s.st.AddCuratedCard(c); err != nil {
		writeErr(w, http.StatusConflict, "ajout impossible (déjà présente ?): "+err.Error())
		return
	}
	if err := s.refreshCuratedExtra(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, curatedJSON(c))
}

func (s *server) handleDeleteCurated(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteCuratedCard(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.refreshCuratedExtra(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// tcgProduct is the subset of a TCGplayer product we use to seed a curated card.
type tcgProduct struct {
	ProductID int64
	Number    string // official card code, e.g. "P-074"
	Name      string // cleaned card name (distribution suffix stripped)
	Rarity    string // e.g. "PR"
}

var (
	reTCGProductID  = regexp.MustCompile(`/product/(\d+)`)
	reTrailingParen = regexp.MustCompile(`\s*\([^)]*\)\s*$`)
)

// parseTCGProductID extracts the numeric product id from a TCGplayer product URL
// (or returns the input if it's already just a number).
func parseTCGProductID(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if m := reTCGProductID.FindStringSubmatch(s); m != nil {
		id, err := strconv.ParseInt(m[1], 10, 64)
		return id, err == nil
	}
	id, err := strconv.ParseInt(s, 10, 64)
	return id, err == nil && id > 0
}

// fetchTCGProduct looks up a TCGplayer product's details (the same public
// endpoint the website uses; no auth) to seed a curated card.
func fetchTCGProduct(ctx context.Context, productID int64) (*tcgProduct, error) {
	url := fmt.Sprintf("https://mp-search-api.tcgplayer.com/v1/product/%d/details", productID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Origin", "https://www.tcgplayer.com")
	req.Header.Set("Referer", "https://www.tcgplayer.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 AppleWebKit/537.36 Chrome/124 Safari/537.36")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tcgplayer product %d: %s", productID, resp.Status)
	}
	var d struct {
		ProductName      string `json:"productName"`
		CustomAttributes struct {
			Number       string `json:"number"`
			RarityDbName string `json:"rarityDbName"`
		} `json:"customAttributes"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("decode tcgplayer product: %w", err)
	}
	if d.CustomAttributes.Number == "" {
		return nil, fmt.Errorf("tcgplayer product %d has no card number", productID)
	}
	name := strings.TrimSpace(reTrailingParen.ReplaceAllString(d.ProductName, ""))
	rarity := d.CustomAttributes.RarityDbName
	if rarity == "" {
		rarity = "PR"
	}
	return &tcgProduct{
		ProductID: productID,
		Number:    strings.ToUpper(strings.TrimSpace(d.CustomAttributes.Number)),
		Name:      name,
		Rarity:    rarity,
	}, nil
}
