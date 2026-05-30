package bundle_test

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/iyzitrace/iyzitrace-observability-platform/cli/internal/bundle"
)

func TestFetchExtract(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "bundle.tar.gz")
	writeBundle(t, tarPath, map[string]string{
		"BUNDLE_VERSION":           "0.1.0\n",
		"iyzitrace.yaml.default":   "version: 1\n",
		"templates/prometheus.tmpl": "scrape_interval: {{ .Services.Prometheus.ScrapeInterval }}\n",
	})

	// write checksum sibling
	sum := sha256File(t, tarPath)
	if err := os.WriteFile(tarPath+".sha256", []byte(sum+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := "file://" + tarPath
	dst := filepath.Join(dir, "extracted")
	v, err := bundle.FetchAndExtract(src, dst, true)
	if err != nil {
		t.Fatal(err)
	}
	if v != "0.1.0" {
		t.Errorf("version: %q", v)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "templates/prometheus.tmpl")); err != nil {
		t.Fatal(err)
	} else if got := string(b); got == "" {
		t.Errorf("template empty: %q", got)
	}
}

func TestFetchExtract_BadChecksum(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "bundle.tar.gz")
	writeBundle(t, tarPath, map[string]string{"BUNDLE_VERSION": "0.1.0"})
	if err := os.WriteFile(tarPath+".sha256", []byte("0000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := bundle.FetchAndExtract("file://"+tarPath, filepath.Join(dir, "x"), true); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestExtract_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "evil.tar.gz")
	writeBundleRaw(t, tarPath, []tar.Header{
		{Name: "../escape.txt", Mode: 0o644, Size: 4, Typeflag: tar.TypeReg},
	}, [][]byte{[]byte("oops")})
	if _, err := bundle.Extract(tarPath, filepath.Join(dir, "out")); err == nil {
		t.Fatal("expected traversal rejection")
	}
}

func writeBundle(t *testing.T, path string, files map[string]string) {
	t.Helper()
	headers := make([]tar.Header, 0, len(files))
	bodies := make([][]byte, 0, len(files))
	for name, body := range files {
		headers = append(headers, tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		bodies = append(bodies, []byte(body))
	}
	writeBundleRaw(t, path, headers, bodies)
}

func writeBundleRaw(t *testing.T, path string, headers []tar.Header, bodies [][]byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for i, h := range headers {
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(bodies[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
