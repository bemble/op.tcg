package main

import (
	"encoding/json"
	"net/http"
)

const (
	settingCollectionGoal = "collection_goal"
	settingGoalFamilies   = "goal_families"
)

type settingsOut struct {
	CollectionGoal string   `json:"collectionGoal"`
	Families       []string `json:"families"`
}

// collectionGoal returns the stored collection goal, defaulting to master.
func (s *server) collectionGoal() string {
	g, _ := s.st.GetSetting(settingCollectionGoal, goalMaster)
	if !validGoal(g) {
		return goalMaster
	}
	return g
}

func validFamily(k string) bool {
	_, ok := familyLabels[k]
	return ok
}

// collectionFamilies returns the set families included in the goal. Defaults to
// just the main series ("main"); never returns empty.
func (s *server) collectionFamilies() []string {
	raw, _ := s.st.GetSetting(settingGoalFamilies, "")
	var fs []string
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &fs)
	}
	out := make([]string, 0, len(fs))
	seen := map[string]bool{}
	for _, f := range fs {
		if validFamily(f) && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return []string{"main"}
	}
	return out
}

// familySet returns the selected families as a lookup set.
func (s *server) familySet() map[string]bool {
	m := map[string]bool{}
	for _, f := range s.collectionFamilies() {
		m[f] = true
	}
	return m
}

func (s *server) settings() settingsOut {
	return settingsOut{CollectionGoal: s.collectionGoal(), Families: s.collectionFamilies()}
}

func (s *server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.settings())
}

func (s *server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CollectionGoal string    `json:"collectionGoal"`
		Families       *[]string `json:"families"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.CollectionGoal != "" {
		if !validGoal(req.CollectionGoal) {
			writeErr(w, http.StatusBadRequest, "objectif invalide (complete|master|wizard)")
			return
		}
		if err := s.st.SetSetting(settingCollectionGoal, req.CollectionGoal); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if req.Families != nil {
		var keep []string
		for _, f := range *req.Families {
			if !validFamily(f) {
				writeErr(w, http.StatusBadRequest, "famille invalide: "+f)
				return
			}
			keep = append(keep, f)
		}
		blob, _ := json.Marshal(keep)
		if err := s.st.SetSetting(settingGoalFamilies, string(blob)); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, s.settings())
}
