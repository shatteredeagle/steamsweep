// Package update checks GitHub releases for a newer steamsweep and replaces the running binary.
package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	Repo        = "ShatteredEagle/steamsweep"
	maxDownload = 100 << 20
)

// Release is the part of a GitHub release we use.
type Release struct {
	Tag    string
	Assets map[string]string // asset name -> download URL
}

// Updater talks to GitHub. The zero value uses the real API.
type Updater struct {
	API    string // e.g. https://api.github.com/repos/ShatteredEagle/steamsweep
	Client *http.Client
}

func (u Updater) api() string {
	if u.API != "" {
		return u.API
	}
	return "https://api.github.com/repos/" + Repo
}

func (u Updater) client() *http.Client {
	if u.Client != nil {
		return u.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (u Updater) get(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "steamsweep-updater")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := u.client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp.Body, nil
}

// Latest fetches the newest published release.
func (u Updater) Latest(ctx context.Context) (Release, error) {
	body, err := u.get(ctx, u.api()+"/releases/latest")
	if err != nil {
		return Release{}, err
	}
	defer body.Close()
	var raw struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 4<<20)).Decode(&raw); err != nil {
		return Release{}, err
	}
	rel := Release{Tag: raw.Tag, Assets: map[string]string{}}
	for _, a := range raw.Assets {
		rel.Assets[a.Name] = a.URL
	}
	return rel, nil
}

func parse(v string) (out [3]int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Newer reports whether latest is a higher version than current. Unparseable versions
// (such as "dev" from a local build) are never considered outdated.
func Newer(current, latest string) bool {
	c, ok1 := parse(current)
	l, ok2 := parse(latest)
	if !ok1 || !ok2 {
		return false
	}
	for i := range c {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func assetName(goos, goarch string) string {
	if goos == "windows" {
		return fmt.Sprintf("steamsweep_%s_%s.zip", goos, goarch)
	}
	return fmt.Sprintf("steamsweep_%s_%s.tar.gz", goos, goarch)
}

func readAll(rc io.ReadCloser) ([]byte, error) {
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxDownload+1))
	if err == nil && len(data) > maxDownload {
		err = errors.New("download too large")
	}
	return data, err
}

func extract(archive []byte, name, binary string) ([]byte, error) {
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if f.Name == binary {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				return readAll(rc)
			}
		}
		return nil, fmt.Errorf("%s not found in archive", binary)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%s not found in archive", binary)
		}
		if err != nil {
			return nil, err
		}
		if h.Name == binary {
			return readAll(io.NopCloser(tr))
		}
	}
}

// Download fetches the release archive for goos/goarch, verifies it against checksums.txt,
// and returns the binary's bytes.
func (u Updater) Download(ctx context.Context, rel Release, goos, goarch string) ([]byte, error) {
	name := assetName(goos, goarch)
	url, ok := rel.Assets[name]
	if !ok {
		return nil, fmt.Errorf("release %s has no %s", rel.Tag, name)
	}
	sumsURL, ok := rel.Assets["checksums.txt"]
	if !ok {
		return nil, fmt.Errorf("release %s has no checksums.txt; refusing to install unverified", rel.Tag)
	}
	body, err := u.get(ctx, url)
	if err != nil {
		return nil, err
	}
	archive, err := readAll(body)
	if err != nil {
		return nil, err
	}
	body, err = u.get(ctx, sumsURL)
	if err != nil {
		return nil, err
	}
	sums, err := readAll(body)
	if err != nil {
		return nil, err
	}
	var want string
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == name {
			want = f[0]
		}
	}
	got := sha256.Sum256(archive)
	if want == "" || !strings.EqualFold(want, hex.EncodeToString(got[:])) {
		return nil, errors.New("checksum mismatch; aborting update")
	}
	binary := "steamsweep"
	if goos == "windows" {
		binary += ".exe"
	}
	return extract(archive, name, binary)
}

// Replace swaps the file at path for data. The new file is written next to it first so the
// final step is an atomic rename; on Windows the running exe is renamed aside instead.
func Replace(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".steamsweep-new-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s (installed by a package manager? update it there): %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		old := path + ".old"
		_ = os.Remove(old)
		if err := os.Rename(path, old); err != nil {
			return err
		}
		if err := os.Rename(tmpName, path); err != nil {
			_ = os.Rename(old, path)
			return err
		}
		return nil
	}
	return os.Rename(tmpName, path)
}

// Apply downloads rel and replaces the running executable.
func (u Updater) Apply(ctx context.Context, rel Release) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	data, err := u.Download(ctx, rel, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	return Replace(exe, data)
}

// Config is the small persisted state: auto-update preference and the last check.
type Config struct {
	AutoUpdate  bool      `json:"auto_update"`
	LastChecked time.Time `json:"last_checked"`
	LatestSeen  string    `json:"latest_seen"`
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "steamsweep", "config.json"), nil
}

func LoadConfig() Config {
	var c Config
	if p, err := configPath(); err == nil {
		if data, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(data, &c)
		}
	}
	return c
}

func SaveConfig(c Config) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(p, data, 0o644)
}

// CheckCached returns the newest version if it is newer than current. It asks GitHub at most once
// per ttl and otherwise answers from the saved config, so starting the app stays offline most times.
func (u Updater) CheckCached(ctx context.Context, current string, ttl time.Duration) (Release, bool, error) {
	cfg := LoadConfig()
	if time.Since(cfg.LastChecked) < ttl && cfg.LatestSeen != "" {
		return Release{Tag: cfg.LatestSeen}, Newer(current, cfg.LatestSeen), nil
	}
	rel, err := u.Latest(ctx)
	if err != nil {
		return Release{}, false, err
	}
	cfg.LastChecked, cfg.LatestSeen = time.Now(), rel.Tag
	_ = SaveConfig(cfg)
	return rel, Newer(current, rel.Tag), nil
}
