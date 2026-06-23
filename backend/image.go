package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"image"
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/draw"
)

// allowedImageHosts whitelists the upstreams we proxy. Keeping it tight avoids
// turning this endpoint into an open proxy / SSRF vector.
var allowedImageHosts = map[string]bool{
	"en.onepiece-cardgame.com":      true,
	"onepiece-cardgame.com":         true,
	"asia-en.onepiece-cardgame.com": true,
	"tcgplayer-cdn.tcgplayer.com":   true, // DON!! card images
}

// jpegQuality for re-encoded images. Card art is opaque/photographic, so JPEG
// at this quality is visually lossless at display size and far smaller than the
// source PNG.
const jpegQuality = 82

var imgClient = &http.Client{Timeout: 15 * time.Second}

// handleImageProxy fetches a remote card image server-side and re-serves it
// from our own origin. The card image hosts set
// `Cross-Origin-Resource-Policy: same-site`, which makes browsers block the
// image when loaded directly from a different origin (ERR_BLOCKED_BY_RESPONSE
// .NotSameSite). Proxying strips that restriction since the browser now sees a
// same-origin response.
//
// Images are downscaled to cfg.ImageWidth (the grid shows them ~220px wide, so
// the full-size source is wasteful) and re-encoded as JPEG. When cfg.ImageCache
// is set the processed result is cached on disk so each card is fetched and
// resized only once.
func (s *server) handleImageProxy(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("u")
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "missing 'u' query param")
		return
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		writeErr(w, http.StatusBadRequest, "invalid url")
		return
	}
	if !allowedImageHosts[strings.ToLower(u.Hostname())] {
		writeErr(w, http.StatusForbidden, "host not allowed")
		return
	}

	// Width: ?w overrides the configured default (the grid uses the default;
	// the click-to-zoom view requests w=0 for the full-size source). 0 keeps
	// the original dimensions; clamp the upper bound so callers can't request
	// absurd sizes.
	width := s.cfg.ImageWidth
	if wq := r.URL.Query().Get("w"); wq != "" {
		if n, err := strconv.Atoi(wq); err == nil && n >= 0 && n <= 1200 {
			width = n
		}
	}

	// Serve from disk cache if present.
	cachePath := s.imageCachePath(raw, width)
	if cachePath != "" {
		if data, err := os.ReadFile(cachePath); err == nil {
			s.writeImage(w, data)
			return
		}
	}

	src, err := s.fetchImage(r, u.String())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	out, err := processImage(src, width)
	if err != nil {
		// Couldn't decode/resize (unexpected format) — serve the original bytes.
		out = src
	}

	if cachePath != "" {
		writeCacheFile(cachePath, out)
	}
	s.writeImage(w, out)
}

func (s *server) fetchImage(r *http.Request, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	// Some hosts are picky about a real-ish UA; harmless otherwise.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; one-piece-collect/1.0)")

	resp, err := imgClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusErr{resp.Status}
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20)) // 16 MiB safety cap
}

type httpStatusErr struct{ status string }

func (e *httpStatusErr) Error() string { return "upstream status " + e.status }

func (s *server) writeImage(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/jpeg")
	// Card art is immutable, so let the browser cache aggressively too.
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// processImage decodes src, downscales it to maxWidth (never upscales) and
// re-encodes as JPEG. maxWidth <= 0 keeps the original dimensions but still
// re-encodes to JPEG.
func processImage(src []byte, maxWidth int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}

	b := img.Bounds()
	dst := img
	if maxWidth > 0 && b.Dx() > maxWidth {
		h := b.Dy() * maxWidth / b.Dx()
		scaled := image.NewRGBA(image.Rect(0, 0, maxWidth, h))
		draw.CatmullRom.Scale(scaled, scaled.Bounds(), img, b, draw.Over, nil)
		dst = scaled
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// imageCachePath returns the on-disk path for a source URL, or "" when caching
// is disabled. The key folds in the target width so changing IMAGE_WIDTH does
// not serve stale dimensions.
func (s *server) imageCachePath(rawURL string, width int) string {
	if !s.cfg.ImageCache {
		return ""
	}
	sum := sha1.Sum([]byte(rawURL))
	name := hex.EncodeToString(sum[:]) + "_w" + strconv.Itoa(width) + ".jpg"
	return filepath.Join(s.cfg.ImageCacheDir, name)
}

func writeCacheFile(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	// Write atomically so a crash mid-write never leaves a truncated cache file.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
