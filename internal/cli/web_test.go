package cli

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestWebDashboardUsesStructuredPanelsAndNewTabNavigation(t *testing.T) {
	for _, want := range []string{
		"grid-template-columns:minmax(0,1fr) minmax(0,2fr)",
		"overflow-y:auto",
		"Skills &amp; manuals",
		"Workflow status",
		"<dialog id=\"details\"",
		"a.target='_blank'",
		"a.rel='noopener noreferrer'",
	} {
		if !strings.Contains(webPage, want) {
			t.Errorf("web dashboard is missing %q", want)
		}
	}
	for _, want := range []string{"event-subject", "role-icon", "e.role||'Agent not recorded'"} {
		if !strings.Contains(webPageMarkup(), want) {
			t.Errorf("web activity attribution presentation is missing %q", want)
		}
	}
}

func TestWebAuthOriginGuard(t *testing.T) {
	s, err := newWebServer(webProject(t), 0, webEnv{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.listener.Close()

	for _, tc := range []struct {
		name   string
		origin string
		want   int
	}{
		{name: "listener origin", origin: "http://" + s.listener.Addr().String(), want: http.StatusSeeOther},
		{name: "embedded loopback origin", origin: "http://127.0.0.1", want: http.StatusSeeOther},
		{name: "other loopback port", origin: "http://127.0.0.1:1", want: http.StatusForbidden},
		{name: "other host", origin: "http://localhost", want: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, s.url()+"auth", strings.NewReader("capability="+s.capability))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", tc.origin)
			req.Host = s.listener.Addr().String()
			rr := httptest.NewRecorder()
			s.handler().ServeHTTP(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d", rr.Code, tc.want)
			}
		})
	}
}

func TestWebActivityReturnsNewestStructuredEventsAndFilters(t *testing.T) {
	project := webProject(t)
	cacheDir := t.TempDir()
	git := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "rev-parse" {
			return []byte(project + "\n" + filepath.Join(project, ".git") + "\n"), nil
		}
		return []byte("https://github.com/example/activity.git\n"), nil
	}
	repo, err := identifyRepository(context.Background(), project, git)
	if err != nil {
		t.Fatal(err)
	}
	encode := func(time, kind string, number int, typ, outcome, rawURL, role string) string {
		t.Helper()
		line, err := json.Marshal(historyEvent{Version: historySchemaVersion, Time: time, Repo: repo.Key, Kind: kind, Number: number, Type: typ, Outcome: outcome, URL: rawURL, Role: role})
		if err != nil {
			t.Fatal(err)
		}
		return string(line)
	}
	writeHistoryFile(t, filepath.Join(cacheDir, historyAppDir, historySubDir), repo.Key,
		encode("2026-11-20T10:00:00Z", "issue", 12, "discovery", "complete", "https://github.com/example/activity/issues/12", ""),
		encode("2026-11-20T11:00:00Z", "pr", 14, "review", "approved", "https://github.com/example/activity/pull/14", "Agent #2"),
		encode("2026-11-20T12:00:00Z", "issue", 12, "implementation", "ready", "https://github.com/example/activity/issues/12#issuecomment-1", "Agent #1"),
	)
	s, err := newWebServer(project, 0, webEnv{History: historyEnv{WorkDir: project, CacheDir: func() (string, error) { return cacheDir, nil }, Git: git}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.listener.Close()
	request := func(rawURL string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, rawURL, nil)
		req.Host = s.listener.Addr().String()
		req.AddCookie(&http.Cookie{Name: "garage_web_capability", Value: s.capability})
		rr := httptest.NewRecorder()
		s.handler().ServeHTTP(rr, req)
		return rr
	}

	rr := request(s.url() + "activity")
	if rr.Code != http.StatusOK {
		t.Fatalf("activity = %d: %s", rr.Code, rr.Body.String())
	}
	var all webActivity
	if err := json.NewDecoder(rr.Body).Decode(&all); err != nil {
		t.Fatal(err)
	}
	if len(all.Events) != 3 || all.Events[0].Time != "2026-11-20T12:00:00Z" || all.Events[0].Type != "implementation" || all.Events[0].Role != "Agent #1" || all.Events[1].Role != "Agent #2" || all.Events[2].Role != "" {
		t.Fatalf("events = %#v, want newest-first structured events", all.Events)
	}

	rr = request(s.url() + "activity?number=12&type=implementation&outcome=ready")
	if rr.Code != http.StatusOK {
		t.Fatalf("filtered activity = %d: %s", rr.Code, rr.Body.String())
	}
	var filtered webActivity
	if err := json.NewDecoder(rr.Body).Decode(&filtered); err != nil {
		t.Fatal(err)
	}
	if len(filtered.Events) != 1 || filtered.Events[0].Number != 12 || filtered.Events[0].URL == "" {
		t.Fatalf("filtered events = %#v, want one linked implementation", filtered.Events)
	}
}

func TestWebActivityRejectsInvalidFilters(t *testing.T) {
	s, err := newWebServer(webProject(t), 0, webEnv{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.listener.Close()
	req := httptest.NewRequest(http.MethodGet, s.url()+"activity?type=agent-chat", nil)
	req.Host = s.listener.Addr().String()
	req.AddCookie(&http.Cookie{Name: "garage_web_capability", Value: s.capability})
	rr := httptest.NewRecorder()
	s.handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid activity filter = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestWebActivityPagesOlderEventsWithoutReturningFullHistory(t *testing.T) {
	project := webProject(t)
	cacheDir := t.TempDir()
	git := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "rev-parse" {
			return []byte(project + "\n" + filepath.Join(project, ".git") + "\n"), nil
		}
		return []byte("https://github.com/example/activity.git\n"), nil
	}
	repo, err := identifyRepository(context.Background(), project, git)
	if err != nil {
		t.Fatal(err)
	}
	events := make([]historyEvent, 0, webActivityPageSize+1)
	for i := 1; i <= webActivityPageSize+1; i++ {
		events = append(events, historyEvent{Version: historySchemaVersion, Time: fmt.Sprintf("2026-11-20T12:00:%02dZ", i), Repo: repo.Key, Kind: "issue", Number: i, Type: "review", Outcome: "approved"})
	}
	encoded, err := encodeHistory(events)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, historyPath(cacheDir, repo.Key), string(encoded))
	s := webServer{projectDir: project, env: webEnv{History: historyEnv{WorkDir: project, CacheDir: func() (string, error) { return cacheDir, nil }, Git: git}}}
	first, err := s.activitySnapshot(context.Background(), webActivityFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != webActivityPageSize || first.NextOffset != webActivityPageSize || first.Events[0].Number != webActivityPageSize+1 {
		t.Fatalf("first page = %#v, want %d newest events and offset %d", first, webActivityPageSize, webActivityPageSize)
	}
	older, err := s.activitySnapshot(context.Background(), webActivityFilter{Offset: first.NextOffset})
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Events) != 1 || older.NextOffset != -1 || older.Events[0].Number != 1 {
		t.Fatalf("older page = %#v, want only the oldest event", older)
	}
}

func TestWebPagePreservesActivityFocusAndReadingPositionOnRedraw(t *testing.T) {
	for _, want := range []string{
		"row.dataset.eventKey=key(e)",
		"top:list.scrollTop",
		"list.scrollTop=p.top",
		"target?.focus({preventScroll:true})",
		"a.target='_blank'",
		"a.rel='noopener noreferrer'",
	} {
		if !strings.Contains(webPage, want) {
			t.Errorf("web page does not preserve activity focus/position: missing %q", want)
		}
	}
}
