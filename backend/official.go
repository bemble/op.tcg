package main

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Scraper for the official One Piece card list at en.onepiece-cardgame.com.
// It replaces ApiTCG as the catalogue source: full set coverage (OP-01..latest,
// ST, EB, PRB, promos), official images, no API key, no request quota.
//
// The page renders all cards server-side as <dl class="modalCol" id="CODE">
// blocks. There is one page per "series" (set); a <select name="series"> lists
// them. We fetch every series and dedupe by card id (parallels have ids like
// OP16-001_p1, which we keep as distinct cards).

const (
	officialBase     = "https://en.onepiece-cardgame.com"
	officialCardlist = officialBase + "/cardlist"
	officialUA       = "Mozilla/5.0 (compatible; one-piece-collect/1.0; +https://github.com/)"
)

type officialClient struct {
	http *http.Client
}

func newOfficialClient() *officialClient {
	return &officialClient{http: &http.Client{Timeout: 30 * time.Second}}
}

var (
	reSeriesSelect = regexp.MustCompile(`(?s)<select[^>]*name="?series"?[^>]*>(.*?)</select>`)
	reOption       = regexp.MustCompile(`(?s)<option[^>]*value="?([^"'>]+)"?[^>]*>(.*?)</option>`)
	reBlock        = regexp.MustCompile(`(?s)<dl class="modalCol" id="([^"]+)">(.*?)</dl>`)
	reInfoCol      = regexp.MustCompile(`(?s)<div class="infoCol">(.*?)</div>`)
	reSpan         = regexp.MustCompile(`(?s)<span>(.*?)</span>`)
	reName         = regexp.MustCompile(`(?s)<div class="cardName">(.*?)</div>`)
	reImg          = regexp.MustCompile(`(?:data-src|src)="([^"]*cardlist/card/[^"]+)"`)
	reBracketBody  = regexp.MustCompile(`\[([^\]]+)\]`)           // contents of [...]
	reSetToken     = regexp.MustCompile(`([A-Za-z]{1,3})-?(\d+)`) // "OP-16" / "OP15" / "EB04"
	reDashName     = regexp.MustCompile(`-\s*(.+?)\s*-`)
	reField        = regexp.MustCompile(`(?s)<h3>(.*?)</h3>(.*?)(?:<h3>|</div>)`)
	reTag          = regexp.MustCompile(`<[^>]+>`)
	reWS           = regexp.MustCompile(`\s+`)
)

func (c *officialClient) get(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", officialUA)
	req.Header.Set("Accept", "text/html")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("official %s: %s", url, resp.Status)
	}
	return string(body), nil
}

func stripTags(s string) string {
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.TrimSpace(reWS.ReplaceAllString(s, " "))
}

// series is one set entry from the <select>: an opaque id plus its set code
// (e.g. "OP16") and display name.
type series struct {
	id      string
	setCode string // normalized prefix, e.g. "OP16", "ST01", "EB01"; "" for promo/other
	setName string
}

// parseSeries extracts the set list and a setCode->setName map from the page.
func parseSeries(page string) ([]series, map[string]string) {
	var out []series
	names := map[string]string{}
	m := reSeriesSelect.FindStringSubmatch(page)
	if m == nil {
		return out, names
	}
	for _, o := range reOption.FindAllStringSubmatch(m[1], -1) {
		id := strings.TrimSpace(o[1])
		if id == "" {
			continue
		}
		label := stripTags(o[2])
		s := series{id: id}

		// Set name = the text between the first pair of dashes, e.g.
		// "BOOSTER PACK -THE TIME OF BATTLE- [OP-16]" -> "THE TIME OF BATTLE".
		if dn := reDashName.FindStringSubmatch(label); dn != nil {
			s.setName = strings.TrimSpace(dn[1])
		}

		// The bracket may list one or several set codes — e.g. "[OP-16]" or the
		// dual-set boosters "[OP15-EB04]". Map the name to every code prefix.
		if bb := reBracketBody.FindStringSubmatch(label); bb != nil {
			for _, t := range reSetToken.FindAllStringSubmatch(bb[1], -1) {
				prefix := strings.ToUpper(t[1]) + t[2] // "OP"+"16" -> "OP16"
				if s.setCode == "" {
					s.setCode = prefix
				}
				if s.setName != "" {
					if _, exists := names[prefix]; !exists {
						names[prefix] = s.setName
					}
				}
			}
		}
		if s.setName == "" {
			s.setName = label // Promotion card / Other Product Card
		}
		out = append(out, s)
	}
	return out, names
}

