package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCP server: exposes the collection (read + write) as tools so an assistant can
// query and manage it. Mounted at /mcp (streamable HTTP). It reuses the same
// store/catalogue as the REST API, so everything stays consistent.

// ---- DTOs (compact, assistant-friendly shapes) ----

type mcpItem struct {
	ID       int64  `json:"id"`
	OwnerID  *int64 `json:"ownerId,omitempty"`
	Owner    string `json:"owner,omitempty"`
	Language string `json:"language,omitempty"`
	Quantity int    `json:"quantity"`
	Status   string `json:"status"`
	Notes    string `json:"notes,omitempty"`
}

type mcpCard struct {
	CardID   string    `json:"cardId"`
	Code     string    `json:"code"`
	Name     string    `json:"name"`
	Rarity   string    `json:"rarity"`
	SetName  string    `json:"setName,omitempty"`
	Owned    bool      `json:"owned"`
	Ordered  bool      `json:"ordered"`
	Wishlist bool      `json:"wishlist"`
	Quantity int       `json:"quantity"`
	InGoal   bool      `json:"inGoal"`
	Items    []mcpItem `json:"items,omitempty"`
}

func toMCPItem(it Item) mcpItem {
	return mcpItem{
		ID: it.ID, OwnerID: it.OwnerID, Owner: it.OwnerName, Language: it.Language,
		Quantity: it.Quantity, Status: normStatus(it.Status), Notes: it.Notes,
	}
}

func toMCPCard(c setCard) mcpCard {
	items := make([]mcpItem, 0, len(c.Items))
	for _, it := range c.Items {
		items = append(items, toMCPItem(it))
	}
	return mcpCard{
		CardID: c.CardID, Code: c.Code, Name: c.Name, Rarity: c.Rarity, SetName: c.SetName,
		Owned: c.Owned, Ordered: c.Ordered, Wishlist: c.Wishlist, Quantity: c.Quantity,
		InGoal: c.InGoal, Items: items,
	}
}

func (s *server) annotateToMCP(cards []Card) []mcpCard {
	ann := s.annotateCards(cards, s.ownedByCard(), s.collectionGoal())
	out := make([]mcpCard, 0, len(ann))
	for _, c := range ann {
		out = append(out, toMCPCard(c))
	}
	return out
}

// ---- server wiring ----

// mcpHandler builds the MCP server (once) and returns an HTTP handler for it,
// guarded by the optional bearer token.
func (s *server) mcpHandler() http.Handler {
	srv := s.newMCPServer()
	stream := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.MCPToken != "" && r.Header.Get("Authorization") != "Bearer "+s.cfg.MCPToken {
			writeErr(w, http.StatusUnauthorized, "unauthorized (missing/invalid MCP token)")
			return
		}
		stream.ServeHTTP(w, r)
	})
}

func (s *server) newMCPServer() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "op.tcg", Version: "1"}, nil)
	s.registerMCPTools(srv)
	return srv
}

// tool registers a typed tool with the standard "(nil, out, nil)" success shape.
func mcpTool[In, Out any](srv *mcp.Server, name, desc string, fn func(context.Context, In) (Out, error)) {
	mcp.AddTool(srv, &mcp.Tool{Name: name, Description: desc},
		func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
			out, err := fn(ctx, in)
			return nil, out, err
		})
}

