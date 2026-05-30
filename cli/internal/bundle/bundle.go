// Package bundle handles fetching and extracting the iyzitrace asset bundle.
//
// A bundle is a gzipped tar archive containing:
//   - BUNDLE_VERSION         (single line, semver)
//   - iyzitrace.yaml.default (default config shipped with this bundle)
//   - templates/             (text/template sources rendered at apply time)
//   - compose/               (docker-compose template fragments)
//
// Source URLs may be file:// (for fixtures and offline installs) or https://
// (GitHub releases by default). A sibling <url>.sha256 file is fetched and
// the tarball is verified before extraction.
package bundle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultGitHubReleaseURLTemplate = "https://github.com/iyzitrace/iyzitrace-observability-platform/releases/download/bundle-v%s/iyzitrace-bundle-%s.tar.gz"

// Fetch downloads the bundle from src into a temp file, verifies its sha256
// against <src>.sha256 (skipped if the sibling file is missing AND
// requireChecksum is false), and returns the local path. Caller deletes it.
func Fetch(src string, requireChecksum bool) (string, error) {
	body, err := openURL(src)
	if err != nil {
		return "", fmt.Errorf("fetch bundle: %w", err)
	}
	defer body.Close()

	tmp, err := os.CreateTemp("", "iyzitrace-bundle-*.tar.gz")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	h := sha256.New()
	mw := io.MultiWriter(tmp, h)
	if _, err := io.Copy(mw, body); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	gotSum := hex.EncodeToString(h.Sum(nil))

	wantSum, err := fetchChecksum(src + ".sha256")
	switch {
	case err == nil:
		if !strings.EqualFold(wantSum, gotSum) {
			os.Remove(tmpPath)
			return "", fmt.Errorf("checksum mismatch: want %s, got %s", wantSum, gotSum)
		}
	case requireChecksum:
		os.Remove(tmpPath)
		return "", fmt.Errorf("checksum required but %s.sha256 unavailable: %w", src, err)
	}
	return tmpPath, nil
}

// Extract un-tars a gzipped bundle into dst, replacing whatever is there.
// Returns the bundle version string from BUNDLE_VERSION.
func Extract(tarballPath, dst string) (string, error) {
	f, err := os.Open(tarballPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		// Defend against path traversal (CWE-22).
		clean := filepath.Clean(hdr.Name)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return "", fmt.Errorf("unsafe path in tarball: %q", hdr.Name)
		}
		target := filepath.Join(dst, clean)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o755|0o600)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return "", err
			}
			if err := out.Close(); err != nil {
				return "", err
			}
		default:
			// symlinks, char devices, etc. — skip silently.
		}
	}

	verPath := filepath.Join(dst, "BUNDLE_VERSION")
	v, err := os.ReadFile(verPath)
	if err != nil {
		return "", fmt.Errorf("bundle missing BUNDLE_VERSION: %w", err)
	}
	return strings.TrimSpace(string(v)), nil
}

// FetchAndExtract is the one-shot helper used by `init` and `upgrade`.
func FetchAndExtract(src, dst string, requireChecksum bool) (version string, err error) {
	tarball, err := Fetch(src, requireChecksum)
	if err != nil {
		return "", err
	}
	defer os.Remove(tarball)
	return Extract(tarball, dst)
}

func openURL(src string) (io.ReadCloser, error) {
	u, err := url.Parse(src)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "file", "":
		return os.Open(u.Path)
	case "http", "https":
		// Default Go-http-client UA is rejected by some CDNs (notably AWS S3
		// signed URLs that GitHub redirects to). Set a real-looking UA and an
		// explicit Accept header.
		req, err := http.NewRequest(http.MethodGet, src, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "iyzitrace-cli")
		req.Header.Set("Accept", "application/octet-stream, */*")
		client := &http.Client{Timeout: 5 * time.Minute}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			return nil, fmt.Errorf("GET %s: %s (body: %s)", src, resp.Status, strings.TrimSpace(string(body)))
		}
		return resp.Body, nil
	default:
		return nil, fmt.Errorf("unsupported url scheme: %s", u.Scheme)
	}
}

func fetchChecksum(src string) (string, error) {
	body, err := openURL(src)
	if err != nil {
		return "", err
	}
	defer body.Close()
	b, err := io.ReadAll(io.LimitReader(body, 256))
	if err != nil {
		return "", err
	}
	// Accept both "abc..." and "abc...  filename" formats.
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return "", errors.New("empty checksum file")
	}
	return fields[0], nil
}