// parseCards extracts every card block on a series page.
func parseCards(page string, setNames map[string]string) []Card {
	var cards []Card
	for _, b := range reBlock.FindAllStringSubmatch(page, -1) {
		id, body := b[1], b[2]
		c := Card{CardID: id}

		if info := reInfoCol.FindStringSubmatch(body); info != nil {
			spans := reSpan.FindAllStringSubmatch(info[1], -1)
			if len(spans) > 0 {
				c.Code = stripTags(spans[0][1])
			}
			if len(spans) > 1 {
				c.Rarity = stripTags(spans[1][1])
			}
		}
		if c.Code == "" {
			c.Code = id
		}
		if n := reName.FindStringSubmatch(body); n != nil {
			c.Name = stripTags(n[1])
		}
		if im := reImg.FindStringSubmatch(body); im != nil {
			c.ImageLarge = absURL(im[1])
			c.ImageSmall = c.ImageLarge
		}

		// Set name comes from the dropdown label (authoritative per set), mapped
		// by code prefix. The per-card "Card Set(s)" field lists each card's
		// individual product history, so it's kept in raw but not used here.
		prefix := codePrefix(c.Code)
		if name, ok := setNames[prefix]; ok {
			c.SetName = name
		} else if prefix == "P" {
			c.SetName = "Promotion"
		}

		c.Raw = officialRaw(c, body)
		cards = append(cards, c)
	}
	return cards
}

// codePrefix turns "OP16-001" / "OP16-001_p1" into "OP16".
func codePrefix(code string) string {
	if i := strings.IndexByte(code, '-'); i > 0 {
		return code[:i]
	}
	return code
}

func absURL(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimPrefix(u, "..")
	if strings.HasPrefix(u, "http") {
		return u
	}
	if !strings.HasPrefix(u, "/") {
		u = "/" + u
	}
	return officialBase + u
}

// officialRaw captures the extra gameplay fields (category, color, type, power,
// counter, attribute, cost/life, effect, trigger) as a JSON object, so nothing
// is lost even though the UI doesn't use them yet.
func officialRaw(c Card, body string) []byte {
	fields := map[string]string{}
	if info := reInfoCol.FindStringSubmatch(body); info != nil {
		spans := reSpan.FindAllStringSubmatch(info[1], -1)
		if len(spans) > 2 {
			fields["category"] = stripTags(spans[2][1])
		}
	}
	for _, f := range reField.FindAllStringSubmatch(body, -1) {
		label := stripTags(f[1])
		val := stripTags(f[2])
		if label != "" && val != "" && val != "-" {
			fields[label] = val
		}
	}

	var sb strings.Builder
	sb.WriteString("{")
	sb.WriteString(`"id":` + jsonStr(c.CardID))
	sb.WriteString(`,"code":` + jsonStr(c.Code))
	sb.WriteString(`,"name":` + jsonStr(c.Name))
	sb.WriteString(`,"rarity":` + jsonStr(c.Rarity))
	sb.WriteString(`,"set":` + jsonStr(c.SetName))
	sb.WriteString(`,"image":` + jsonStr(c.ImageLarge))
	for k, v := range fields {
		sb.WriteString(`,` + jsonStr(k) + `:` + jsonStr(v))
	}
	sb.WriteString("}")
	return []byte(sb.String())
}

func jsonStr(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// fetchCatalogue scrapes every series and returns the deduped card list plus
// the number of series pages fetched.
func (c *officialClient) fetchCatalogue(ctx context.Context) ([]Card, int, error) {
	index, err := c.get(ctx, officialCardlist)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch cardlist index: %w", err)
	}
	allSeries, setNames := parseSeries(index)
	if len(allSeries) == 0 {
		return nil, 0, fmt.Errorf("no series found on cardlist page (layout changed?)")
	}

	byID := map[string]Card{}
	pages := 0
	for _, s := range allSeries {
		pageCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		page, err := c.get(pageCtx, officialCardlist+"?series="+s.id)
		cancel()
		if err != nil {
			return nil, pages, fmt.Errorf("series %s: %w", s.id, err)
		}
		pages++
		for _, card := range parseCards(page, setNames) {
			if _, seen := byID[card.CardID]; !seen {
				byID[card.CardID] = card
			}
		}
	}

	out := make([]Card, 0, len(byID))
	for _, card := range byID {
		out = append(out, card)
	}
	return out, pages, nil
}
