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

// fetchDonCards pulls all One Piece DON!! cards from TCGplayer and maps them to
// catalogue cards. ~5 paged requests; supplementary, so callers may tolerate
// failure.
func fetchDonCards(ctx context.Context) ([]Card, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	first, err := donSearchPage(ctx, client, 0)
	if err != nil {
		return nil, err
	}
	total := first.Results[0].TotalResults
	rows := first.Results[0].Results
	for len(rows) < total {
		pg, err := donSearchPage(ctx, client, len(rows))
		if err != nil {
			return nil, err
		}
		got := pg.Results[0].Results
		if len(got) == 0 {
			break
		}
		rows = append(rows, got...)
	}

	out := make([]Card, 0, len(rows))
	for _, r := range rows {
		pid := int64(r.ProductID)
		if pid <= 0 {
			continue
		}
		prefix := donSetPrefix(r.SetCode)
		gold := strings.Contains(strings.ToLower(r.ProductName), "(gold)")

		code := prefix + "-DON-" + strconv.FormatInt(pid, 10)
		cardID := code
		if gold {
			cardID += "_p1" // level 1 -> counts from "master"
		}
		image := donImageBase + strconv.FormatInt(pid, 10) + "_in_1000x1000.jpg"

		raw, _ := json.Marshal(map[string]any{
			"id": cardID, "code": code, "name": r.ProductName, "rarity": "DON!!",
			"source": "tcgplayer.com", "productId": pid, "marketPrice": r.MarketPrice,
			"setName": r.SetName,
		})
		out = append(out, Card{
			CardID:     cardID,
			Code:       code,
			Name:       r.ProductName,
			Rarity:     "DON!!",
			ImageSmall: image,
			ImageLarge: image,
			Raw:        raw,
		})
	}
	return out, nil
}
