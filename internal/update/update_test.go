package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		cur, latest string
		want        bool
	}{
		{"0.1.0", "v0.2.0", true},
		{"v0.2.0", "0.2.0", false},
		{"0.2.0", "0.1.9", false},
		{"0.9.0", "0.10.0", true},
		{"dev", "9.9.9", false},
		{"0.1.0", "garbage", false},
		{"0.1.0-rc1", "0.1.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.cur, c.latest); got != c.want {
			t.Errorf("Newer(%q,%q)=%v want %v", c.cur, c.latest, got, c.want)
		}
	}
}

func tarball(t *testing.T, name string, body []byte) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))})
	tw.Write(body)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func server(t *testing.T, archive []byte, sum string) *httptest.Server {
	var srv *httptest.Server
	mux := http.NewServeMux()
	asset := assetName("linux", "amd64")
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v0.2.0","assets":[{"name":%q,"browser_download_url":"%s/dl/a"},{"name":"checksums.txt","browser_download_url":"%s/dl/sums"}]}`,
			asset, srv.URL, srv.URL)
	})
	mux.HandleFunc("/dl/a", func(w http.ResponseWriter, r *http.Request) { w.Write(archive) })
	mux.HandleFunc("/dl/sums", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, "%s  %s\n", sum, asset) })
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func hexSum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func TestDownloadVerifiesAndExtracts(t *testing.T) {
	archive := tarball(t, "steamsweep", []byte("NEWBINARY"))
	srv := server(t, archive, hexSum(archive))
	u := Updater{API: srv.URL}
	rel, err := u.Latest(context.Background())
	if err != nil || rel.Tag != "v0.2.0" {
		t.Fatalf("latest: %v %+v", err, rel)
	}
	data, err := u.Download(context.Background(), rel, "linux", "amd64")
	if err != nil || string(data) != "NEWBINARY" {
		t.Fatalf("download: %v %q", err, data)
	}
}

func TestDownloadRejectsBadChecksum(t *testing.T) {
	archive := tarball(t, "steamsweep", []byte("EVIL"))
	srv := server(t, archive, hexSum([]byte("something else")))
	u := Updater{API: srv.URL}
	rel, _ := u.Latest(context.Background())
	if _, err := u.Download(context.Background(), rel, "linux", "amd64"); err == nil {
		t.Fatal("accepted a checksum mismatch")
	}
}

func TestReplace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows swap is exercised on CI runners only")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "steamsweep")
	os.WriteFile(exe, []byte("old"), 0o755)
	if err := Replace(exe, []byte("new")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "new" {
		t.Fatalf("got %q", got)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".steamsweep-new-*")); len(left) != 0 {
		t.Errorf("temp file left behind: %v", left)
	}
	if fi, _ := os.Stat(exe); fi.Mode()&0o111 == 0 {
		t.Error("not executable")
	}
}

func TestCheckCachedUsesCache(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, `{"tag_name":"v0.3.0","assets":[]}`)
	}))
	defer srv.Close()
	u := Updater{API: srv.URL}
	for i := 0; i < 3; i++ {
		rel, newer, err := u.CheckCached(context.Background(), "0.1.0", 24*3600*1e9)
		if err != nil || !newer || rel.Tag != "v0.3.0" {
			t.Fatalf("run %d: %v %v %+v", i, err, newer, rel)
		}
	}
	if hits != 1 {
		t.Errorf("hit the network %d times, want 1", hits)
	}
}
