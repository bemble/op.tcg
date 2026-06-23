package main

import (
	"net/http"
	"sort"
	"strconv"
)

// rarityBucket groups an owned card for the "by rarity" stat: base cards keep
// their rarity; parallels become "P1"/"P2"/…; DON!! splits standard vs gold.
func rarityBucket(cardID, code, rarity string) string {
	level := parallelLevel(cardID, code)
	if rarity == "DON!!" {
		if level > 0 {
			return "DON!! Gold"
		}
		return "DON!!"
	}
	if level > 0 {
		return "P" + strconv.Itoa(level)
	}
	return rarity
}

// rarityRank orders the buckets: base rarities, then parallels, then DON, gold.
func rarityRank(label string) int {
	switch {
	case label == "DON!! Gold":
		return 4
	case label == "DON!!":
		return 3
	case len(label) >= 2 && label[0] == 'P' && label[1] >= '0' && label[1] <= '9':
		return 2
	default:
		return 1
	}
}

// statBucket is a generic labelled stat row. Only the relevant numeric fields
// are populated per section (owned, total, copies).
type statBucket struct {
	Label  string `json:"label"`
	Owned  int    `json:"owned,omitempty"`
	Total  int    `json:"total,omitempty"`
	Copies int    `json:"copies,omitempty"`
}

type fullStats struct {
	Goal           string       `json:"goal"`           // active collection goal
	Owned          int          `json:"owned"`          // distinct owned cards in selected families (all variants)
	Copies         int          `json:"copies"`         // sum of quantities in selected families (all variants)
	GoalOwned      int          `json:"goalOwned"`      // distinct owned cards counting toward the goal
	CatalogueTotal int          `json:"catalogueTotal"` // distinct in-goal cards in the catalogue
	SetsComplete   int          `json:"setsComplete"`
	SetsTotal      int          `json:"setsTotal"`
	ByGroup        []statBucket `json:"byGroup"`    // owned / total per set family (goal-scoped)
	ByOwner        []statBucket `json:"byOwner"`    // owned + copies per owner (full collection)
	ByLanguage     []statBucket `json:"byLanguage"` // copies per language (full collection)
	ByRarity       []statBucket `json:"byRarity"`   // owned per rarity (full collection)
}

// handleFullStats computes collection statistics for the selected set families.
// The completion metrics (catalogue %, sets complete, per-family progress) are
// scoped to the active collection goal (complete/master/wizard) — only cards
// counting toward the goal. The inventory breakdowns (by owner, language and
// rarity) describe the full owned collection within those families, so parallels
// and DON gold show up even when the goal is "complete". Owned counts are
// distinct cards; copies are summed quantities.
func (s *server) handleFullStats(w http.ResponseWriter, r *http.Request) {
	items, err := s.st.ListItems(0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	goal := s.collectionGoal()
	fams := s.familySet()

	owned := map[string]bool{}     // all owned in selected families
	goalOwned := map[string]bool{} // owned cards counting toward the goal
	copies := 0
	ownerCards := map[string]map[string]bool{}
	ownerCopies := map[string]int{}
	langCopies := map[string]int{}
	rarityCards := map[string]map[string]bool{}
	prefixCards := map[string]map[string]bool{} // in-goal owned distinct cards per set prefix

	add := func(m map[string]map[string]bool, key, id string) {
		if m[key] == nil {
			m[key] = map[string]bool{}
		}
		m[key][id] = true
	}

	for _, it := range items {
		rarity, code := "—", it.CardID
		if c, ok := s.cat.Get(it.CardID); ok {
			if c.Rarity != "" {
				rarity = c.Rarity
			}
			if c.Code != "" {
				code = c.Code
			}
		}
		// Family is the only hard filter: the inventory breakdowns cover every
		// owned variant (parallels, gold) of the selected families.
		if !fams[setFamily(codePrefix(code))] {
			continue
		}

		owned[it.CardID] = true
		copies += it.Quantity

		ownerName := it.OwnerName
		if ownerName == "" {
			ownerName = "Non attribué"
		}
		add(ownerCards, ownerName, it.CardID)
		ownerCopies[ownerName] += it.Quantity
		langCopies[it.Language] += it.Quantity
		add(rarityCards, rarityBucket(it.CardID, code, rarity), it.CardID)

		// Completion accounting is restricted to cards counting toward the goal.
		if inGoal(parallelLevel(it.CardID, code), goal) {
			goalOwned[it.CardID] = true
			add(prefixCards, codePrefix(code), it.CardID)
		}
	}

	goalTotals := s.cat.SetGoalTotals(goal)
	catalogueTotal := 0
	for p, n := range goalTotals {
		if fams[setFamily(p)] {
			catalogueTotal += n
		}
	}

	st := fullStats{
		Goal:           goal,
		Owned:          len(owned),
		Copies:         copies,
		GoalOwned:      len(goalOwned),
		CatalogueTotal: catalogueTotal,
		ByGroup:        []statBucket{},
		ByOwner:        []statBucket{},
		ByLanguage:     []statBucket{},
		ByRarity:       []statBucket{},
	}

	// Sets + groups (catalogue order preserves OP, EB, PRB, ST, others).
	metas := s.cat.SetList()
	groupTotal := map[string]int{}
	groupOwned := map[string]int{}
	var groupOrder []string
	for _, m := range metas {
		gt := goalTotals[m.Code]
		if gt == 0 || !fams[m.Family] {
			continue // no in-goal cards, or family not selected
		}
		st.SetsTotal++
		if _, seen := groupTotal[m.Group]; !seen {
			groupOrder = append(groupOrder, m.Group)
		}
		groupTotal[m.Group] += gt
		groupOwned[m.Group] += len(prefixCards[m.Code])
		if len(prefixCards[m.Code]) >= gt {
			st.SetsComplete++
		}
	}
	for _, g := range groupOrder {
		st.ByGroup = append(st.ByGroup, statBucket{Label: g, Owned: groupOwned[g], Total: groupTotal[g]})
	}

	for name, cards := range ownerCards {
		st.ByOwner = append(st.ByOwner, statBucket{Label: name, Owned: len(cards), Copies: ownerCopies[name]})
	}
	sort.Slice(st.ByOwner, func(i, j int) bool { return st.ByOwner[i].Owned > st.ByOwner[j].Owned })

	for lang, n := range langCopies {
		st.ByLanguage = append(st.ByLanguage, statBucket{Label: lang, Copies: n})
	}
	sort.Slice(st.ByLanguage, func(i, j int) bool { return st.ByLanguage[i].Copies > st.ByLanguage[j].Copies })

	for rarity, cards := range rarityCards {
		st.ByRarity = append(st.ByRarity, statBucket{Label: rarity, Owned: len(cards)})
	}
	sort.Slice(st.ByRarity, func(i, j int) bool {
		ri, rj := rarityRank(st.ByRarity[i].Label), rarityRank(st.ByRarity[j].Label)
		if ri != rj {
			return ri < rj
		}
		if ri == 2 { // parallels: P1, P2, P3…
			return st.ByRarity[i].Label < st.ByRarity[j].Label
		}
		return st.ByRarity[i].Owned > st.ByRarity[j].Owned
	})

	writeJSON(w, http.StatusOK, st)
}
