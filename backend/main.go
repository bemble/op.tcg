package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type server struct {
	cfg Config
	st  *Store
	cat *Catalogue

	syncMu   sync.Mutex
	syncing  bool
	lastSync syncResult
	lastErr  string
}

func main() {
	cfg := loadConfig()

	st, err := openStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer st.Close()

	cat, err := LoadCatalogue(cfg.CataloguePath, cfg.CatalogueSeed)
	if err != nil {
		log.Fatalf("load catalogue: %v", err)
	}
	log.Printf("catalogue: %d cards (file=%s)", cat.Count(), cfg.CataloguePath)

	s := &server{cfg: cfg, st: st, cat: cat}

	// Merge curated extras (built-in + user-added) into the catalogue.
	if err := s.refreshCuratedExtra(); err != nil {
		log.Printf("curated cards: %v", err)
	}

	// Refresh the catalogue from the official site on startup (background, so
	// the server serves the existing catalogue immediately). Skipped when the
	// catalogue is already fresh — see catalogueStale / SYNC_MAX_AGE_HOURS.
	if cfg.SyncOnStart && s.catalogueStale() {
		log.Printf("catalogue empty or stale — starting background sync")
		s.beginSync()
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      s.routes(),
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		log.Printf("one-piece-collect listening on :%s (db=%s)", cfg.Port, cfg.DBPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Println("shutdown complete")
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("GET /api/cards/{id}", s.handleGetCard)
	mux.HandleFunc("GET /api/img", s.handleImageProxy)

	mux.HandleFunc("GET /api/sync", s.handleSyncStatus)
	mux.HandleFunc("POST /api/sync", s.handleSync)

	mux.HandleFunc("GET /api/stats", s.handleFullStats)
	mux.HandleFunc("GET /api/sets", s.handleSets)
	mux.HandleFunc("GET /api/sets/{code}", s.handleSetDetail)
	mux.HandleFunc("GET /api/missing", s.handleMissing)

	mux.HandleFunc("GET /api/collection", s.handleListItems)
	mux.HandleFunc("POST /api/collection", s.handleAddItem)
	mux.HandleFunc("POST /api/collection/batch", s.handleBatchAddItems)
	mux.HandleFunc("PATCH /api/collection/bulk", s.handleBulkEditItems)
	mux.HandleFunc("GET /api/collection/stats", s.handleStats)
	mux.HandleFunc("PATCH /api/collection/{id}", s.handleUpdateItem)
	mux.HandleFunc("DELETE /api/collection/{id}", s.handleDeleteItem)

	mux.HandleFunc("GET /api/owners", s.handleListOwners)
	mux.HandleFunc("POST /api/owners", s.handleAddOwner)
	mux.HandleFunc("DELETE /api/owners/{id}", s.handleDeleteOwner)

	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handleUpdateSettings)

	mux.HandleFunc("GET /api/curated", s.handleListCurated)
	mux.HandleFunc("POST /api/curated", s.handleAddCurated)
	mux.HandleFunc("GET /api/curated/{id}/image", s.handleCuratedImage)
	mux.HandleFunc("DELETE /api/curated/{id}", s.handleDeleteCurated)

	// Any unmatched /api/* path returns a clean JSON 404 instead of falling
	// through to the SPA handler (which would return HTML, or — with an empty
	// web dir in dev — a misleading "frontend not built" error).
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "unknown API endpoint: "+r.Method+" "+r.URL.Path)
	})

	mux.Handle("/", s.spaHandler())

	return logging(mux)
}

// ---- API handlers ----

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"cardCount": s.cat.Count(),
	})
}

