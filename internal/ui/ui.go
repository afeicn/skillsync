package ui

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/afeicn/skillsync/internal/config"
	"github.com/afeicn/skillsync/internal/gitx"
	"github.com/afeicn/skillsync/internal/recon"
)

//go:embed index.html
var page []byte

type Server struct {
	cfg *config.Config
	mu  sync.Mutex
}

func Serve(cfg *config.Config, port string, noBrowser bool) error {
	s := &Server{cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/api/status", s.status)
	mux.HandleFunc("/api/sync", s.sync)
	mux.HandleFunc("/api/sync-stream", s.syncStream)
	mux.HandleFunc("/api/resolve", s.resolve)
	mux.HandleFunc("/api/skills", s.skills)
	mux.HandleFunc("/api/pending", s.pending)
	mux.HandleFunc("/api/history", s.history)
	mux.HandleFunc("/api/settings", s.settings)
	mux.HandleFunc("/api/launch", s.launch)
	mux.HandleFunc("/api/init", s.init)
	mux.HandleFunc("/api/skill", s.skillDetail)

	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return err
	}
	actual := ln.Addr().(*net.TCPAddr).Port
	fmt.Printf("LISTENING %d\n", actual)
	url := fmt.Sprintf("http://127.0.0.1:%d", actual)
	if !noBrowser {
		openBrowser(url)
	}
	return http.Serve(ln, mux)
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "darwin":
		_, _ = exec.Command("open", url).Output()
	case "windows":
		_, _ = exec.Command("cmd", "/c", "start", "", url).Output()
	default:
		_, _ = exec.Command("xdg-open", url).Output()
	}
}

type wfn func(string)

func (f wfn) Write(p []byte) (int, error) {
	f(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page)
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		json.NewEncoder(w).Encode(map[string]any{"configured": false})
		return
	}
	rep, err := recon.Collect(s.cfg)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	host, _ := os.Hostname()
	pending, _ := filepath.Glob(config.StateDir() + "/pending-*.md")
	last := recon.LastSync()
	json.NewEncoder(w).Encode(map[string]any{
		"configured": true,
		"machine":    host,
		"repo":      s.cfg.Repo,
		"branch":    s.cfg.Branch,
		"skillsDir": s.cfg.SkillsDir,
		"interval":  s.cfg.Interval,
		"report":    rep,
		"pending":   pending,
		"lastSync":  last,
	})
}

func (s *Server) needCfg(w http.ResponseWriter) bool {
	if s.cfg == nil {
		http.Error(w, "not configured", 400)
		return false
	}
	return true
}

func (s *Server) sync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !s.needCfg(w) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var log []string
	recon.SetLog(func(l string) { log = append(log, l) })
	code := recon.Auto(s.cfg)
	recon.SetLog(nil)
	json.NewEncoder(w).Encode(map[string]any{"code": code, "log": log})
}

func (s *Server) syncStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !s.needCfg(w) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch := make(chan string, 128)
	recon.SetLog(func(l string) { ch <- l })
	go func() {
		code := recon.Auto(s.cfg)
		ch <- fmt.Sprintf("EXIT %d", code)
		close(ch)
	}()
	for line := range ch {
		fmt.Fprintf(w, "data: %s\n\n", strings.ReplaceAll(line, "\n", " "))
		fl.Flush()
	}
	recon.SetLog(nil)
}

