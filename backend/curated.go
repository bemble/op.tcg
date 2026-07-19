package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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
	productID int64  // TCGplayer productId -> image URL (0 for manual entries)
	imageURL  string // image URL served for this card (may be a local /api path)
	sourceURL string // where it was imported from (TCGplayer/Cardmarket/…)
	imageBlob []byte // downloaded + downscaled image bytes (manual imports)
}

// Built-in curated cards (the "P-073/074/075 Tin Pack Set Vol. 2" alt arts).
var curatedCards = []curatedCard{
	{cardID: "P-073_p2", code: "P-073", name: "Sabo", rarity: "PR", productID: 669296},
	{cardID: "P-074_p2", code: "P-074", name: "Portgas.D.Ace", rarity: "PR", productID: 669293},
	{cardID: "P-075_p2", code: "P-075", name: "Monkey.D.Luffy", rarity: "PR", productID: 669279},
}

// curatedToCard materialises one curated entry into a catalogue Card. The image
// is an explicit URL (manual entries) or TCGplayer's CDN by product id; empty
// for manual entries with no image (the UI then shows a placeholder).
func curatedToCard(c curatedCard) Card {
	img := c.imageURL
	if img == "" && c.productID > 0 {
		img = donImageURL(c.productID)
	}
	raw, _ := json.Marshal(map[string]any{
		"id": c.cardID, "code": c.code, "name": c.name, "rarity": c.rarity,
		"productId": c.productID, "curated": true,
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
	img := c.imageURL
	if img == "" && c.productID > 0 {
		img = donImageURL(c.productID)
	}
	return map[string]any{
		"cardId": c.cardID, "code": c.code, "name": c.name,
		"rarity": c.rarity, "productId": c.productID, "image": img,
		"sourceUrl": c.sourceURL,
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

// curatedReq is a request to add a curated card, in either mode (see buildCurated).
type curatedReq struct {
	URL       string `json:"url"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Rarity    string `json:"rarity"`
	ImageURL  string `json:"imageUrl"`
	SourceURL string `json:"sourceUrl"`
}

// handleAddCurated adds a card the automated sources miss. Two modes:
//   - {url}: a TCGplayer product URL/id — we look up number/name/rarity/image;
//   - {code, name, rarity?, imageUrl?}: manual entry, for cards only on other
//     sites (e.g. Cardmarket, which we can't scrape). The code decides the set;
//     a parallel slot is picked if the code already exists.
func (s *server) handleAddCurated(w http.ResponseWriter, r *http.Request) {
	var req curatedReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	c, err := s.buildCurated(r.Context(), req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.persistCurated(c); err != nil {
		writeErr(w, http.StatusConflict, "ajout impossible (déjà présente ?): "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, curatedJSON(c))
}

// buildCurated resolves a curated card from a request (TCGplayer lookup or
// manual entry), downloading its image where applicable. It does not persist.
func (s *server) buildCurated(ctx context.Context, req curatedReq) (curatedCard, error) {
	if strings.TrimSpace(req.Code) != "" || strings.TrimSpace(req.Name) != "" {
		// Manual entry.
		code := strings.ToUpper(strings.TrimSpace(req.Code))
		name := strings.TrimSpace(req.Name)
		if code == "" || name == "" {
			return curatedCard{}, fmt.Errorf("code et nom requis")
		}
		rarity := strings.TrimSpace(req.Rarity)
		if rarity == "" {
			rarity = "PR"
		}
		cardID := s.resolveCuratedID(code)
		c := curatedCard{
			cardID:    cardID,
			code:      code,
			name:      name,
			rarity:    rarity,
			sourceURL: strings.TrimSpace(req.SourceURL),
		}
		// Download + downscale the image now and store it locally (the /api/img
		// proxy only whitelists a few hosts, and sources may block hotlinking
		// later). On failure, import anyway.
		if raw := strings.TrimSpace(req.ImageURL); raw != "" {
			if blob, ferr := fetchAndThumb(ctx, raw); ferr == nil && len(blob) > 0 {
				c.imageBlob = blob
				c.imageURL = "/api/curated/" + cardID + "/image"
			} else if ferr != nil {
				log.Printf("curated image fetch failed (%s): %v", raw, ferr)
			}
		}
		return c, nil
	}

	// TCGplayer mode.
	pid, ok := parseTCGProductID(req.URL)
	if !ok {
		return curatedCard{}, fmt.Errorf("URL ou identifiant produit TCGplayer invalide")
	}
	prod, err := fetchTCGProduct(ctx, pid)
	if err != nil {
		return curatedCard{}, fmt.Errorf("TCGplayer: %w", err)
	}
	// Most cards have an official number (e.g. "P-074") we group under. Some
	// promos (sealed-battle, event leaders…) have none on TCGplayer — synthesize
	// a code in the set the product belongs to (OP-PR -> "P"), keyed by product
	// id so it's unique, so the card still lands in the right set.
	code, cardID, name := prod.Number, "", prod.Name
	if code != "" {
		cardID = s.resolveCuratedID(code)
	} else {
		code = fmt.Sprintf("%s-%d", donSetPrefix(prod.SetCode), prod.ProductID)
		cardID = code
		name = prod.FullName // no number to distinguish it — keep the full name
	}
	// Keep the pasted URL as the source; fall back to a canonical product URL if
	// only a bare id was given.
	src := strings.TrimSpace(req.URL)
	if !strings.Contains(src, "://") {
		src = fmt.Sprintf("https://www.tcgplayer.com/product/%d", prod.ProductID)
	}
	return curatedCard{
		cardID:    cardID,
		code:      code,
		name:      name,
		rarity:    prod.Rarity,
		productID: prod.ProductID,
		sourceURL: src,
	}, nil
}

// persistCurated saves a curated card and merges it into the live catalogue.
func (s *server) persistCurated(c curatedCard) error {
	if err := s.st.AddCuratedCard(c); err != nil {
		return err
	}
	return s.refreshCuratedExtra()
}

// fetchAndThumb downloads an image (browser-ish headers so hotlink-protected
// hosts serve it), downscales it and re-encodes as JPEG for local storage.
func fetchAndThumb(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("URL image invalide")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 AppleWebKit/537.36 Chrome/124 Safari/537.36")
	req.Header.Set("Referer", u.Scheme+"://"+u.Host+"/")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image %s", resp.Status)
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	out, err := processImage(data, 500) // downscale + re-encode JPEG (see image.go)
	if err != nil {
		return nil, fmt.Errorf("image illisible: %w", err)
	}
	return out, nil
}

func (s *server) handleCuratedImage(w http.ResponseWriter, r *http.Request) {
	blob, err := s.st.CuratedImage(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(blob) == 0 {
		writeErr(w, http.StatusNotFound, "pas d'image")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=604800")
	_, _ = w.Write(blob)
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
	Number    string // official card code, e.g. "P-074" (may be empty)
	SetCode   string // TCGplayer set code, e.g. "OP-PR", "OP16"
	Name      string // cleaned card name (distribution suffix stripped)
	FullName  string // raw product name (keeps the distribution suffix)
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
		SetCode          string `json:"setCode"`
		CustomAttributes struct {
			Number       string `json:"number"`
			RarityDbName string `json:"rarityDbName"`
		} `json:"customAttributes"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("decode tcgplayer product: %w", err)
	}
	name := strings.TrimSpace(reTrailingParen.ReplaceAllString(d.ProductName, ""))
	if name == "" {
		return nil, fmt.Errorf("tcgplayer product %d has no name", productID)
	}
	rarity := d.CustomAttributes.RarityDbName
	if rarity == "" {
		rarity = "PR"
	}
	return &tcgProduct{
		ProductID: productID,
		Number:    strings.ToUpper(strings.TrimSpace(d.CustomAttributes.Number)),
		SetCode:   d.SetCode,
		Name:      name,
		FullName:  strings.TrimSpace(d.ProductName),
		Rarity:    rarity,
	}, nil
}
