package cli

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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
	tmpl, err := template.New("page").Parse(webPageMarkup())
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
			if !s.acceptsOrigin(origin) {
				http.Error(w, "invalid Origin", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// acceptsOrigin permits the listener's exact origin and the no-port loopback
// origin sent by the embedded local browser surface. The Host check above still
// requires that the request be addressed to this specific server instance.
func (s *webServer) acceptsOrigin(origin string) bool {
	return origin == "" || origin == "http://"+s.listener.Addr().String() || origin == "http://127.0.0.1"
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

type webProjectSummary struct {
	Schema            int
	IntegrationBranch string
	Manuals           []string
	Harnesses         []string
}

type webSkillTargetSummary struct {
	Harness string
	State   string
}

type webSkillSummary struct {
	Current, Stale, Missing, Collisions, Other int
	Targets                                    []webSkillTargetSummary
	Details                                    []string
	Problem                                    string
}

type webStatusLine struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

type webWorkflowStatus struct {
	Number      int             `json:"number"`
	Kind        string          `json:"kind"`
	Title       string          `json:"title"`
	URL         string          `json:"url,omitempty"`
	Facts       []webStatusLine `json:"facts"`
	Suggestions []webStatusLine `json:"suggestions"`
	Details     []webStatusLine `json:"details"`
	Problem     string          `json:"problem,omitempty"`
}

type webPageData struct {
	Project, Root, Slug string
	Config              webProjectSummary
	Skills              webSkillSummary
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
	_ = number
	return data
}

func (s *webServer) configSummary() webProjectSummary {
	cfg, err := loadConfigForCommand(s.projectDir)
	if err != nil {
		return webProjectSummary{}
	}
	result := webProjectSummary{Schema: cfg.SchemaVersion}
	if cfg.Project != nil && cfg.Project.IntegrationBranch != nil {
		result.IntegrationBranch = *cfg.Project.IntegrationBranch
	}
	if cfg.Project != nil && cfg.Project.Guidance != nil {
		for _, m := range cfg.Project.Guidance.Manuals {
			result.Manuals = append(result.Manuals, m.Name+" "+m.Version)
		}
	}
	if cfg.Tooling != nil && cfg.Tooling.SkillTargets != nil {
		for _, t := range *cfg.Tooling.SkillTargets {
			result.Harnesses = append(result.Harnesses, t.Harness)
		}
	}
	if result.IntegrationBranch == "" {
		result.IntegrationBranch = "Not declared"
	}
	if len(result.Manuals) == 0 {
		result.Manuals = []string{"None declared"}
	}
	if len(result.Harnesses) == 0 {
		result.Harnesses = []string{"None configured"}
	}
	return result
}

func (s *webServer) skillSummary(ctx context.Context) webSkillSummary {
	env := s.env.Verify
	env.WorkDir = s.projectDir
	verification, err := inspectSkills(ctx, env, nil)
	if err != nil {
		return webSkillSummary{Problem: "Verification unavailable: " + err.Error()}
	}
	result := webSkillSummary{Details: []string{"Embedded skills revision: " + verification.Revision}}
	for _, target := range verification.Targets {
		worst := "Current"
		result.Details = append(result.Details, target.Target.Config.Harness+" — "+target.Target.Root)
		for _, skill := range target.Skills {
			state := skill.State.String()
			result.Details = append(result.Details, "  "+skill.Name+": "+state)
			classifyWebSkillState(&result, state, &worst)
		}
		for _, manual := range target.Manuals {
			state := manual.State.String()
			result.Details = append(result.Details, "  "+manual.Name+": "+describeManualState(manual, verification.Repository))
			classifyWebSkillState(&result, state, &worst)
		}
		for _, collision := range target.Collisions {
			result.Collisions++
			worst = "Collisions"
			result.Details = append(result.Details, "  collision: "+collision.describe(target.Target.Config.Harness))
		}
		result.Other += len(target.Extras)
		for _, coverage := range target.Coverage {
			result.Other++
			if worst == "Current" {
				worst = "Other"
			}
			result.Details = append(result.Details, "  "+coverage)
		}
		result.Targets = append(result.Targets, webSkillTargetSummary{Harness: target.Target.Config.Harness, State: worst})
	}
	if len(result.Targets) == 0 {
		result.Problem = "No configured skill targets were available for verification."
	}
	return result
}

func classifyWebSkillState(summary *webSkillSummary, state string, worst *string) {
	switch state {
	case "Current":
		summary.Current++
	case "Stale":
		summary.Stale++
		if *worst == "Current" {
			*worst = "Stale"
		}
	case "Missing":
		summary.Missing++
		if *worst == "Current" || *worst == "Stale" {
			*worst = "Missing"
		}
	default:
		summary.Other++
		if *worst == "Current" {
			*worst = "Other"
		}
	}
}

const webActivityPageSize = 25

// webActivity is the browser read model for persisted local history. It
// deliberately contains only fields the local history already stores.
type webActivity struct {
	Events     []webActivityEvent `json:"events"`
	NextOffset int                `json:"nextOffset"`
	Discarded  int                `json:"discarded"`
}

type webActivityEvent struct {
	Time    string `json:"time"`
	Kind    string `json:"kind"`
	Number  int    `json:"number"`
	Type    string `json:"type"`
	Outcome string `json:"outcome"`
	URL     string `json:"url,omitempty"`
	Role    string `json:"role,omitempty"`
}

type webActivityFilter struct {
	Number  int
	Type    string
	Outcome string
	Offset  int
}

func (s *webServer) activitySnapshot(ctx context.Context, filter webActivityFilter) (webActivity, error) {
	env := s.env.History
	env.WorkDir = s.projectDir
	repo, err := identifyRepository(ctx, env.WorkDir, env.Git)
	if err != nil {
		return webActivity{}, err
	}
	cacheDir, err := historyCacheDir(env)
	if err != nil {
		return webActivity{}, err
	}
	events, discarded, err := readHistory(historyPath(cacheDir, repo.Key))
	if err != nil {
		return webActivity{}, err
	}
	matching := make([]historyEvent, 0, len(events))
	for _, event := range events {
		if filter.Number != 0 && event.Number != filter.Number {
			continue
		}
		if filter.Type != "" && event.Type != filter.Type {
			continue
		}
		if filter.Outcome != "" && event.Outcome != filter.Outcome {
			continue
		}
		matching = append(matching, event)
	}
	sort.SliceStable(matching, func(i, j int) bool { return matching[i].Time > matching[j].Time })
	if filter.Offset >= len(matching) {
		return webActivity{Events: []webActivityEvent{}, NextOffset: -1, Discarded: discarded}, nil
	}
	end := filter.Offset + webActivityPageSize
	if end > len(matching) {
		end = len(matching)
	}
	result := webActivity{Events: make([]webActivityEvent, 0, end-filter.Offset), NextOffset: -1, Discarded: discarded}
	for _, event := range matching[filter.Offset:end] {
		result.Events = append(result.Events, webActivityEvent{Time: event.Time, Kind: event.Kind, Number: event.Number, Type: event.Type, Outcome: event.Outcome, URL: event.URL, Role: event.Role})
	}
	if end < len(matching) {
		result.NextOffset = end
	}
	return result, nil
}

func (s *webServer) statusSummary(ctx context.Context, number int) webWorkflowStatus {
	env := s.env.Status
	env.WorkDir = s.projectDir
	report, err := statusReportFor(ctx, number, env)
	result := webWorkflowStatus{Number: number}
	if report == nil {
		if err != nil {
			result.Problem = "Status unavailable: " + err.Error()
		} else {
			result.Problem = "Status unavailable: no recorded status was returned."
		}
		return result
	}
	for i, line := range report.header {
		if i == 0 {
			result.Title = line
			result.Kind = "Recorded status"
		} else if strings.HasPrefix(line, "https://") {
			result.URL = line
		}
	}
	for _, section := range report.sections {
		for _, line := range section.lines {
			item := webStatusLine{Label: line.label, Text: line.text}
			result.Details = append(result.Details, item)
			if strings.Contains(strings.ToLower(section.heading), "suggest") {
				result.Suggestions = append(result.Suggestions, item)
			} else if len(result.Facts) < 2 {
				result.Facts = append(result.Facts, item)
			}
		}
	}
	if err != nil {
		result.Problem = err.Error()
	}
	return result
}

func (s *webServer) activity(w http.ResponseWriter, r *http.Request) {
	filter, err := parseWebActivityFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	activity, err := s.activitySnapshot(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(activity); err != nil {
		return
	}
}

func parseWebActivityFilter(r *http.Request) (webActivityFilter, error) {
	filter := webActivityFilter{Offset: 0}
	if raw := r.URL.Query().Get("number"); raw != "" {
		number, err := strconv.Atoi(raw)
		if err != nil || number < 1 || number > maxTargetNumber {
			return webActivityFilter{}, errors.New("number must be a positive issue or pull request number")
		}
		filter.Number = number
	}
	if raw := r.URL.Query().Get("type"); raw != "" {
		if !contains(historyEventTypes, raw) {
			return webActivityFilter{}, fmt.Errorf("unknown activity type %q", raw)
		}
		filter.Type = raw
	}
	if raw := r.URL.Query().Get("outcome"); raw != "" {
		if !contains(historyOutcomes, raw) {
			return webActivityFilter{}, fmt.Errorf("unknown activity outcome %q", raw)
		}
		filter.Outcome = raw
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 || offset >= historyEventLimit {
			return webActivityFilter{}, fmt.Errorf("offset must be between 0 and %d", historyEventLimit-1)
		}
		filter.Offset = offset
	}
	return filter, nil
}
func (s *webServer) status(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.URL.Query().Get("number"))
	if err != nil || n < 1 {
		http.Error(w, "number must be a positive issue or pull request number", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(s.statusSummary(r.Context(), n))
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

// webPageMarkup adds the activity-role presentation without changing the
// stored template's data or its browser navigation behavior. Roles arrive
// only from the persisted history event; a missing role stays missing until
// the browser renders the explicit fallback.
func webPageMarkup() string {
	page := strings.Replace(webPage, "</style>", `.event-subject{min-width:0}.role{display:flex;align-items:center;gap:4px;margin-top:3px;color:#465978;font-size:12px;white-space:nowrap}.role-icon{width:13px;height:13px;flex:0 0 13px;fill:none;stroke:currentColor;stroke-width:1.7}</style>`, 1)
	return strings.Replace(page, `row.append(target);const stage`, `const subject=document.createElement('div');subject.className='event-subject';subject.append(target);const role=document.createElement('span');role.className='role';const icon=document.createElementNS('http://www.w3.org/2000/svg','svg');icon.classList.add('role-icon');icon.setAttribute('aria-hidden','true');icon.setAttribute('viewBox','0 0 16 16');const head=document.createElementNS('http://www.w3.org/2000/svg','circle');head.setAttribute('cx','8');head.setAttribute('cy','5');head.setAttribute('r','2.5');const body=document.createElementNS('http://www.w3.org/2000/svg','path');body.setAttribute('d','M3 14c.5-3 2.2-4.5 5-4.5s4.5 1.5 5 4.5');icon.append(head,body);role.append(icon,document.createTextNode(e.role||'Agent not recorded'));subject.append(role);row.append(subject);const stage`, 1)
}

const webPage = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Project}} — Builder Garage</title><style>
:root{font:15px/1.35 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#132b4d;background:#eef3f9}*{box-sizing:border-box}html,body{margin:0;height:100%}button,input,select{font:inherit}button{cursor:pointer}.shell{height:100dvh;min-height:600px;display:grid;grid-template-rows:56px 82px minmax(0,1fr) 28px;overflow:hidden}.top{background:#132b4d;color:white;display:flex;align-items:center;justify-content:space-between;padding:0 24px;font-weight:700}.identity,.dashboard,.foot{padding-left:22px;padding-right:22px}.identity{display:flex;align-items:center;justify-content:space-between;min-width:0}.identity h1{font-size:30px;line-height:1;margin:0}.root{margin:5px 0 0;overflow-wrap:anywhere;color:#465978}.updated{white-space:nowrap;color:#465978}.dashboard{min-height:0;display:grid;grid-template-columns:minmax(0,1fr) minmax(0,2fr);gap:16px;padding-top:12px;padding-bottom:8px}.left{min-height:0;display:grid;grid-template-rows:minmax(0,.92fr) minmax(0,1.12fr) minmax(0,.96fr);gap:16px}.card{min-height:0;background:#fff;border:1px solid #cbd7e8;border-radius:12px;padding:16px;box-shadow:0 1px 2px #132b4d0d;overflow:hidden}.card h2{font-size:21px;line-height:1.2;margin:0 0 12px}.row{display:flex;align-items:center;gap:8px;border-bottom:1px solid #e2e9f3;padding:7px 0}.row:last-child{border:0}.row strong{min-width:115px}.chip,.badge{display:inline-flex;align-items:center;border-radius:7px;padding:3px 8px;background:#e8eef7;color:#243a58;font-size:13px;white-space:nowrap}.branch{background:#dbeafe;color:#075be8}.badge.Current,.badge.ready,.badge.approved,.badge.complete,.badge.merged{background:#d8f4df;color:#155b32}.badge.Stale{background:#fff0ca;color:#8a5200}.badge.Missing,.badge.Collisions{background:#fee1e3;color:#a82836}.badge.Other{background:#e8eef7}.harnesses{display:flex;flex-wrap:wrap;gap:6px}.counts{display:grid;grid-template-columns:repeat(4,1fr);gap:8px;text-align:center}.counts span{font-size:12px}.counts b{display:inline-block;margin-top:4px;padding:4px 9px;border-radius:7px;background:#e8eef7}.targets{margin-top:10px}.target{display:flex;justify-content:space-between;border-top:1px solid #e2e9f3;padding:6px 0;font-size:14px}.details-button,.secondary{border:0;background:transparent;color:#075be8;font-weight:700;padding:0}.workflow{display:flex;flex-direction:column}.lookup{display:grid;grid-template-columns:1fr auto;gap:8px}.lookup input,.filters input,.filters select{border:1px solid #b8c9df;border-radius:7px;padding:8px;min-width:0}.primary{border:1px solid #075be8;background:#075be8;color:white;border-radius:7px;padding:8px 12px;font-weight:700}.workflow-result{margin-top:10px;font-size:14px}.workflow-result p{margin:5px 0}.external::after{content:" ↗"}.activity{display:grid;grid-template-rows:auto auto auto minmax(0,1fr) auto;gap:10px}.activity-head{display:flex;justify-content:space-between;align-items:flex-start;gap:12px}.activity-head h2{margin-bottom:2px}.subtle{margin:0;color:#465978}.pause{border:1px solid #b8c9df;background:#f5f8fc;color:#132b4d;border-radius:7px;padding:8px 12px;white-space:nowrap}.filters{display:grid;grid-template-columns:1fr 1fr 1fr;gap:10px}.filters label{display:grid;gap:4px;font-size:13px;font-weight:700}.activity-state{margin:0;min-height:20px;color:#465978;font-size:14px}.activity-state.error{color:#a82836}.activity-list{min-height:0;overflow-y:auto;border-top:1px solid #dbe4f0}.date{position:sticky;top:0;background:#fff;color:#465978;font-size:12px;font-weight:700;padding:7px 0;border-bottom:1px solid #e2e9f3}.event{display:grid;grid-template-columns:minmax(108px,1fr) minmax(92px,1fr) auto 95px;gap:10px;align-items:center;border-bottom:1px solid #e2e9f3;padding:9px 0}.event a{color:#075be8;font-weight:700}.event time{color:#465978;font-size:13px;text-align:right}.activity-footer{display:flex;justify-content:center}.load{width:100%;border:1px solid #b8c9df;background:#f5f8fc;color:#075be8;border-radius:7px;padding:8px;font-weight:700}.foot{display:flex;align-items:center;justify-content:space-between;color:#465978;font-size:12px}.drawer{width:min(680px,calc(100vw - 32px));max-height:min(78vh,680px);border:1px solid #cbd7e8;border-radius:12px;color:#132b4d;padding:0;box-shadow:0 20px 50px #132b4d44}.drawer::backdrop{background:#132b4d66}.drawer-head{display:flex;justify-content:space-between;align-items:center;padding:16px;border-bottom:1px solid #dbe4f0}.drawer-body{max-height:calc(min(78vh,680px) - 70px);overflow:auto;padding:16px}.drawer pre{white-space:pre-wrap;overflow-wrap:anywhere;margin:0;font:13px/1.4 ui-monospace,SFMono-Regular,Menlo,monospace}.close{border:0;background:transparent;font-size:20px}@media(max-width:800px){.shell{height:auto;min-height:100vh;display:block;overflow:visible}.top{height:56px}.identity{padding-top:18px;padding-bottom:18px}.updated{display:none}.dashboard{display:block;padding-bottom:16px}.left{display:grid;grid-template-rows:none;margin-bottom:16px}.card{min-height:auto}.activity{min-height:600px}.activity-list{min-height:360px}.foot{height:36px}.filters{grid-template-columns:1fr}.event{grid-template-columns:1fr 1fr}.event time{text-align:left}.event .stage{grid-column:1}.identity h1{font-size:27px}}@media(max-height:720px) and (min-width:801px){.shell{height:auto;overflow:visible}.dashboard{min-height:620px}.activity-list{min-height:300px}}
</style></head><body><div class="shell"><header class="top"><strong>BUILDER GARAGE</strong><span>Local · Read only</span></header><section class="identity"><div><h1>{{.Project}}</h1><p class="root">{{.Root}}</p></div><span class="updated">Updated while open</span></section><main class="dashboard"><aside class="left"><section class="card"><h2>Project overview</h2><div class="row"><strong>Integration branch</strong><span class="chip branch">{{.Config.IntegrationBranch}}</span></div><div class="row"><strong>Manuals</strong><span>{{range $i,$m:=.Config.Manuals}}{{if $i}}, {{end}}{{$m}}{{end}}</span></div><div class="row"><strong>Harnesses</strong><span class="harnesses">{{range .Config.Harnesses}}<span class="chip">{{.}}</span>{{end}}</span></div></section><section class="card"><div style="display:flex;justify-content:space-between;align-items:start"><h2>Skills &amp; manuals</h2><button class="details-button" type="button" data-drawer="skills">View details</button></div>{{if .Skills.Problem}}<p>{{.Skills.Problem}}</p>{{else}}<div class="counts"><span>Current<br><b class="badge Current">{{.Skills.Current}}</b></span><span>Stale<br><b class="badge Stale">{{.Skills.Stale}}</b></span><span>Missing<br><b class="badge Missing">{{.Skills.Missing}}</b></span><span>Collisions<br><b class="badge Collisions">{{.Skills.Collisions}}</b></span></div><div class="targets">{{range .Skills.Targets}}<div class="target"><span>{{.Harness}}</span><span class="badge {{.State}}">{{.State}}</span></div>{{end}}{{if .Skills.Other}}<div class="target"><span>Additional states</span><span class="badge Other">{{.Skills.Other}}</span></div>{{end}}</div>{{end}}</section><section class="card workflow"><h2>Workflow status</h2><form id="workflow-form" class="lookup"><input id="workflow-number" inputmode="numeric" placeholder="#165" aria-label="Issue or pull request number"><button class="primary">Look up</button></form><div id="workflow-result" class="workflow-result"><p class="subtle">Look up an issue or pull request for recorded GitHub facts and suggested next actions.</p></div></section></aside><section class="card activity"><div class="activity-head"><div><h2>Recorded activity</h2><p class="subtle">Local Garage events — not live agent execution or a transcript.</p></div><button id="pause" class="pause" type="button" aria-pressed="false">Pause</button></div><div class="filters" aria-label="Activity filters"><label>Issue or PR<input id="number-filter" inputmode="numeric" placeholder="All issues"></label><label>Workflow stage<select id="type-filter"><option value="">All stages</option><option>discovery</option><option>readiness</option><option>implementation</option><option>correction</option><option>review</option><option>pr-review</option><option>finalization</option></select></label><label>Outcome<select id="outcome-filter"><option value="">All outcomes</option><option>ready</option><option>not-ready</option><option>blocked</option><option>approved</option><option>changes-required</option><option>complete</option><option>merged</option></select></label></div><p id="activity-state" class="activity-state" role="status">Loading recorded events…</p><div id="activity-list" class="activity-list" tabindex="0" aria-label="Recorded Garage events"></div><div id="activity-footer" class="activity-footer" hidden><button id="older" class="load" type="button">Load older events</button></div></section></main><footer class="foot"><span>GitHub records remain authoritative.</span><span>Read-only local dashboard</span></footer></div><dialog id="details" class="drawer" aria-labelledby="drawer-title"><div class="drawer-head"><strong id="drawer-title">Details</strong><button id="close-drawer" class="close" type="button" aria-label="Close details">×</button></div><div id="drawer-body" class="drawer-body"></div></dialog><template id="skills-details">{{range .Skills.Details}}{{.}}
{{end}}</template><script>
const list=document.querySelector('#activity-list'),state=document.querySelector('#activity-state'),footer=document.querySelector('#activity-footer'),older=document.querySelector('#older'),pause=document.querySelector('#pause'),numberFilter=document.querySelector('#number-filter'),typeFilter=document.querySelector('#type-filter'),outcomeFilter=document.querySelector('#outcome-filter'),drawer=document.querySelector('#details'),drawerTitle=document.querySelector('#drawer-title'),drawerBody=document.querySelector('#drawer-body'),workflowForm=document.querySelector('#workflow-form'),workflowNumber=document.querySelector('#workflow-number'),workflowResult=document.querySelector('#workflow-result');let paused=false,nextOffset=-1,newest='',rendered=new Set();
function external(a,label){a.target='_blank';a.rel='noopener noreferrer';a.classList.add('external');a.setAttribute('aria-label',label+' (opens in a new tab)');return a}function filters(offset){const q=new URLSearchParams();if(numberFilter.value.trim())q.set('number',numberFilter.value.trim());if(typeFilter.value)q.set('type',typeFilter.value);if(outcomeFilter.value)q.set('outcome',outcomeFilter.value);if(offset)q.set('offset',offset);return q}function key(e){return[e.time,e.kind,e.number,e.type,e.outcome,e.url].join('|')}function label(e){return e.kind==='pr'?'PR #'+e.number:'Issue #'+e.number}function stamp(v){const d=new Date(v);return Number.isNaN(d.getTime())?v:d.toLocaleString()}function date(v){const d=new Date(v);return Number.isNaN(d.getTime())?'Recorded events':d.toLocaleDateString(undefined,{weekday:'long',month:'short',day:'numeric'})}
function position(){const a=document.activeElement,r=a instanceof Element?a.closest('[data-event-key]'):null;return{top:list.scrollTop,key:r?.dataset.eventKey||'',link:a?.tagName==='A'}}function restore(p){list.scrollTop=p.top;if(!p.key)return;const r=[...list.children].find(n=>n.dataset?.eventKey===p.key),target=p.link?r?.querySelector('a'):r;target?.focus({preventScroll:true})}function render(events,append){const p=append?null:position();if(!append){list.replaceChildren();rendered=new Set()}if(!events.length&&!append){list.innerHTML='<p class="subtle" style="padding:12px 0">No recorded Garage events match these filters.</p>';return}let last='';for(const e of events){const d=date(e.time);if(d!==last){const h=document.createElement('div');h.className='date';h.textContent=d;list.append(h);last=d}const row=document.createElement('article');row.className='event';row.dataset.eventKey=key(e);const target=e.url?external(document.createElement('a'),label(e)):document.createElement('span');target.textContent=label(e);if(e.url)target.href=e.url;row.append(target);const stage=document.createElement('span');stage.className='stage';stage.textContent=e.type;row.append(stage);const outcome=document.createElement('span');outcome.className='badge '+e.outcome;outcome.textContent=e.outcome;row.append(outcome);const time=document.createElement('time');time.dateTime=e.time;time.title=stamp(e.time);time.textContent=stamp(e.time);row.append(time);list.append(row);rendered.add(key(e))}if(p)restore(p)}
async function load(o={}){const append=!!o.append,auto=!!o.auto,offset=append?nextOffset:0;if(offset<0&&append)return;try{const r=await fetch('activity?'+filters(offset));if(!r.ok)throw new Error(await r.text()||'Activity unavailable');const data=await r.json(),incoming=data.events||[];if(auto&&list.scrollTop>8&&incoming.length&&newest&&key(incoming[0])!==newest){state.textContent='New recorded events available. Return to the top or change a filter to refresh.';return}render(incoming,append);nextOffset=data.nextOffset;footer.hidden=nextOffset<0;if(incoming.length&&!append)newest=key(incoming[0]);state.className='activity-state';state.textContent=(paused?'Automatic refresh paused. ':'Refreshing every 15 seconds. ')+'Updated '+new Date().toLocaleTimeString()+'.'}catch(e){state.className='activity-state error';state.textContent='Activity unavailable: '+e.message;footer.hidden=true}}function reset(){nextOffset=-1;newest='';load()}[numberFilter,typeFilter,outcomeFilter].forEach(x=>x.addEventListener('change',reset));older.addEventListener('click',()=>load({append:true}));pause.addEventListener('click',()=>{paused=!paused;pause.setAttribute('aria-pressed',String(paused));pause.textContent=paused?'Resume':'Pause';state.textContent=paused?'Automatic refresh paused.':'Automatic refresh resumed.'});setInterval(()=>{if(!paused)load({auto:true})},15000);load();
function openDetails(title,content){drawerTitle.textContent=title;drawerBody.replaceChildren();const pre=document.createElement('pre');pre.textContent=content;drawerBody.append(pre);drawer.showModal()}document.querySelectorAll('[data-drawer]').forEach(b=>b.addEventListener('click',()=>openDetails('Skills & manuals details',document.querySelector('#skills-details').content.textContent)));document.querySelector('#close-drawer').addEventListener('click',()=>drawer.close());drawer.addEventListener('click',e=>{if(e.target===drawer)drawer.close()});
workflowForm.addEventListener('submit',async e=>{e.preventDefault();const n=workflowNumber.value.trim().replace(/^#/,'');if(!/^[1-9][0-9]*$/.test(n)){workflowResult.textContent='Enter a positive issue or pull request number.';return}workflowResult.textContent='Loading recorded status…';try{const r=await fetch('status?number='+encodeURIComponent(n));if(!r.ok)throw new Error(await r.text());const d=await r.json();workflowResult.replaceChildren();const title=document.createElement('strong');title.textContent=d.title||d.kind;workflowResult.append(title);for(const fact of d.facts||[]){const p=document.createElement('p');p.textContent=fact.label+': '+fact.text;workflowResult.append(p)}if(d.url){const a=external(document.createElement('a'),'Open '+(d.kind||'record'));a.href=d.url;a.textContent=d.url.includes('/pull/')?'Open PR':'Open issue';workflowResult.append(a)}if((d.suggestions||[]).length){const p=document.createElement('p');p.textContent='Suggestions: '+d.suggestions.map(x=>x.text).join(' ');workflowResult.append(p)}if((d.details||[]).length){const b=document.createElement('button');b.className='details-button';b.type='button';b.textContent='View evidence';b.addEventListener('click',()=>openDetails('Workflow status details',d.details.map(x=>x.label+': '+x.text).join('\n')));workflowResult.append(document.createTextNode(' '),b)}if(d.problem){const p=document.createElement('p');p.className='subtle';p.textContent='Status limitation: '+d.problem;workflowResult.append(p)}}catch(err){workflowResult.textContent='Status unavailable: '+err.message}});
</script></body></html>`