// handleSearch reads only the in-memory catalogue — no network call, so it
// works offline. Populate the catalogue first with POST /api/sync.
func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	page := atoiDefault(r.URL.Query().Get("page"), 1)
	limit := atoiDefault(r.URL.Query().Get("limit"), 50)
	if limit > 200 {
		limit = 200
	}
	cards, total := s.cat.Search(name, page, limit)
	totalPages := 0
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
	}
	// Annotate with the user's ownership/status so search results behave like the
	// set-detail grid (owned/ordered/wishlist badges, click to manage).
	annotated := s.annotateCards(cards, s.ownedByCard(), s.collectionGoal())
	writeJSON(w, http.StatusOK, map[string]any{
		"page": page, "limit": limit, "total": total, "totalPages": totalPages,
		"cards": annotated,
	})
}

func (s *server) handleGetCard(w http.ResponseWriter, r *http.Request) {
	c, ok := s.cat.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "card not in catalogue")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// enrich fills each item's Card from the catalogue. Cards absent from the
// catalogue (e.g. owned before a re-sync changed ids) get a minimal stub so
// the collection still renders.
func (s *server) enrich(items []Item) {
	for i := range items {
		if c, ok := s.cat.Get(items[i].CardID); ok {
			cc := c
			items[i].Card = &cc
		} else {
			items[i].Card = &Card{CardID: items[i].CardID, Name: items[i].CardID, Code: items[i].CardID}
		}
	}
}

func (s *server) handleListItems(w http.ResponseWriter, r *http.Request) {
	owner := int64(atoiDefault(r.URL.Query().Get("owner"), 0))
	items, err := s.st.ListItems(owner)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.enrich(items)

	// Optional name/code filter, applied against the enriched card data.
	if q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q"))); q != "" {
		filtered := items[:0]
		for _, it := range items {
			name := strings.ToLower(it.Card.Name)
			code := strings.ToLower(it.Card.Code)
			if strings.Contains(name, q) || strings.Contains(code, q) {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}

	if items == nil {
		items = []Item{}
	}
	// Sort by card name for a stable, readable collection view.
	sortItemsByName(items)
	writeJSON(w, http.StatusOK, items)
}

type addItemRequest struct {
	CardID   string `json:"cardId"`
	OwnerID  *int64 `json:"ownerId"`
	Quantity int    `json:"quantity"`
	Language string `json:"language"`
	Notes    string `json:"notes"`
	Status   string `json:"status"` // owned (default) | ordered | wishlist
}

func (s *server) handleAddItem(w http.ResponseWriter, r *http.Request) {
	var req addItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.CardID == "" {
		writeErr(w, http.StatusBadRequest, "missing 'cardId'")
		return
	}
	if _, ok := s.cat.Get(req.CardID); !ok {
		writeErr(w, http.StatusBadRequest, "carte absente du catalogue — synchronise le catalogue d'abord")
		return
	}

	status := normStatus(req.Status)
	if !validStatus(status) {
		writeErr(w, http.StatusBadRequest, "statut invalide")
		return
	}
	it := Item{
		OwnerID:  req.OwnerID,
		Quantity: req.Quantity,
		Language: orDefault(req.Language, "EN"),
		Notes:    req.Notes,
		Status:   status,
	}
	out, err := s.st.AddItem(req.CardID, it)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	one := []Item{*out}
	s.enrich(one)
	writeJSON(w, http.StatusCreated, one[0])
}

type batchAddRequest struct {
	Items []addItemRequest `json:"items"`
}

// handleBatchAddItems adds many possessions in a single transaction (one DB
// round-trip instead of N). Validation is up-front: if any card is unknown the
// whole batch is rejected before touching the DB.
func (s *server) handleBatchAddItems(w http.ResponseWriter, r *http.Request) {
	var req batchAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Items) == 0 {
		writeErr(w, http.StatusBadRequest, "missing 'items'")
		return
	}

	entries := make([]BatchEntry, 0, len(req.Items))
	for _, in := range req.Items {
		if in.CardID == "" {
			writeErr(w, http.StatusBadRequest, "missing 'cardId' in one of the items")
			return
		}
		if _, ok := s.cat.Get(in.CardID); !ok {
			writeErr(w, http.StatusBadRequest, "carte absente du catalogue : "+in.CardID)
			return
		}
		status := normStatus(in.Status)
		if !validStatus(status) {
			writeErr(w, http.StatusBadRequest, "statut invalide")
			return
		}
		entries = append(entries, BatchEntry{
			CardID: in.CardID,
			Item: Item{
				OwnerID:  in.OwnerID,
				Quantity: in.Quantity,
				Language: orDefault(in.Language, "EN"),
				Notes:    in.Notes,
				Status:   status,
			},
		})
	}

	items, err := s.st.AddItems(entries)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.enrich(items)
	writeJSON(w, http.StatusCreated, items)
}

