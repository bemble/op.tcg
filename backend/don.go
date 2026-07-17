package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DON!! cards are absent from the official card list, so we source them from
// TCGplayer's public search API (the same one its website grid uses, no auth).
// They're merged into the catalogue alongside the official cards.
//
// Goal mapping: a standard DON counts like a base card (level 0 -> complete);
// a "(Gold)" DON counts like a first parallel (level 1 -> master). We encode
// that in the synthesized id so the existing parallelLevel/inGoal logic applies
// unchanged.
const (
	donSearchURL = "https://mp-search-api.tcgplayer.com/v1/search/request?q=&isList=false"
	donImageBase = "https://tcgplayer-cdn.tcgplayer.com/product/"
	donPageSize  = 48
)

// donReqBody is the search payload; from/size are filled per page.
const donReqBody = `{"algorithm":"sales_synonym_v2","from":%d,"size":%d,"filters":{"term":{"productLineName":["one-piece-card-game"],"productTypeName":["Cards"],"CardType":["DON!!"]},"range":{},"match":{}},"context":{"cart":{},"shippingCountry":"US"},"settings":{"useFuzzySearch":true,"didYouMean":{}},"sort":{}}`

type tcgSearchResp struct {
	Results []struct {
		TotalResults int `json:"totalResults"`
		Results      []struct {
			ProductID   float64 `json:"productId"`
			ProductName string  `json:"productName"`
			SetCode     string  `json:"setCode"`
			SetName     string  `json:"setName"`
			MarketPrice float64 `json:"marketPrice"`
		} `json:"results"`
	} `json:"results"`
}

func donSearchPage(ctx context.Context, client *http.Client, from int) (*tcgSearchResp, error) {
	body := fmt.Sprintf(donReqBody, from, donPageSize)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, donSearchURL, bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://www.tcgplayer.com")
	req.Header.Set("Referer", "https://www.tcgplayer.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 AppleWebKit/537.36 Chrome/124 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tcgplayer search %s: %s", donSearchURL, resp.Status)
	}
	var sr tcgSearchResp
	if err := json.Unmarshal(data, &sr); err != nil {
		return nil, fmt.Errorf("decode tcgplayer search: %w", err)
	}
	if len(sr.Results) == 0 {
		return nil, fmt.Errorf("tcgplayer search: empty results envelope")
	}
	return &sr, nil
}

// donSetPrefix maps a TCGplayer setCode ("OP16", "EB-03", "OP15-EB04",
// "ST-01 PRE", "OP-PR", "OP-DD") to our set prefix; promos/demo -> "P".
func donSetPrefix(setCode string) string {
	m := reSetToken.FindStringSubmatch(setCode) // first LETTERS+DIGITS token
	if m == nil {
		return "P" // OP-PR, OP-DD, …
	}
	return strings.ToUpper(m[1]) + m[2]
}

// isGoldDon reports whether a DON product name is the "(Gold)" variant.
func isGoldDon(name string) bool {
	return strings.Contains(strings.ToLower(name), "(gold)")
}

// donCharKey normalises a DON name to a per-character key by dropping the
// "(Gold)" marker and collapsing whitespace, so a standard and its Gold pair up
// ("DON!! Card (Marco)" == "DON!! Card (Marco) (Gold)").
func donCharKey(name string) string {
	n := strings.ReplaceAll(strings.ToLower(name), "(gold)", "")
	return strings.Join(strings.Fields(n), " ")
}

func donImageURL(pid int64) string {
	return donImageBase + strconv.FormatInt(pid, 10) + "_in_1000x1000.jpg"
}

// donCard builds a catalogue Card for a DON!! entry. productID/marketPrice are
// the TCGplayer source fields kept in raw (a synthesized foil reuses its
// standard's values).
func donCard(cardID, code, name, image string, productID int64, marketPrice float64, setName string) Card {
	raw, _ := json.Marshal(map[string]any{
		"id": cardID, "code": code, "name": name, "rarity": "DON!!",
		"source": "tcgplayer.com", "productId": productID, "marketPrice": marketPrice,
		"setName": setName,
	})
	return Card{
		CardID:     cardID,
		Code:       code,
		Name:       name,
		Rarity:     "DON!!",
		ImageSmall: image,
		ImageLarge: image,
		Raw:        raw,
	}
}

