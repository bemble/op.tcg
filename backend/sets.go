package main

import "net/http"

// ownedByCard groups possessions by card id (each version is its own slot).
func (s *server) ownedByCard() map[string][]Item {
	items, _ := s.st.ListItems(0)
	byCard := map[string][]Item{}
	for _, it := range items {
		byCard[it.CardID] = append(byCard[it.CardID], it)
	}
	return byCard
}

// hasStatus reports whether any of the items has the given status.
func hasStatus(items []Item, status string) bool {
	for _, it := range items {
		if normStatus(it.Status) == status {
			return true
		}
	}
	return false
}

// isAcquired reports whether a card counts as acquired for completion: it's
// physically owned or on order. Wishlist entries don't count.
func isAcquired(items []Item) bool {
	return hasStatus(items, statusOwned) || hasStatus(items, statusOrdered)
}

// ownedCountsForGoal counts, per set prefix, the distinct acquired (owned or
// ordered) cards that count toward the goal.
func (s *server) ownedCountsForGoal(byCard map[string][]Item, goal string) map[string]int {
	counts := map[string]int{}
	for cardID, items := range byCard {
		c, ok := s.cat.Get(cardID)
		if !ok {
			continue
		}
		if !inGoal(parallelLevel(cardID, c.Code), goal) {
			continue
		}
		if !isAcquired(items) {
			continue
		}
		counts[setPrefix(c.Code, c.Rarity, c.Name)]++
	}
	return counts
}

type setMetaOut struct {
	SetMeta
	Owned int `json:"owned"`
}

// handleSets returns every set with its completion (owned cards / total cards)
// scoped to the collection goal (?goal=).
func (s *server) handleSets(w http.ResponseWriter, r *http.Request) {
	goal := s.collectionGoal()
	totals := s.cat.SetGoalTotals(goal)
	owned := s.ownedCountsForGoal(s.ownedByCard(), goal)
	metas := s.cat.SetList()
	// All families are returned; each carries its `family` key so the UI can
	// show progress only for families included in the collection goal.
	out := make([]setMetaOut, 0, len(metas))
	for _, m := range metas {
		m.Total = totals[m.Code]
		out = append(out, setMetaOut{SetMeta: m, Owned: owned[m.Code]})
	}
	writeJSON(w, http.StatusOK, out)
}

// setCard is one card in a set, annotated with ownership/status and whether it
// counts toward the active collection goal. Owned/Ordered/Wishlist are the
// aggregate (all owners) states; the frontend rescopes them per owner from
// Items when an owner filter is active.
type setCard struct {
	Card
	Owned    bool   `json:"owned"`    // has a physical copy
	Ordered  bool   `json:"ordered"`  // has an on-order copy
	Wishlist bool   `json:"wishlist"` // wanted by someone
	Quantity int    `json:"quantity"` // physical (owned) quantity
	InGoal   bool   `json:"inGoal"`
	Items    []Item `json:"items"`
}

// handleSetDetail returns every card of a set (incl. parallels) with its
// ownership state for the grid. The header totals (total/owned) are scoped to
// the collection goal (?goal=); the grid itself always shows all cards so an
// owned card is never hidden. Each card carries `inGoal` so the UI can tell.
func (s *server) handleSetDetail(w http.ResponseWriter, r *http.Request) {
	prefix := r.PathValue("code")
	cards, name, ok := s.cat.SetCards(prefix)
	if !ok {
		writeErr(w, http.StatusNotFound, "set inconnu")
		return
	}
	goal := s.collectionGoal()
	byCard := s.ownedByCard()

	out := s.annotateCards(cards, byCard, goal)
	total, ownedCount := 0, 0
	for _, c := range out {
		if c.InGoal {
			total++
			if c.Owned || c.Ordered { // "acquired" — ordered counts, wishlist doesn't
				ownedCount++
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code":  prefix,
		"name":  name,
		"total": total,
		"owned": ownedCount,
		"cards": out,
	})
}

// handleMissing returns the not-acquired (neither owned nor ordered) cards of
// every set in the selected families, grouped by set in display order. With
// ?goalOnly=1 only cards counting toward the collection goal are included.
func (s *server) handleMissing(w http.ResponseWriter, r *http.Request) {
	goal := s.collectionGoal()
	goalOnly := r.URL.Query().Get("goalOnly") == "1"
	fams := s.familySet()
	byCard := s.ownedByCard()

	type group struct {
		Code  string    `json:"code"`
		Name  string    `json:"name"`
		Cards []setCard `json:"cards"`
	}
	out := []group{}
	for _, m := range s.cat.SetList() {
		if !fams[m.Family] {
			continue
		}
		cards, name, ok := s.cat.SetCards(m.Code)
		if !ok {
			continue
		}
		miss := make([]setCard, 0)
		for _, c := range s.annotateCards(cards, byCard, goal) {
			if c.Owned || c.Ordered {
				continue // acquired
			}
			if goalOnly && !c.InGoal {
				continue
			}
			miss = append(miss, c)
		}
		if len(miss) > 0 {
			out = append(out, group{Code: m.Code, Name: name, Cards: miss})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// annotateCards turns catalogue cards into setCards annotated with the user's
// ownership/status per card. Shared by the set-detail and search views.
func (s *server) annotateCards(cards []Card, byCard map[string][]Item, goal string) []setCard {
	out := make([]setCard, 0, len(cards))
	for _, c := range cards {
		items := byCard[c.CardID]
		qty := 0
		for _, it := range items {
			if normStatus(it.Status) == statusOwned {
				qty += it.Quantity
			}
		}
		out = append(out, setCard{
			Card:     c,
			Owned:    hasStatus(items, statusOwned),
			Ordered:  hasStatus(items, statusOrdered),
			Wishlist: hasStatus(items, statusWishlist),
			Quantity: qty,
			Items:    items,
			InGoal:   inGoal(parallelLevel(c.CardID, c.Code), goal),
		})
	}
	return out
}
