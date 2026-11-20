package cli

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// webServer owns one read-only, loopback-only view of one Garage project.
// It deliberately receives a fixed project directory: requests cannot select
// another project or filesystem path.
type webServer struct {
	projectDir string
	project    string
	slug       string
	capability string
	listener   net.Listener
	env        webEnv
	tmpl       *template.Template
}

type webEnv struct {
	History historyEnv
	Status  statusEnv
	Verify  skillsEnv
	Now     func() time.Time
}

func defaultWebEnv(version string) (webEnv, error) {
	history, err := defaultHistoryEnv()
	if err != nil {
		return webEnv{}, err
	}
	status, err := defaultStatusEnv()
	if err != nil {
		return webEnv{}, err
	}
	verify, err := defaultSkillsEnv(version)
	if err != nil {
		return webEnv{}, err
	}
	return webEnv{History: history, Status: status, Verify: verify, Now: time.Now}, nil
}

func newWebCommand(version string) *cobra.Command {
	var port int
	var openBrowser bool
	cmd := &cobra.Command{
		Use:   "web",
		Short: "Serve a read-only local view of this Garage project",
		Long: `Serve a read-only local interface for the Garage project in the current directory.

The server binds only to 127.0.0.1 and one project. It never accepts a
browser-supplied filesystem path, writes project data, advances workflow, or
starts a background service. The printed URL does not contain the access
capability. Paste the separately printed access code into the local page, or
use --open to open a one-time fragment-based bootstrap URL.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := defaultWebEnv(version)
			if err != nil {
				return err
			}
			projectDir, err := currentDir()
			if err != nil {
				return err
			}
			server, err := newWebServer(projectDir, port, env)
			if err != nil {
				return err
			}
			defer server.listener.Close()
			base := server.url()
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Garage local interface: %s\nAccess code: %s\n", base, server.capability); err != nil {
				return err
			}
			if openBrowser {
				if err := openLocalBrowser(base + "#cap=" + url.QueryEscape(server.capability)); err != nil {
					return err
				}
			}
			return server.serve(cmd.Context())
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "bind this local TCP port (default: an OS-assigned available port)")
	cmd.Flags().BoolVar(&openBrowser, "open", false, "open the local interface with a one-time access bootstrap")
	return cmd
}

func newWebServer(projectDir string, port int, env webEnv) (*webServer, error) {
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("port must be between 0 and 65535")
	}
	projectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve project directory: %w", err)
	}
	if _, err := loadConfigForCommand(projectDir); err != nil {
		return nil, fmt.Errorf("cannot start local interface: %w", err)
	}
	project := filepath.Base(projectDir)
	slug := webSlug(project)
	capability, err := newWebCapability()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("cannot bind 127.0.0.1:%d: %w", port, err)
	}
	tmpl, err := template.New("page").Parse(webPage)
	if err != nil {
		listener.Close()
		return nil, fmt.Errorf("cannot prepare local interface: %w", err)
	}
	return &webServer{projectDir: projectDir, project: project, slug: slug, capability: capability, listener: listener, env: env, tmpl: tmpl}, nil
}

func (s *webServer) url() string { return "http://" + s.listener.Addr().String() + "/" + s.slug + "/" }

func newWebCapability() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("cannot create local access capability: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func webSlug(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "project"
	}
	return result
}

func (s *webServer) serve(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	httpServer := &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	done := make(chan error, 1)
	go func() { done <- httpServer.Serve(s.listener) }()
	select {
	case err := <-done:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		return ctx.Err()
	}
}

func (s *webServer) handler() http.Handler {
	mux := http.NewServeMux()
	prefix := "/" + s.slug + "/"
	mux.HandleFunc(prefix, s.page)
	mux.HandleFunc(prefix+"auth", s.auth)
	mux.HandleFunc(prefix+"activity", s.authorized(s.activity))
	mux.HandleFunc(prefix+"status", s.authorized(s.status))
	return s.guard(mux)
}

func (s *webServer) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != s.listener.Addr().String() {
			http.Error(w, "invalid Host", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.Method == http.MethodPost {
			origin := r.Header.Get("Origin")
			if origin != "" && origin != s.url()[:len(s.url())-1] {
				http.Error(w, "invalid Origin", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *webServer) authorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("garage_web_capability")
		if err != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.capability)) != 1 {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *webServer) auth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid authorization", http.StatusBadRequest)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Form.Get("capability")), []byte(s.capability)) != 1 {
		http.Error(w, "authorization denied", http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "garage_web_capability", Value: s.capability, Path: "/" + s.slug + "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/"+s.slug+"/", http.StatusSeeOther)
}

type webPageData struct {
	Project, Root, Slug, Config, Skills, History string
	StatusNumber                                 int
	Status                                       string
	ActivityError                                string
}

func (s *webServer) page(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/"+s.slug+"/" {
		http.NotFound(w, r)
		return
	}
	cookie, err := r.Cookie("garage_web_capability")
	if err != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.capability)) != 1 {
		io.WriteString(w, webBootstrap(s.slug))
		return
	}
	data := s.snapshot(r.Context(), r.URL.Query().Get("number"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tmpl.Execute(w, data)
}

func (s *webServer) snapshot(ctx context.Context, number string) webPageData {
	data := webPageData{Project: s.project, Root: s.projectDir, Slug: s.slug, Config: s.configSummary(), Skills: s.skillSummary(ctx)}
	if n, err := strconv.Atoi(number); err == nil && n > 0 {
		data.StatusNumber = n
		data.Status = s.statusSummary(ctx, n)
	}
	data.History, data.ActivityError = s.historySummary(ctx)
	return data
}

func (s *webServer) configSummary() string {
	cfg, err := loadConfigForCommand(s.projectDir)
	if err != nil {
		return "Unavailable: " + err.Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Schema version: %d\n", cfg.SchemaVersion)
	if cfg.Project != nil && cfg.Project.IntegrationBranch != nil {
		fmt.Fprintf(&b, "Integration branch: %s\n", *cfg.Project.IntegrationBranch)
	}
	if cfg.Project != nil && cfg.Project.Guidance != nil {
		for _, m := range cfg.Project.Guidance.Manuals {
			fmt.Fprintf(&b, "Manual: %s %s\n", m.Name, m.Version)
		}
	}
	if cfg.Tooling != nil && cfg.Tooling.SkillTargets != nil {
		for _, t := range *cfg.Tooling.SkillTargets {
			fmt.Fprintf(&b, "Skill target: %s (%s)\n", t.Harness, t.Scope)
		}
	}
	return strings.TrimSpace(b.String())
}

func (s *webServer) skillSummary(ctx context.Context) string {
	env := s.env.Verify
	env.WorkDir = s.projectDir
	var out strings.Builder
	verification, err := inspectSkills(ctx, env, nil)
	if err != nil {
		return "Verification unavailable: " + err.Error()
	}
	fmt.Fprintf(&out, "Embedded skills: revision %s\n", verification.Revision)
	for _, target := range verification.Targets {
		fmt.Fprintf(&out, "%s  %s\n", target.Target.Config.Harness, target.Target.Root)
		for _, skill := range target.Skills {
			fmt.Fprintf(&out, "  %s: %s\n", skill.Name, skill.State)
		}
		for _, manual := range target.Manuals {
			fmt.Fprintf(&out, "  %s: %s\n", manual.Name, manual.State)
		}
		for _, collision := range target.Collisions {
			fmt.Fprintf(&out, "  collision: %s\n", collision.describe(target.Target.Config.Harness))
		}
		for _, coverage := range target.Coverage {
			fmt.Fprintf(&out, "  %s\n", coverage)
		}
	}
	if verification.Problems > 0 {
		fmt.Fprintf(&out, "Verification found %d problem(s).\n", verification.Problems)
	}
	return strings.TrimSpace(out.String())
}

func (s *webServer) historySummary(ctx context.Context) (string, string) {
	env := s.env.History
	env.WorkDir = s.projectDir
	repo, err := identifyRepository(ctx, env.WorkDir, env.Git)
	if err != nil {
		return "", err.Error()
	}
	cacheDir, err := historyCacheDir(env)
	if err != nil {
		return "", err.Error()
	}
	events, discarded, err := readHistory(historyPath(cacheDir, repo.Key))
	if err != nil {
		return "", err.Error()
	}
	var b strings.Builder
	for _, event := range events {
		fmt.Fprintf(&b, "%s  %s #%d  %s  %s", event.Time, event.Kind, event.Number, event.Type, event.Outcome)
		if event.URL != "" {
			fmt.Fprintf(&b, "  %s", event.URL)
		}
		b.WriteByte('\n')
	}
	if discarded > 0 {
		return strings.TrimSpace(b.String()), fmt.Sprintf("%d unreadable history entr%s skipped", discarded, map[bool]string{true: "y", false: "ies"}[discarded == 1])
	}
	return strings.TrimSpace(b.String()), ""
}

func (s *webServer) statusSummary(ctx context.Context, number int) string {
	env := s.env.Status
	env.WorkDir = s.projectDir
	report, err := statusReportFor(ctx, number, env)
	if report == nil {
		return "Status unavailable: " + err.Error()
	}
	var b strings.Builder
	for _, line := range report.header {
		if line != "" {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	for _, section := range report.sections {
		fmt.Fprintf(&b, "\n%s\n", section.heading)
		for _, line := range section.lines {
			fmt.Fprintf(&b, "[%s] %s\n", line.label, line.text)
		}
	}
	if err != nil {
		fmt.Fprintf(&b, "\nStatus unavailable: %v", err)
	}
	return strings.TrimSpace(b.String())
}

func (s *webServer) activity(w http.ResponseWriter, r *http.Request) {
	text, problem := s.historySummary(r.Context())
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if problem != "" {
		http.Error(w, problem, http.StatusServiceUnavailable)
		return
	}
	io.WriteString(w, text)
}
func (s *webServer) status(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.URL.Query().Get("number"))
	if err != nil || n < 1 {
		http.Error(w, "number must be a positive issue or pull request number", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, s.statusSummary(r.Context(), n))
}

func openLocalBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot open local browser: %w", err)
	}
	return nil
}

const webBootstrapPage = `<!doctype html><title>Garage local access</title><h1>Garage local access</h1><form method="post" action="auth"><label>Access code <input name="capability" autocomplete="off"></label><button>Open project</button></form><script>const c=new URL(location).hash.slice(1);if(c.startsWith('cap=')){document.querySelector('input').value=decodeURIComponent(c.slice(4));history.replaceState(null,'',location.pathname);document.querySelector('form').requestSubmit()}</script>`

func webBootstrap(slug string) string {
	return strings.Replace(webBootstrapPage, "action=\"auth\"", "action=\"/"+slug+"/auth\"", 1)
}

const webPage = `<!doctype html><html><head><meta charset="utf-8"><title>{{.Project}} — Garage</title><style>body{font:16px system-ui;margin:2rem;max-width:70rem}pre{white-space:pre-wrap;background:#f4f4f4;padding:1rem}section{margin:2rem 0}</style></head><body><h1>{{.Project}}</h1><p><strong>Local project root:</strong> {{.Root}}</p><p>This interface is read-only. GitHub records are workflow authority; local activity is a private annotation and may be incomplete.</p><section><h2>Project overview</h2><h3>Adopted configuration</h3><pre>{{.Config}}</pre><h3>Installed skills and manuals</h3><pre>{{.Skills}}</pre></section><section><h2>Workflow status</h2><form method="get"><label>Issue or PR number <input name="number" value="{{.StatusNumber}}"></label><button>Check status</button></form>{{if .Status}}<pre>{{.Status}}</pre>{{end}}</section><section><h2>Recorded Garage activity</h2><p>Chronological recorded Garage events, not a transcript or live agent execution view. Refreshes every 15 seconds while open.</p>{{if .ActivityError}}<p>{{.ActivityError}}</p>{{else}}<pre id="activity">{{.History}}</pre>{{end}}</section><script>setInterval(async()=>{const r=await fetch('activity');if(r.ok)document.getElementById('activity').textContent=await r.text()},15000)</script></body></html>`