// fetchDonCards pulls all One Piece DON!! cards from TCGplayer and maps them to
// catalogue cards. ~5 paged requests; supplementary, so callers may tolerate
// failure.
//
// Premium Booster (PRB) DON!! cards are special-cased: each character exists in
// three editions — standard, foil and gold — but TCGplayer only lists the
// standard and the gold. We synthesize the foil from the standard (same art),
// and group the three under the standard's base code as parallel levels so they
// sort standard -> foil -> gold and count as base / master / wizard:
//
//	standard  PRB##-DON-<pidStd>        (level 0)
//	foil      PRB##-DON-<pidStd>_p1     (level 1, synthesized, standard's art)
//	gold      PRB##-DON-<pidStd>_p2     (level 2)
//
// The gold previously lived at "PRB##-DON-<pidGold>_p1"; the returned remap maps
// each such old id to its new one so owned copies can be migrated.
func fetchDonCards(ctx context.Context) ([]Card, map[string]string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	first, err := donSearchPage(ctx, client, 0)
	if err != nil {
		return nil, nil, err
	}
	total := first.Results[0].TotalResults
	rows := first.Results[0].Results
	for len(rows) < total {
		pg, err := donSearchPage(ctx, client, len(rows))
		if err != nil {
			return nil, nil, err
		}
		got := pg.Results[0].Results
		if len(got) == 0 {
			break
		}
		rows = append(rows, got...)
	}

	// Index PRB standard product ids by (prefix, character) so each gold can be
	// anchored onto its standard's base code.
	type prbKey struct{ prefix, char string }
	stdPid := map[prbKey]int64{}
	for _, r := range rows {
		prefix := donSetPrefix(r.SetCode)
		if !strings.HasPrefix(prefix, "PRB") || isGoldDon(r.ProductName) {
			continue
		}
		stdPid[prbKey{prefix, donCharKey(r.ProductName)}] = int64(r.ProductID)
	}

	out := make([]Card, 0, len(rows))
	remap := map[string]string{}
	for _, r := range rows {
		pid := int64(r.ProductID)
		if pid <= 0 {
			continue
		}
		prefix := donSetPrefix(r.SetCode)
		gold := isGoldDon(r.ProductName)

		if strings.HasPrefix(prefix, "PRB") {
			if gold {
				base := stdPid[prbKey{prefix, donCharKey(r.ProductName)}]
				if base == 0 {
					base = pid // no matching standard: keep gold on its own code
				}
				code := prefix + "-DON-" + strconv.FormatInt(base, 10)
				cardID := code + "_p2"
				out = append(out, donCard(cardID, code, r.ProductName, donImageURL(pid), pid, r.MarketPrice, r.SetName))
				if old := prefix + "-DON-" + strconv.FormatInt(pid, 10) + "_p1"; old != cardID {
					remap[old] = cardID
				}
			} else {
				code := prefix + "-DON-" + strconv.FormatInt(pid, 10)
				img := donImageURL(pid)
				out = append(out, donCard(code, code, r.ProductName, img, pid, r.MarketPrice, r.SetName))
				// Synthesized foil: standard's art, one parallel level up.
				foil := r.ProductName + " (Foil)"
				out = append(out, donCard(code+"_p1", code, foil, img, pid, r.MarketPrice, r.SetName))
			}
			continue
		}

		// Non-PRB: standard at level 0, gold as a single parallel (master).
		code := prefix + "-DON-" + strconv.FormatInt(pid, 10)
		cardID := code
		if gold {
			cardID += "_p1"
		}
		out = append(out, donCard(cardID, code, r.ProductName, donImageURL(pid), pid, r.MarketPrice, r.SetName))
	}
	return out, remap, nil
}