func (s *Server) resolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var req struct {
		Skill string `json:"skill"`
		Side  string `json:"side"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Skill == "" {
		http.Error(w, "bad request", 400)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	g := gitx.Repo{Dir: s.cfg.SkillsDir}
	var log []string
	wl := func(line string) { log = append(log, line) }
	if !g.HasMerge() {
		out, _ := g.MergeNoCommit("origin/" + s.cfg.Branch)
		wl(out)
	}
	var kl, kr []string
	if req.Side == "local" {
		kl = []string{req.Skill}
	} else {
		kr = []string{req.Skill}
	}
	code := recon.ResolveW(s.cfg, kl, kr, wfn(wl))
	if code == 0 && len(g.Unmerged()) == 0 {
		if fails := recon.Verify(s.cfg); recon.HasFail(fails) {
			g.MergeAbort()
			wl("verify failed after resolve — merge aborted")
			code = 1
		} else if g.HasMerge() || !g.StagedClean() {
			out, err := g.Run("commit", "-m", "chore(ui): resolve "+req.Skill+" keep "+req.Side)
			wl(out)
			if err == nil {
				out, err = g.Run("push", "origin", s.cfg.Branch)
				wl(out)
				if err != nil {
					code = 1
				}
			} else {
				code = 1
			}
		}
	}
	json.NewEncoder(w).Encode(map[string]any{"code": code, "log": log})
}

type skillInfo struct {
	Name    string `json:"name"`
	Desc    string `json:"desc"`
	Updated string `json:"updated"`
}

func (s *Server) skills(w http.ResponseWriter, r *http.Request) {
	if !s.needCfg(w) {
		return
	}
	g := gitx.Repo{Dir: s.cfg.SkillsDir}
	out, err := g.Run("ls-tree", "-d", "--name-only", "HEAD")
	if err != nil {
		http.Error(w, out, 500)
		return
	}
	var list []skillInfo
	for _, name := range strings.Split(out, "\n") {
		name = strings.TrimSpace(name)
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		si := skillInfo{Name: name}
		if b, err := os.ReadFile(filepath.Join(s.cfg.SkillsDir, name, "SKILL.md")); err == nil {
			si.Desc = frontDesc(string(b))
		}
		if ts, err := g.Run("log", "-1", "--format=%ct", "--", name); err == nil {
			if n, e := strconv.ParseInt(strings.TrimSpace(ts), 10, 64); e == nil {
				si.Updated = fmtTime(n)
			}
		}
		list = append(list, si)
	}
	json.NewEncoder(w).Encode(list)
}

func fmtTime(unix int64) string {
	return time.Unix(unix, 0).Format("2006-01-02")
}

func frontDesc(md string) string {
	lines := strings.Split(md, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			break
		}
		if strings.HasPrefix(l, "description:") {
			d := strings.TrimSpace(strings.TrimPrefix(l, "description:"))
			if len(d) > 160 {
				d = d[:160] + "…"
			}
			return d
		}
	}
	return ""
}

func (s *Server) pending(w http.ResponseWriter, r *http.Request) {
	files, _ := filepath.Glob(config.StateDir() + "/pending-*.md")
	type item struct {
		Name string `json:"name"`
		Text string `json:"text"`
	}
	var list []item
	for _, f := range files {
		b, _ := os.ReadFile(f)
		t := string(b)
		if len(t) > 4000 {
			t = t[:4000] + "\n…"
		}
		list = append(list, item{Name: filepath.Base(f), Text: t})
	}
	json.NewEncoder(w).Encode(list)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	b, err := os.ReadFile(filepath.Join(config.StateDir(), "skillsync.log"))
	if err != nil {
		json.NewEncoder(w).Encode([]string{})
		return
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[len(lines)-1-i] = l
	}
	json.NewEncoder(w).Encode(out)
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	if !s.needCfg(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		json.NewEncoder(w).Encode(map[string]any{
			"repo":     s.cfg.Repo,
			"branch":   s.cfg.Branch,
			"interval": s.cfg.Interval,
			"targets":  s.cfg.Targets,
			"launch":   launchInstalled(),
		})
	case http.MethodPut:
		var req struct {
			Interval int    `json:"interval"`
			Repo     string `json:"repo"`
			Branch   string `json:"branch"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if req.Repo != "" && req.Repo != s.cfg.Repo {
			g := gitx.Repo{Dir: s.cfg.SkillsDir}
			if _, err := g.Run("ls-remote", req.Repo); err != nil {
				http.Error(w, "仓库无法访问，请检查地址和权限", 400)
				return
			}
			s.cfg.Repo = req.Repo
		}
		if req.Branch != "" {
			s.cfg.Branch = req.Branch
		}
		if req.Interval >= 1 {
			s.cfg.Interval = req.Interval
		}
		if err := s.cfg.Save(""); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	default:
		http.Error(w, "GET/PUT only", 405)
	}
}

func launchPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "cn.afeicn.skillsync.plist")
}

func launchInstalled() bool { _, err := os.Stat(launchPath()); return err == nil }

func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !s.needCfg(w) {
		return
	}
	var req struct {
		Action string `json:"action"` // install | uninstall
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	bin, _ := os.Executable()
	path := launchPath()
	switch req.Action {
	case "install":
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>Label</key><string>cn.afeicn.skillsync</string>
<key>ProgramArguments</key><array><string>%s</string><string>sync</string></array>
<key>StartInterval</key><integer>%d</integer>
<key>RunAtLoad</key><true/>
<key>StandardOutPath</key><string>%s</string>
<key>StandardErrorPath</key><string>%s</string>
</dict></plist>`, bin, s.cfg.Interval*60,
			filepath.Join(config.StateDir(), "launchd-out.log"),
			filepath.Join(config.StateDir(), "launchd-err.log"))
		if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_, _ = exec.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid()), path).Output()
		_, _ = exec.Command("launchctl", "bootstrap", "gui/"+strconv.Itoa(os.Getuid()), path).Output()
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "installed": true})
	case "uninstall":
		_, _ = exec.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid()), path).Output()
		_ = os.Remove(path)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "installed": false})
	default:
		http.Error(w, "action required", 400)
	}
}

func (s *Server) init(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	if s.cfg != nil {
		http.Error(w, "already configured", 400)
		return
	}
	var req struct {
		Repo     string `json:"repo"`
		Branch   string `json:"branch"`
		Interval int    `json:"interval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Repo == "" {
		http.Error(w, "仓库地址不能为空", 400)
		return
	}
	branch := req.Branch
	if branch == "" {
		branch = "main"
	}
	interval := req.Interval
	if interval < 1 {
		interval = 60
	}
	home, _ := os.UserHomeDir()
	skillsDir := filepath.Join(home, ".skillsync", "skills")
	if _, err := os.Stat(filepath.Join(skillsDir, ".git")); err != nil {
		g := gitx.Repo{Dir: filepath.Dir(skillsDir)}
		_ = os.MkdirAll(filepath.Dir(skillsDir), 0o755)
		if out, err := g.Run("clone", req.Repo, skillsDir); err != nil {
			http.Error(w, "克隆失败："+strings.TrimSpace(out), 400)
			return
		}
	}
	cfg := &config.Config{Repo: req.Repo, Branch: branch, SkillsDir: skillsDir,
		Interval: interval, Targets: detectTargets()}
	if err := cfg.Save(""); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.cfg = cfg
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "skillsDir": skillsDir})
}

func detectTargets() []string {
	home, _ := os.UserHomeDir()
	cands := []string{
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".codex", "skills"),
		filepath.Join(home, ".config", "opencode", "skills"),
		filepath.Join(home, ".cursor", "skills"),
	}
	var out []string
	for _, c := range cands {
		if st, err := os.Stat(filepath.Dir(c)); err == nil && st.IsDir() {
			out = append(out, c)
		}
	}
	return out
}

func (s *Server) skillDetail(w http.ResponseWriter, r *http.Request) {
	if !s.needCfg(w) {
		return
	}
	name := filepath.Base(r.URL.Query().Get("name"))
	if name == "" || name == "." || name == ".." {
		http.Error(w, "bad name", 400)
		return
	}
	b, err := os.ReadFile(filepath.Join(s.cfg.SkillsDir, name, "SKILL.md"))
	if err != nil {
		http.Error(w, "SKILL.md not found", 404)
		return
	}
	t := string(b)
	if len(t) > 20000 {
		t = t[:20000] + "\n…"
	}
	json.NewEncoder(w).Encode(map[string]any{"name": name, "content": t})
}