// handleBulkEditItems changes the language of the given possessions (by item id)
// in one shot (list-view "édition groupée"), with per-owner granularity.
func (s *server) handleBulkEditItems(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ItemIDs  []int64 `json:"itemIds"`
		Language string  `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	if req.Language == "" {
		writeErr(w, http.StatusBadRequest, "langue manquante")
		return
	}
	if len(req.ItemIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "aucun exemplaire sélectionné")
		return
	}
	n, err := s.st.BulkSetLanguageItems(req.ItemIDs, req.Language)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": n})
}

func (s *server) handleUpdateItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	cur, err := s.st.GetItem(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	// Decode into a generic map first so we can tell "field omitted" from
	// "field set to zero/empty".
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	merged := *cur
	if v, ok := raw["quantity"]; ok {
		_ = json.Unmarshal(v, &merged.Quantity)
	}
	if v, ok := raw["language"]; ok {
		var l string
		_ = json.Unmarshal(v, &l)
		merged.Language = orDefault(l, cur.Language)
	}
	if v, ok := raw["notes"]; ok {
		_ = json.Unmarshal(v, &merged.Notes)
	}
	if v, ok := raw["status"]; ok {
		var st string
		_ = json.Unmarshal(v, &st)
		st = normStatus(st)
		if !validStatus(st) {
			writeErr(w, http.StatusBadRequest, "statut invalide")
			return
		}
		merged.Status = st
	}
	if v, ok := raw["ownerId"]; ok {
		// null or 0 clears the owner; otherwise set it.
		var oid *int64
		_ = json.Unmarshal(v, &oid)
		if oid != nil && *oid <= 0 {
			oid = nil
		}
		merged.OwnerID = oid
	}

	out, err := s.st.UpdateItem(id, merged)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if out == nil {
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
		return
	}
	one := []Item{*out}
	s.enrich(one)
	writeJSON(w, http.StatusOK, one[0])
}

func (s *server) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.st.DeleteItem(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.st.Stats()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// ---- owners ----

func (s *server) handleListOwners(w http.ResponseWriter, r *http.Request) {
	owners, err := s.st.ListOwners()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, owners)
}

func (s *server) handleAddOwner(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	o, err := s.st.AddOwner(name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (s *server) handleDeleteOwner(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.st.DeleteOwner(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- static SPA ----

func (s *server) spaHandler() http.Handler {
	root := s.cfg.WebDir
	fs := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := filepath.Clean(r.URL.Path)
		full := filepath.Join(root, clean)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		// SPA fallback
		index := filepath.Join(root, "index.html")
		if _, err := os.Stat(index); err != nil {
			writeErr(w, http.StatusNotFound, "frontend not built (web dir empty)")
			return
		}
		http.ServeFile(w, r, index)
	})
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func sortItemsByName(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		ni, nj := "", ""
		if items[i].Card != nil {
			ni = items[i].Card.Name
		}
		if items[j].Card != nil {
			nj = items[j].Card.Name
		}
		if !strings.EqualFold(ni, nj) {
			return strings.ToLower(ni) < strings.ToLower(nj)
		}
		return items[i].ID < items[j].ID
	})
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}
