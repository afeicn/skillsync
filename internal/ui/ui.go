package ui

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/afeicn/skillsync/internal/config"
	"github.com/afeicn/skillsync/internal/gitx"
	"github.com/afeicn/skillsync/internal/recon"
)

type wfn func(string)

func (f wfn) Write(p []byte) (int, error) {
	f(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

//go:embed index.html
var page []byte

type Server struct {
	cfg *config.Config
	mu  sync.Mutex
}

func Serve(cfg *config.Config, port string) error {
	s := &Server{cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
	})
	mux.HandleFunc("/api/status", s.status)
	mux.HandleFunc("/api/sync", s.sync)
	mux.HandleFunc("/api/resolve", s.resolve)
	host := "127.0.0.1:" + port
	url := "http://" + host
	fmt.Println("skillsync ui →", url)
	openBrowser(url)
	return http.ListenAndServe(host, mux)
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

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	rep, err := recon.Collect(s.cfg)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	host, _ := os.Hostname()
	pending, _ := filepath.Glob(config.StateDir() + "/pending-*.md")
	json.NewEncoder(w).Encode(map[string]any{
		"machine":   host,
		"repo":      s.cfg.Repo,
		"branch":    s.cfg.Branch,
		"skillsDir": s.cfg.SkillsDir,
		"interval":  s.cfg.Interval,
		"report":    rep,
		"pending":   pending,
	})
}

func (s *Server) sync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
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

func (s *Server) resolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var req struct {
		Skill string `json:"skill"`
		Side  string `json:"side"` // local | remote
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