func (s *server) registerMCPTools(srv *mcp.Server) {
	// ---------- read ----------

	type searchIn struct {
		Query string `json:"query" jsonschema:"card name or code to search (empty = browse all)"`
		Limit int    `json:"limit,omitempty" jsonschema:"max results (default 50)"`
	}
	mcpTool(srv, "search_cards",
		"Search the One Piece catalogue by name or code, annotated with what the user owns/ordered/wishlists.",
		func(_ context.Context, in searchIn) (struct {
			Total int       `json:"total"`
			Cards []mcpCard `json:"cards"`
		}, error) {
			limit := in.Limit
			if limit <= 0 || limit > 200 {
				limit = 50
			}
			cards, total := s.cat.Search(in.Query, 1, limit)
			return struct {
				Total int       `json:"total"`
				Cards []mcpCard `json:"cards"`
			}{total, s.annotateToMCP(cards)}, nil
		})

	type cardIn struct {
		CardID string `json:"cardId" jsonschema:"the catalogue card id, e.g. OP01-001 or OP01-001_p1"`
	}
	mcpTool(srv, "get_card",
		"Get one catalogue card with the user's ownership/status for it.",
		func(_ context.Context, in cardIn) (mcpCard, error) {
			c, ok := s.cat.Get(in.CardID)
			if !ok {
				return mcpCard{}, fmt.Errorf("carte inconnue: %s", in.CardID)
			}
			return s.annotateToMCP([]Card{c})[0], nil
		})

	mcpTool(srv, "list_sets",
		"List every set with its completion (acquired/total) scoped to the active collection goal.",
		func(_ context.Context, _ struct{}) (struct {
			Sets []setMetaOut `json:"sets"`
		}, error) {
			goal := s.collectionGoal()
			totals := s.cat.SetGoalTotals(goal)
			owned := s.ownedCountsForGoal(s.ownedByCard(), goal)
			metas := s.cat.SetList()
			out := make([]setMetaOut, 0, len(metas))
			for _, m := range metas {
				m.Total = totals[m.Code]
				out = append(out, setMetaOut{SetMeta: m, Owned: owned[m.Code]})
			}
			return struct {
				Sets []setMetaOut `json:"sets"`
			}{out}, nil
		})

	type setIn struct {
		Code string `json:"code" jsonschema:"set code, e.g. OP16, EB03, PRB02, ST01, P, DON"`
	}
	mcpTool(srv, "get_set",
		"Get all cards of a set (incl. parallels) annotated with the user's ownership/status.",
		func(_ context.Context, in setIn) (struct {
			Code  string    `json:"code"`
			Name  string    `json:"name"`
			Total int       `json:"total"`
			Owned int       `json:"owned"`
			Cards []mcpCard `json:"cards"`
		}, error) {
			cards, name, ok := s.cat.SetCards(in.Code)
			if !ok {
				return struct {
					Code  string    `json:"code"`
					Name  string    `json:"name"`
					Total int       `json:"total"`
					Owned int       `json:"owned"`
					Cards []mcpCard `json:"cards"`
				}{}, fmt.Errorf("set inconnu: %s", in.Code)
			}
			ann := s.annotateCards(cards, s.ownedByCard(), s.collectionGoal())
			total, acq := 0, 0
			out := make([]mcpCard, 0, len(ann))
			for _, c := range ann {
				if c.InGoal {
					total++
					if c.Owned || c.Ordered {
						acq++
					}
				}
				out = append(out, toMCPCard(c))
			}
			return struct {
				Code  string    `json:"code"`
				Name  string    `json:"name"`
				Total int       `json:"total"`
				Owned int       `json:"owned"`
				Cards []mcpCard `json:"cards"`
			}{in.Code, name, total, acq, out}, nil
		})

	type missingIn struct {
		GoalOnly *bool  `json:"goalOnly,omitempty" jsonschema:"only cards counting toward the goal (default true)"`
		Family   string `json:"family,omitempty" jsonschema:"restrict to one family: main, eb, prb, deck, promo"`
	}
	type missingGroup struct {
		Code  string    `json:"code"`
		Name  string    `json:"name"`
		Cards []mcpCard `json:"cards"`
	}
	mcpTool(srv, "missing_cards",
		"List the not-acquired (neither owned nor ordered) cards, grouped by set.",
		func(_ context.Context, in missingIn) (struct {
			Groups []missingGroup `json:"groups"`
		}, error) {
			goal := s.collectionGoal()
			goalOnly := in.GoalOnly == nil || *in.GoalOnly
			fams := s.familySet()
			byCard := s.ownedByCard()
			var groups []missingGroup
			for _, m := range s.cat.SetList() {
				if in.Family != "" {
					if m.Family != in.Family {
						continue
					}
				} else if !fams[m.Family] {
					continue
				}
				cards, name, ok := s.cat.SetCards(m.Code)
				if !ok {
					continue
				}
				var miss []mcpCard
				for _, c := range s.annotateCards(cards, byCard, goal) {
					if c.Owned || c.Ordered {
						continue
					}
					if goalOnly && !c.InGoal {
						continue
					}
					miss = append(miss, toMCPCard(c))
				}
				if len(miss) > 0 {
					groups = append(groups, missingGroup{m.Code, name, miss})
				}
			}
			return struct {
				Groups []missingGroup `json:"groups"`
			}{groups}, nil
		})

	mcpTool(srv, "collection_stats",
		"Collection completion stats scoped to the active goal: overall, by family, owner, language and rarity.",
		func(_ context.Context, _ struct{}) (fullStats, error) {
			return s.computeFullStats()
		})

	mcpTool(srv, "list_owners",
		"List the co-owners of the collection.",
		func(_ context.Context, _ struct{}) (struct {
			Owners []Owner `json:"owners"`
		}, error) {
			owners, err := s.st.ListOwners()
			return struct {
				Owners []Owner `json:"owners"`
			}{owners}, err
		})

	// ---------- write ----------

	if s.cfg.MCPReadOnly {
		log.Printf("MCP server ready (read-only, 7 tools) at /mcp")
		return
	}

	type addIn struct {
		CardID   string `json:"cardId" jsonschema:"catalogue card id (use search_cards to find it)"`
		Status   string `json:"status,omitempty" jsonschema:"owned (default), ordered or wishlist"`
		OwnerID  *int64 `json:"ownerId,omitempty" jsonschema:"co-owner id (from list_owners); omit for unassigned"`
		Language string `json:"language,omitempty" jsonschema:"EN, FR or JP (owned/ordered only; default EN)"`
		Quantity int    `json:"quantity,omitempty" jsonschema:"copies to add (owned only; default 1)"`
		Notes    string `json:"notes,omitempty"`
	}
	mcpTool(srv, "add_card",
		"Add/track a card in the collection with a status (owned, ordered or wishlist).",
		func(_ context.Context, in addIn) (mcpItem, error) {
			if _, ok := s.cat.Get(in.CardID); !ok {
				return mcpItem{}, fmt.Errorf("carte absente du catalogue: %s", in.CardID)
			}
			status := normStatus(in.Status)
			if !validStatus(status) {
				return mcpItem{}, fmt.Errorf("statut invalide: %s", in.Status)
			}
			out, err := s.st.AddItem(in.CardID, Item{
				OwnerID: in.OwnerID, Quantity: in.Quantity,
				Language: orDefault(in.Language, "EN"), Notes: in.Notes, Status: status,
			})
			if err != nil {
				return mcpItem{}, err
			}
			return toMCPItem(*out), nil
		})

	type updIn struct {
		ItemID   int64   `json:"itemId" jsonschema:"the possession id (item.id from a card's items)"`
		Status   *string `json:"status,omitempty"`
		OwnerID  *int64  `json:"ownerId,omitempty"`
		Language *string `json:"language,omitempty"`
		Quantity *int    `json:"quantity,omitempty" jsonschema:"owned only; 0 deletes the copy"`
		Notes    *string `json:"notes,omitempty"`
	}
	mcpTool(srv, "update_possession",
		"Update a possession (status, owner, language, quantity, notes). Omitted fields are kept.",
		func(_ context.Context, in updIn) (struct {
			Deleted bool     `json:"deleted"`
			Item    *mcpItem `json:"item,omitempty"`
		}, error) {
			cur, err := s.st.GetItem(in.ItemID)
			if err != nil {
				return struct {
					Deleted bool     `json:"deleted"`
					Item    *mcpItem `json:"item,omitempty"`
				}{}, fmt.Errorf("exemplaire introuvable: %d", in.ItemID)
			}
			m := *cur
			if in.Status != nil {
				st := normStatus(*in.Status)
				if !validStatus(st) {
					return struct {
						Deleted bool     `json:"deleted"`
						Item    *mcpItem `json:"item,omitempty"`
					}{}, fmt.Errorf("statut invalide: %s", *in.Status)
				}
				m.Status = st
			}
			if in.OwnerID != nil {
				if *in.OwnerID <= 0 {
					m.OwnerID = nil
				} else {
					m.OwnerID = in.OwnerID
				}
			}
			if in.Language != nil {
				m.Language = *in.Language
			}
			if in.Quantity != nil {
				m.Quantity = *in.Quantity
			}
			if in.Notes != nil {
				m.Notes = *in.Notes
			}
			out, err := s.st.UpdateItem(in.ItemID, m)
			if err != nil {
				return struct {
					Deleted bool     `json:"deleted"`
					Item    *mcpItem `json:"item,omitempty"`
				}{}, err
			}
			if out == nil {
				return struct {
					Deleted bool     `json:"deleted"`
					Item    *mcpItem `json:"item,omitempty"`
				}{Deleted: true}, nil
			}
			mi := toMCPItem(*out)
			return struct {
				Deleted bool     `json:"deleted"`
				Item    *mcpItem `json:"item,omitempty"`
			}{Item: &mi}, nil
		})

	type rmIn struct {
		ItemID int64 `json:"itemId" jsonschema:"the possession id to remove"`
	}
	mcpTool(srv, "remove_possession",
		"Remove a possession by its item id.",
		func(_ context.Context, in rmIn) (struct {
			OK bool `json:"ok"`
		}, error) {
			err := s.st.DeleteItem(in.ItemID)
			return struct {
				OK bool `json:"ok"`
			}{err == nil}, err
		})

	type bulkIn struct {
		ItemIDs  []int64 `json:"itemIds" jsonschema:"possession ids to re-language"`
		Language string  `json:"language" jsonschema:"target language: EN, FR or JP"`
	}
	mcpTool(srv, "set_language_bulk",
		"Change the language of several owned possessions at once.",
		func(_ context.Context, in bulkIn) (struct {
			Updated int `json:"updated"`
		}, error) {
			if in.Language == "" {
				return struct {
					Updated int `json:"updated"`
				}{}, fmt.Errorf("langue manquante")
			}
			n, err := s.st.BulkSetLanguageItems(in.ItemIDs, in.Language)
			return struct {
				Updated int `json:"updated"`
			}{n}, err
		})

	type ownerIn struct {
		Name string `json:"name" jsonschema:"co-owner name"`
	}
	mcpTool(srv, "add_owner",
		"Add a co-owner (returned owner id can be used in add_card).",
		func(_ context.Context, in ownerIn) (*Owner, error) {
			if in.Name == "" {
				return nil, fmt.Errorf("nom requis")
			}
			return s.st.AddOwner(in.Name)
		})

	type curatedIn struct {
		URL       string `json:"url,omitempty" jsonschema:"TCGplayer product URL or id (auto-fills name/number/image)"`
		Code      string `json:"code,omitempty" jsonschema:"manual: official code, e.g. OP08-043 (a parallel slot is picked if it exists)"`
		Name      string `json:"name,omitempty" jsonschema:"manual: card name"`
		Rarity    string `json:"rarity,omitempty" jsonschema:"manual: rarity (default PR)"`
		ImageURL  string `json:"imageUrl,omitempty" jsonschema:"manual: direct image URL (downloaded + stored)"`
		SourceURL string `json:"sourceUrl,omitempty" jsonschema:"manual: page it came from (e.g. Cardmarket)"`
	}
	mcpTool(srv, "add_missing_card",
		"Add a card the automated sources miss (TCGplayer URL, or a manual code+name for Cardmarket-only cards).",
		func(ctx context.Context, in curatedIn) (mcpCard, error) {
			c, err := s.buildCurated(ctx, curatedReq{
				URL: in.URL, Code: in.Code, Name: in.Name, Rarity: in.Rarity,
				ImageURL: in.ImageURL, SourceURL: in.SourceURL,
			})
			if err != nil {
				return mcpCard{}, err
			}
			if err := s.persistCurated(c); err != nil {
				return mcpCard{}, err
			}
			got, ok := s.cat.Get(c.cardID)
			if !ok {
				return mcpCard{}, fmt.Errorf("carte ajoutée mais introuvable: %s", c.cardID)
			}
			return s.annotateToMCP([]Card{got})[0], nil
		})

	log.Printf("MCP server ready (%d tools) at /mcp", 13)
}
