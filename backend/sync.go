package main

import (
	"context"
	"log"
	"net/http"
	"time"
)

// syncResult summarises a catalogue sync run.
type syncResult struct {
	Cards int    `json:"cards"`
	Pages int    `json:"pages"`
	At    string `json:"at"`
}

// runSync scrapes the full One Piece catalogue from the official card list
// (en.onepiece-cardgame.com), one request per set (~53). No API key, no quota.
// It then merges DON!! cards from TCGplayer's public catalogue, which the
// official list omits. The result replaces catalogue.json.
func (s *server) runSync(ctx context.Context) (syncResult, error) {
	var res syncResult
	cards, pages, err := newOfficialClient().fetchCatalogue(ctx)
	res.Pages = pages
	if err != nil {
		return res, err
	}

	// DON!! cards are supplementary: if their source is unreachable, keep the
	// ones already in the catalogue rather than dropping them.
	if don, derr := fetchDonCards(ctx); derr == nil {
		cards = append(cards, don...)
		log.Printf("DON!! cards: %d from tcgplayer.com", len(don))
	} else if existing := s.cat.DonCards(); len(existing) > 0 {
		cards = append(cards, existing...)
		log.Printf("DON!! fetch failed (%v); kept %d existing", derr, len(existing))
	} else {
		log.Printf("DON!! fetch failed (%v); none available", derr)
	}

	res.At = time.Now().UTC().Format(time.RFC3339)
	if err := s.cat.Replace(cards, res.At); err != nil {
		return res, err
	}
	res.Cards = s.cat.Count()
	return res, nil
}

// beginSync starts a background catalogue sync unless one is already running.
// Returns false if a sync was already in progress. A full scrape takes
// ~20-40s, so callers don't wait on it — they poll GET /api/sync.
func (s *server) beginSync() bool {
	s.syncMu.Lock()
	if s.syncing {
		s.syncMu.Unlock()
		return false
	}
	s.syncing = true
	s.lastErr = ""
	s.syncMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		res, err := s.runSync(ctx)

		s.syncMu.Lock()
		s.syncing = false
		if err != nil {
			s.lastErr = err.Error()
			log.Printf("sync failed after %d set(s): %v", res.Pages, err)
		} else {
			s.lastSync = res
			log.Printf("catalogue sync: %d cards over %d set(s)", res.Cards, res.Pages)
		}
		s.syncMu.Unlock()
	}()
	return true
}

// catalogueStale reports whether a startup auto-sync is warranted: the
// catalogue is empty, has no/invalid timestamp, or is older than SyncMaxAge
// (SyncMaxAge <= 0 means always sync).
func (s *server) catalogueStale() bool {
	if s.cat.Count() == 0 {
		return true
	}
	if s.cfg.SyncMaxAge <= 0 {
		return true
	}
	at, err := time.Parse(time.RFC3339, s.cat.SyncedAt())
	if err != nil {
		return true
	}
	return time.Since(at) > s.cfg.SyncMaxAge
}

// handleSync starts a sync in the background and returns immediately (202).
// The client polls GET /api/sync for progress.
func (s *server) handleSync(w http.ResponseWriter, r *http.Request) {
	if !s.beginSync() {
		writeErr(w, http.StatusConflict, "une synchronisation est déjà en cours")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"started": true})
}

func (s *server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	s.syncMu.Lock()
	syncing := s.syncing
	lastErr := s.lastErr
	s.syncMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"syncing":   syncing,
		"cardCount": s.cat.Count(),
		"lastSync":  s.cat.SyncedAt(),
		"error":     lastErr,
	})
}
