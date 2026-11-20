package cli

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func webProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, garageDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPathIn(dir), []byte(validConfigYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestWebAcceptsBrowserOriginWithoutProjectSlug(t *testing.T) {
	s, err := newWebServer(webProject(t), 0, webEnv{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.listener.Close()
	form := url.Values{"capability": {s.capability}}.Encode()
	req := httptest.NewRequest(http.MethodPost, s.url()+"auth", strings.NewReader(form))
	req.Host = s.listener.Addr().String()
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://"+s.listener.Addr().String())
	rr := httptest.NewRecorder()
	s.handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("authorization with browser Origin = %d, want %d: %s", rr.Code, http.StatusSeeOther, rr.Body.String())
	}
}

func TestWebSlug(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"Builder Garage", "builder-garage"}, {"___", "project"}, {"One--Two", "one-two"}} {
		if got := webSlug(tc.in); got != tc.want {
			t.Errorf("webSlug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWebRequiresCapabilityForProjectData(t *testing.T) {
	s, err := newWebServer(webProject(t), 0, webEnv{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.listener.Close()
	req := httptest.NewRequest(http.MethodGet, s.url()+"activity", nil)
	req.Host = s.listener.Addr().String()
	rr := httptest.NewRecorder()
	s.handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("activity without capability = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestWebRejectsOtherSlug(t *testing.T) {
	s, err := newWebServer(webProject(t), 0, webEnv{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.listener.Close()
	req := httptest.NewRequest(http.MethodGet, "http://"+s.listener.Addr().String()+"/other/", nil)
	req.Host = s.listener.Addr().String()
	rr := httptest.NewRecorder()
	s.handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("other slug = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestWebBootstrapDoesNotExposeCapability(t *testing.T) {
	s, err := newWebServer(webProject(t), 0, webEnv{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.listener.Close()
	req := httptest.NewRequest(http.MethodGet, s.url(), nil)
	req.Host = s.listener.Addr().String()
	rr := httptest.NewRecorder()
	s.handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("bootstrap = %d", rr.Code)
	}
	if strings.Contains(rr.Body.String(), s.capability) {
		t.Fatal("bootstrap exposed capability")
	}
}
