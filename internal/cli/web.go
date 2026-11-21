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

type webPageData struct {
	Project, Root, Slug, Config, Skills string
	StatusNumber                        int
	Status                              string
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
		result.Events = append(result.Events, webActivityEvent{Time: event.Time, Kind: event.Kind, Number: event.Number, Type: event.Type, Outcome: event.Outcome, URL: event.URL})
	}
	if end < len(matching) {
		result.NextOffset = end
	}
	return result, nil
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

const webPage = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>{{.Project}} — Builder Garage</title><style>
:root{color:#12213d;background:#edf2f9;font:16px/1.45 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}*{box-sizing:border-box}body{margin:0}.bar{background:#10294e;color:#fff;padding:.8rem max(1rem,calc((100vw - 76rem)/2));display:flex;justify-content:space-between;gap:1rem}.bar strong{letter-spacing:.02em}.page{max-width:76rem;margin:0 auto;padding:1.5rem 1rem 2rem}.project{margin-bottom:1.25rem}.project h1{margin:0;font-size:1.75rem}.root{overflow-wrap:anywhere;color:#465978;margin:.15rem 0}.notice{color:#465978;margin:.5rem 0 0}.layout{display:grid;grid-template-columns:minmax(18rem,1fr) minmax(0,2fr);gap:1rem;align-items:start}.panel{background:#fff;border:1px solid #cfdaeb;border-radius:.75rem;padding:1rem;box-shadow:0 1px 2px #172c5214}.panel h2{margin:0 0 .35rem;font-size:1.2rem}.panel h3{font-size:1rem;margin:1rem 0 .25rem}.panel pre{white-space:pre-wrap;overflow-wrap:anywhere;background:#f5f8fc;border-radius:.4rem;padding:.7rem;margin:0;font:13px/1.45 ui-monospace,SFMono-Regular,Menlo,monospace}.status-form{display:flex;gap:.5rem;flex-wrap:wrap}.status-form input,.filters input,.filters select,button{font:inherit;border:1px solid #b4c4dd;border-radius:.4rem;padding:.45rem .55rem;background:#fff;color:#12213d}button{cursor:pointer;font-weight:650}button.primary{background:#0969da;border-color:#0969da;color:#fff}.activity-panel{min-height:32rem;display:flex;flex-direction:column}.activity-head{display:flex;justify-content:space-between;gap:1rem;align-items:start}.activity-head p{margin:.1rem 0;color:#465978}.filters{display:grid;grid-template-columns:1fr 1fr 1fr;gap:.5rem;margin:1rem 0 .65rem}.filters label{display:grid;gap:.2rem;font-size:.84rem;font-weight:650}.activity-state{min-height:1.4rem;margin:.1rem 0 .5rem;color:#465978}.activity-state.error{color:#b42318}.activity-list{border-top:1px solid #d9e2ef;overflow:auto;max-height:34rem;min-height:16rem}.event{display:grid;grid-template-columns:auto minmax(8rem,1fr) minmax(7rem,1fr) auto;gap:.55rem;align-items:center;padding:.7rem .25rem;border-bottom:1px solid #e3eaf4}.event time{font-variant-numeric:tabular-nums;color:#465978;white-space:nowrap}.event a{color:#075dc8;font-weight:700}.event .type{font-weight:650}.badge{border-radius:999px;padding:.12rem .5rem;background:#e8eef7;color:#253957;font-size:.88rem;white-space:nowrap}.badge.ready,.badge.approved,.badge.complete,.badge.merged{background:#d8f4df;color:#14532d}.badge.blocked,.badge.changes-required,.badge.not-ready{background:#fee2e2;color:#991b1b}.empty{padding:1.2rem .25rem;color:#465978}.activity-footer{display:flex;justify-content:center;padding-top:.75rem}.activity-footer[hidden]{display:none}@media (max-width:50rem){.layout{grid-template-columns:1fr}.activity-panel{min-height:auto}.activity-list{max-height:28rem}.filters{grid-template-columns:1fr}}@media (max-width:35rem){.event{grid-template-columns:1fr 1fr}.event .type{grid-column:1}.bar{padding:.8rem 1rem}}
</style></head><body><header class="bar"><strong>⚒ BUILDER GARAGE</strong><span>Local · Read only</span></header><main class="page"><section class="project"><h1>{{.Project}}</h1><p class="root">{{.Root}}</p><p class="notice">GitHub records are workflow authority. Local activity is a private annotation and may be incomplete.</p></section><div class="layout"><div><section class="panel"><h2>Project overview</h2><h3>Adopted configuration</h3><pre>{{.Config}}</pre><h3>Installed skills and manuals</h3><pre>{{.Skills}}</pre></section><section class="panel"><h2>Workflow status</h2><form class="status-form" method="get"><label>Issue or PR number <input name="number" value="{{.StatusNumber}}" inputmode="numeric"></label><button class="primary">Check status</button></form>{{if .Status}}<pre>{{.Status}}</pre>{{end}}</section></div><section class="panel activity-panel" aria-labelledby="activity-title"><div class="activity-head"><div><h2 id="activity-title">Recorded activity</h2><p>Persisted Garage events, not live agent execution or a conversation transcript.</p></div><button id="pause" type="button" aria-pressed="false">Pause refresh</button></div><div class="filters" aria-label="Activity filters"><label>Issue or PR number<input id="number-filter" inputmode="numeric" pattern="[0-9]*" placeholder="All targets"></label><label>Workflow stage<select id="type-filter"><option value="">All stages</option><option>discovery</option><option>readiness</option><option>implementation</option><option>correction</option><option>review</option><option>pr-review</option><option>finalization</option></select></label><label>Outcome<select id="outcome-filter"><option value="">All outcomes</option><option>ready</option><option>not-ready</option><option>blocked</option><option>approved</option><option>changes-required</option><option>complete</option><option>merged</option></select></label></div><p id="activity-state" class="activity-state" role="status">Loading recorded events…</p><div id="activity-list" class="activity-list" tabindex="0" aria-label="Recorded Garage events"></div><div id="activity-footer" class="activity-footer" hidden><button id="older" type="button">Load older recorded events</button></div></section></div></main><script>
const list=document.querySelector('#activity-list'),state=document.querySelector('#activity-state'),footer=document.querySelector('#activity-footer'),older=document.querySelector('#older'),pause=document.querySelector('#pause'),numberFilter=document.querySelector('#number-filter'),typeFilter=document.querySelector('#type-filter'),outcomeFilter=document.querySelector('#outcome-filter');let paused=false,nextOffset=-1,newest='',rendered=new Set(),timer;
function filters(offset){const q=new URLSearchParams();if(numberFilter.value.trim())q.set('number',numberFilter.value.trim());if(typeFilter.value)q.set('type',typeFilter.value);if(outcomeFilter.value)q.set('outcome',outcomeFilter.value);if(offset)q.set('offset',offset);return q}
function key(e){return[e.time,e.kind,e.number,e.type,e.outcome,e.url].join('|')}
function label(e){return e.kind==='pr'?'PR #'+e.number:'Issue #'+e.number}
function stamp(value){const d=new Date(value);return Number.isNaN(d.getTime())?value:d.toLocaleString()}
function activityPosition(){const active=document.activeElement,row=active instanceof Element?active.closest('[data-event-key]'):null;return{scrollTop:list.scrollTop,focusedKey:row?.dataset.eventKey||'',focusedLink:active?.tagName==='A'}}
function restoreActivityPosition(position){list.scrollTop=position.scrollTop;if(!position.focusedKey)return;const row=[...list.children].find(node=>node.dataset&&node.dataset.eventKey===position.focusedKey),target=position.focusedLink?row?.querySelector('a'):row;target?.focus({preventScroll:true})}
function render(events,append){const position=append?null:activityPosition();if(!append){list.replaceChildren();rendered=new Set()}if(!events.length&&!append){const p=document.createElement('p');p.className='empty';p.textContent='No recorded Garage events match these filters.';list.append(p)}for(const e of events){const row=document.createElement('article');row.className='event';row.dataset.eventKey=key(e);const time=document.createElement('time');time.dateTime=e.time;time.textContent=stamp(e.time);row.append(time);const target=e.url?document.createElement('a'):document.createElement('span');target.textContent=label(e);if(e.url){target.href=e.url;target.rel='noreferrer'}row.append(target);const type=document.createElement('span');type.className='type';type.textContent=e.type;row.append(type);const outcome=document.createElement('span');outcome.className='badge '+e.outcome;outcome.textContent=e.outcome;row.append(outcome);list.append(row);rendered.add(key(e))}if(position)restoreActivityPosition(position)}
async function load(opts={}){const append=!!opts.append,auto=!!opts.auto,offset=append?nextOffset:0;if(offset<0&&append)return;try{const response=await fetch('activity?'+filters(offset),{headers:{Accept:'application/json'}});if(!response.ok)throw new Error(await response.text()||'Activity is unavailable');const data=await response.json();const incoming=data.events||[];if(auto&&list.scrollTop>8&&incoming.length&&newest&&key(incoming[0])!==newest){const unseen=incoming.filter(e=>!rendered.has(key(e))).length;state.className='activity-state';state.textContent=(unseen||1)+' new recorded event'+((unseen||1)===1?'':'s')+' available. Return to the top or change a filter to refresh.';return}render(incoming,append);nextOffset=data.nextOffset;footer.hidden=nextOffset<0;if(incoming.length&&!append)newest=key(incoming[0]);state.className='activity-state';state.textContent='Showing recorded Garage events. Refreshed '+new Date().toLocaleTimeString()+'.'+(data.discarded?' '+data.discarded+' unreadable entr'+(data.discarded===1?'y':'ies')+' skipped.':'')}catch(err){state.className='activity-state error';state.textContent='Activity unavailable: '+err.message;footer.hidden=true}}
function reset(){nextOffset=-1;newest='';load()}numberFilter.addEventListener('change',reset);typeFilter.addEventListener('change',reset);outcomeFilter.addEventListener('change',reset);older.addEventListener('click',()=>load({append:true}));pause.addEventListener('click',()=>{paused=!paused;pause.setAttribute('aria-pressed',String(paused));pause.textContent=paused?'Resume refresh':'Pause refresh';state.className='activity-state';state.textContent=paused?'Automatic refresh paused.':'Automatic refresh resumed.'});load();timer=setInterval(()=>{if(!paused)load({auto:true})},15000);
</script></body></html>`
