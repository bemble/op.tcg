package main

import (
	"bufio"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port          string
	DBPath        string
	WebDir        string
	CataloguePath string // JSON file the catalogue is read from / written to
	CatalogueSeed string // optional bundled JSON used to seed CataloguePath if missing

	SyncOnStart bool          // auto-sync the catalogue on startup (background)
	SyncMaxAge  time.Duration // skip startup auto-sync if catalogue is younger than this (0 = always sync)

	ImageCache    bool   // persist proxied (small) images to disk to avoid refetching
	ImageCacheDir string // directory for the on-disk image cache
	ImageWidth    int    // downscale proxied images to this width (px); 0 disables resizing

	MCPEnabled  bool   // expose the MCP server at /mcp
	MCPReadOnly bool   // only register read tools (no add/update/remove)
	MCPToken    string // if set, /mcp requires Authorization: Bearer <token>
}

func loadConfig() Config {
	loadDotenv()
	return Config{
		Port: env("PORT", "8080"),
		// Defaults are local-dev friendly (relative to the working directory).
		// The Docker image overrides DB_PATH / CATALOGUE_PATH to absolute /data
		// paths via its ENV block.
		DBPath:        env("DB_PATH", "./data/onepiece.db"),
		WebDir:        env("WEB_DIR", "./web"),
		CataloguePath: env("CATALOGUE_PATH", "./catalogue.json"),
		CatalogueSeed: os.Getenv("CATALOGUE_SEED"),

		SyncOnStart: envBool("SYNC_ON_START", true),
		SyncMaxAge:  time.Duration(envInt("SYNC_MAX_AGE_HOURS", 24)) * time.Hour,

		ImageCache:    envBool("IMAGE_CACHE", false),
		ImageCacheDir: env("IMAGE_CACHE_DIR", "./data/imgcache"),
		ImageWidth:    envInt("IMAGE_WIDTH", 220),

		MCPEnabled:  envBool("MCP_ENABLED", true),
		MCPReadOnly: envBool("MCP_READ_ONLY", false),
		MCPToken:    strings.TrimSpace(os.Getenv("MCP_TOKEN")),
	}
}

// envBool parses a boolean env var. Truthy: 1, true, yes, on (case-insensitive).
func envBool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

// envInt parses an int env var, falling back to def when unset/invalid.
func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// loadDotenv loads KEY=VALUE pairs from a .env file into the environment, for
// local dev convenience (`go run .` without a long command line). Real
// environment variables always win, so Docker/compose — which inject vars
// directly — are unaffected. Looked up via ENV_FILE, else ./.env, else ../.env
// (so it works when run from backend/). Missing file = silent no-op.
func loadDotenv() {
	path := os.Getenv("ENV_FILE")
	if path == "" {
		for _, c := range []string{".env", "../.env"} {
			if _, err := os.Stat(c); err == nil {
				path = c
				break
			}
		}
	}
	if path == "" {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	loaded := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if key == "" {
			continue
		}
		if _, set := os.LookupEnv(key); set {
			continue // real env wins
		}
		os.Setenv(key, unquote(strings.TrimSpace(line[eq+1:])))
		loaded++
	}
	if loaded > 0 {
		log.Printf("config: loaded %d var(s) from %s", loaded, path)
	}
}

// unquote strips a single pair of matching surrounding quotes, if present.
func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
