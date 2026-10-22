package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/afeicn/skillsync/internal/config"
	"github.com/afeicn/skillsync/internal/gitx"
	"github.com/afeicn/skillsync/internal/recon"
	"github.com/afeicn/skillsync/internal/ui"
)

var version = "dev"

const usage = `skillsync — git-based skill sync & reconciliation for AI coding agents

Usage:
  skillsync init    --repo <git-url> [--skills-dir DIR] [--targets a,b,c]
  skillsync status  [--config PATH]
  skillsync sync    [--config PATH]        # unattended: snapshot, reconcile, merge, push
  skillsync resolve --keep-local a,b --keep-remote c
  skillsync verify
  skillsync link                           # symlink skills into detected tool dirs
  skillsync watch                          # run sync on an interval
  skillsync ui      [--port 7788]          # local web dashboard
  skillsync version
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Print(usage)
		os.Exit(1)
	}
	switch args[0] {
	case "init":
		cmdInit(args[1:])
	case "status":
		cmdStatus(args[1:])
	case "sync":
		os.Exit(recon.Auto(loadCfg(args[1:])))
	case "merge":
		cfg := loadCfg(args[1:])
		g := gitx.Repo{Dir: cfg.SkillsDir}
		out, err := g.MergeNoCommit("origin/" + cfg.Branch)
		fmt.Println(out)
		if err != nil {
			fmt.Println("conflicts present — use `skillsync resolve --keep-local A --keep-remote B`")
			os.Exit(2)
		}
		fmt.Println("merged cleanly; run verify, commit & push")
	case "resolve":
		fs := flag.NewFlagSet("resolve", flag.ExitOnError)
		cfgPath := fs.String("config", "", "")
		kl := fs.String("keep-local", "", "")
		kr := fs.String("keep-remote", "", "")
		_ = fs.Parse(args[1:])
		os.Exit(recon.Resolve(loadCfgPath(*cfgPath),
			strings.Split(*kl, ","), strings.Split(*kr, ",")))
	case "verify":
		issues := recon.Verify(loadCfg(args[1:]))
		for _, i := range issues {
			fmt.Printf("[%s] %s: %s\n", i.Level, i.Path, i.Why)
		}
		if out, ok := recon.PreSync(loadCfg(args[1:])); !ok {
			fmt.Println("[pre-sync] FAILED:\n" + out)
		} else if out != "" {
			fmt.Println("[pre-sync] passed:\n" + out)
		}
		if len(issues) == 0 {
			fmt.Println("verify passed")
		}
	case "link":
		cmdLink(loadCfg(args[1:]))
	case "watch":
		cfg := loadCfg(args[1:])
		for {
			recon.Auto(cfg)
			time.Sleep(time.Duration(cfg.Interval) * time.Minute)
		}
	case "ui":
		fs := flag.NewFlagSet("ui", flag.ExitOnError)
		cfgPath := fs.String("config", "", "")
		port := fs.String("port", "7788", "")
		nobrowser := fs.Bool("no-browser", false, "")
		_ = fs.Parse(args[1:])
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			cfg = nil // no config yet: UI serves onboarding
		}
		if err := ui.Serve(cfg, *port, *nobrowser); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "version":
		fmt.Println(version)
	default:
		fmt.Print(usage)
		os.Exit(1)
	}
}

func loadCfg(args []string) *config.Config {
	fs := flag.NewFlagSet("cfg", flag.ExitOnError)
	p := fs.String("config", "", "")
	_ = fs.Parse(args)
	return loadCfgPath(*p)
}

func loadCfgPath(p string) *config.Config {
	cfg, err := config.Load(p)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		fmt.Fprintln(os.Stderr, "run `skillsync init --repo <git-url>` first")
		os.Exit(1)
	}
	return cfg
}

func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	repo := fs.String("repo", "", "skills git repo url (required)")
	dir := fs.String("skills-dir", "~/.skillsync/skills", "local clone of the repo")
	targets := fs.String("targets", "", "comma-separated tool skill dirs; empty = auto-detect")
	branch := fs.String("branch", "main", "")
	interval := fs.Int("interval", 60, "minutes between auto syncs")
	cfgPath := fs.String("config", "", "")
	_ = fs.Parse(args)
	if *repo == "" {
		fmt.Println("init: --repo is required")
		os.Exit(1)
	}
	skillsDir := config.Expand(*dir)
	if err := os.MkdirAll(filepath.Dir(skillsDir), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, ".git")); err != nil {
		g := gitx.Repo{Dir: filepath.Dir(skillsDir)}
		if _, err := g.Run("clone", *repo, skillsDir); err != nil {
			fmt.Fprintln(os.Stderr, "clone failed:", err)
			os.Exit(1)
		}
		fmt.Println("cloned", *repo, "->", skillsDir)
	} else {
		fmt.Println("repo already present at", skillsDir)
	}
	tgts := detectTargets()
	if *targets != "" {
		tgts = nil
		for _, t := range strings.Split(*targets, ",") {
			tgts = append(tgts, config.Expand(t))
		}
	}
	cfg := &config.Config{Repo: *repo, Branch: *branch, SkillsDir: skillsDir,
		Interval: *interval, Targets: tgts}
	if err := cfg.Save(*cfgPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("config saved:", config.DefaultPath())
	fmt.Println("detected targets:")
	for _, t := range tgts {
		fmt.Println(" -", t)
	}
	cmdLink(cfg)
}

func detectTargets() []string {
	home, _ := os.UserHomeDir()
	cands := []string{
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".codex", "skills"),
		filepath.Join(home, ".config", "opencode", "skills"),
		filepath.Join(home, ".cursor", "skills"),
		filepath.Join(home, ".trae", "skills"),
		filepath.Join(home, ".gemini", "antigravity", "skills"),
	}
	var out []string
	for _, c := range cands {
		if st, err := os.Stat(filepath.Dir(c)); err == nil && st.IsDir() {
			out = append(out, c)
		}
	}
	return out
}

func cmdLink(cfg *config.Config) {
	entries, err := os.ReadDir(cfg.SkillsDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	linked, skipped := 0, 0
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == "skills" {
			continue
		}
		src := filepath.Join(cfg.SkillsDir, e.Name())
		for _, t := range cfg.Targets {
			if err := os.MkdirAll(t, 0o755); err != nil {
				continue
			}
			dst := filepath.Join(t, e.Name())
			if st, err := os.Lstat(dst); err == nil {
				_ = st
				skipped++
				continue
			}
			if err := os.Symlink(src, dst); err == nil {
				linked++
			}
		}
	}
	fmt.Printf("link: %d created, %d already present\n", linked, skipped)
}

func cmdStatus(args []string) {
	cfg := loadCfg(args)
	rep, err := recon.Collect(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("# skillsync status (HEAD %s / base %s)\n", rep.Head, rep.Base)
	fmt.Printf("- ahead %d, behind %d\n\n", rep.Ahead, rep.Behind)
	if len(rep.Rows) == 0 {
		fmt.Println("clean: local and remote are in sync")
	}
	advice := map[string]string{
		recon.KindLocalOnly:  "will be committed & pushed",
		recon.KindRemoteOnly: "will be accepted",
		recon.KindLocalMod:   "kept, pushed on sync",
		recon.KindRemoteMod:  "accepted on sync",
		recon.KindBoth:       "NEEDS DECISION: resolve --keep-local/--keep-remote",
		recon.KindOneDel:     "NEEDS DECISION: confirm deletion",
	}
	fmt.Printf("%-28s %-18s %s\n", "SKILL", "KIND", "ADVICE")
	for _, r := range rep.Rows {
		fmt.Printf("%-28s %-18s %s\n", r.Skill, r.Kind, advice[r.Kind])
	}
	if len(rep.NameClash) > 0 {
		fmt.Println("\nname clashes (untracked local vs remote):", strings.Join(rep.NameClash, ", "))
	}
	for _, h := range rep.RenameHints {
		fmt.Printf("rename hint: local %q ~= remote %q (keep one)\n", h[0], h[1])
	}
	for _, i := range rep.Hygiene {
		fmt.Printf("[hygiene %s] %s: %s\n", i.Level, i.Path, i.Why)
	}
}
