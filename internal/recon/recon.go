package recon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/afeicn/skillsync/internal/config"
	"github.com/afeicn/skillsync/internal/gitx"
	"github.com/afeicn/skillsync/internal/notify"
)

var secretPats = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`ghp_[0-9A-Za-z]{20,}`),
	regexp.MustCompile(`gho_[0-9A-Za-z]{20,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`(?i)(api[_-]?key|auth_code|secret)\s*=\s*['"][^'"]{8,}`),
}

const (
	KindLocalOnly    = "local-only"
	KindRemoteOnly   = "remote-only"
	KindLocalMod     = "local-modified"
	KindRemoteMod    = "remote-modified"
	KindBoth         = "both-modified"
	KindOneDel       = "deleted-one-side"
	warnBytes        = 2 * 1024 * 1024
	failBytes        = 10 * 1024 * 1024
)

type Row struct {
	Skill  string
	Kind   string
	Local  []string
	Remote []string
}

type Issue struct {
	Path  string
	Level string
	Why   string
}

type Report struct {
	Head, Base  string
	Ahead, Behind int
	Rows        []Row
	NameClash   []string
	RenameHints [][2]string
	Hygiene     []Issue
}

func topName(p string) string {
	if i := strings.Index(p, "/"); i >= 0 {
		return p[:i]
	}
	return "(root)"
}

func parseNameStatus(out string) map[string]map[string]bool {
	d := map[string]map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		letter := string(parts[0][0])
		var paths []string
		if strings.HasPrefix(parts[0], "R") {
			paths = parts[1:]
		} else if len(parts) > 1 {
			paths = []string{parts[1]}
		}
		for _, p := range paths {
			s := topName(p)
			if d[s] == nil {
				d[s] = map[string]bool{}
			}
			d[s][letter] = true
		}
	}
	return d
}

func keys(m map[string]bool) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func blobHash(g gitx.Repo, revPath string) string {
	data, err := g.Show(revPath)
	if err != nil || len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}

func fileHash(p string) string {
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}

func Collect(cfg *config.Config) (*Report, error) {
	g := gitx.Repo{Dir: cfg.SkillsDir}
	ref := "origin/" + cfg.Branch
	_ = g.Fetch()
	head := g.Head()
	base, err := g.MergeBase(ref)
	hasBase := err == nil
	ahead := g.Count(ref + "..HEAD")
	behind := g.Count("HEAD.." + ref)

	localDiff, remoteDiff := map[string]map[string]bool{}, map[string]map[string]bool{}
	if hasBase {
		localDiff = parseNameStatus(g.NameStatus(base, "HEAD"))
		remoteDiff = parseNameStatus(g.NameStatus(base, ref))
	}

	uncommitted := map[string]map[string]bool{}
	dirUntracked := map[string]bool{}
	for _, line := range strings.Split(g.Porcelain(), "\n") {
		if len(line) < 4 {
			continue
		}
		xy, p := line[:2], strings.Trim(strings.TrimSpace(line[3:]), `"`)
		if strings.TrimSpace(xy) == "" {
			continue
		}
		tag := "UNT"
		if xy != "??" {
			tag = strings.TrimSpace(xy)
		}
		if uncommitted[topName(p)] == nil {
			uncommitted[topName(p)] = map[string]bool{}
		}
		uncommitted[topName(p)][tag] = true
		if xy == "??" && (p == topName(p)+"/" || p == topName(p)) {
			dirUntracked[topName(p)] = true
		}
	}
	for s, tags := range uncommitted {
		if localDiff[s] == nil {
			localDiff[s] = map[string]bool{}
		}
		for t := range tags {
			localDiff[s][t] = true
		}
	}

	remoteNames := map[string]bool{}
	for _, p := range strings.Split(g.LsTree(ref), "\n") {
		if p = strings.TrimSpace(p); p != "" {
			remoteNames[topName(p)] = true
		}
	}

	all := map[string]bool{}
	for s := range localDiff {
		all[s] = true
	}
	for s := range remoteDiff {
		all[s] = true
	}
	rep := &Report{Head: head[:min(8, len(head))], Ahead: ahead, Behind: behind}
	if hasBase {
		rep.Base = base[:min(8, len(base))]
	}
	for _, s := range sortedKeys(all) {
		l, r := localDiff[s], remoteDiff[s]
		kind := ""
		switch {
		case len(l) > 0 && len(r) > 0:
			if l["D"] || r["D"] {
				kind = KindOneDel
			} else {
				kind = KindBoth
			}
		case len(l) > 0:
			if !remoteNames[s] {
				kind = KindLocalOnly
			} else {
				kind = KindLocalMod
			}
		default:
			if r["A"] {
				kind = KindRemoteOnly
			} else {
				kind = KindRemoteMod
			}
		}
		rep.Rows = append(rep.Rows, Row{Skill: s, Kind: kind, Local: keys(l), Remote: keys(r)})
	}

	for s := range dirUntracked {
		if remoteNames[s] {
			rep.NameClash = append(rep.NameClash, s)
		}
	}

	lh := map[string]string{}
	rh := map[string]string{}
	for _, s := range sortedKeys(all) {
		if localDiff[s] != nil && remoteDiff[s] == nil && subset(localDiff[s], "A", "UNT") {
			if h := fileHash(filepath.Join(cfg.SkillsDir, s, "SKILL.md")); h != "" {
				lh[h] = s
			}
		}
		if remoteDiff[s] != nil && localDiff[s] == nil && subset(remoteDiff[s], "A") {
			if h := blobHash(g, ref+":"+s+"/SKILL.md"); h != "" {
				rh[h] = s
			}
		}
	}
	for h, l := range lh {
		if r, ok := rh[h]; ok && r != l {
			rep.RenameHints = append(rep.RenameHints, [2]string{l, r})
		}
	}

	if hasBase {
		rep.Hygiene = hygiene(g, base, ref, cfg)
	}
	return rep, nil
}

func subset(m map[string]bool, ok ...string) bool {
	if len(m) == 0 {
		return false
	}
	allow := map[string]bool{}
	for _, o := range ok {
		allow[o] = true
	}
	for k := range m {
		if !allow[k] {
			return false
		}
	}
	return true
}

func sortedKeys(m map[string]bool) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	for i := 0; i < len(ks); i++ {
		for j := i + 1; j < len(ks); j++ {
			if ks[j] < ks[i] {
				ks[i], ks[j] = ks[j], ks[i]
			}
		}
	}
	return ks
}

func hygiene(g gitx.Repo, base, ref string, cfg *config.Config) []Issue {
	var issues []Issue
	seen := map[string]bool{}
	for _, p := range strings.Split(g.NameStatus(base, ref), "\n") {
		parts := strings.Split(strings.TrimSpace(p), "\t")
		if len(parts) < 2 {
			continue
		}
		for _, f := range parts[1:] {
			if f = strings.TrimSpace(f); f != "" && !seen[f] {
				seen[f] = true
				issues = append(issues, checkFile(g, ref+":"+f, f, cfg))
			}
		}
	}
	var keep []Issue
	for _, i := range issues {
		if i.Why != "" {
			keep = append(keep, i)
		}
	}
	return keep
}

func checkFile(g gitx.Repo, revPath, path string, cfg *config.Config) Issue {
	for _, b := range cfg.Ignore {
		if strings.Contains(path, b) {
			return Issue{Path: path, Level: "fail", Why: "ignored path must not be committed"}
		}
	}
	size, err := g.BlobSize(revPath)
	if err != nil {
		return Issue{}
	}
	limit := int64(cfg.MaxFileMB) * 1024 * 1024
	if size >= limit {
		return Issue{Path: path, Level: "fail", Why: fmt.Sprintf("%d bytes exceeds limit", size)}
	}
	if size >= warnBytes {
		return Issue{Path: path, Level: "warn", Why: fmt.Sprintf("%d bytes is large", size)}
	}
	data, err := g.Show(revPath)
	if err == nil {
		text := string(data)
		for _, re := range secretPats {
			if re.MatchString(text) {
				return Issue{Path: path, Level: "fail", Why: "possible secret"}
			}
		}
	}
	return Issue{}
}

// Verify checks working-tree + staged changes before commit.
func Verify(cfg *config.Config) []Issue {
	g := gitx.Repo{Dir: cfg.SkillsDir}
	touched := map[string]bool{}
	staged, _ := g.Run("diff", "--cached", "--name-only")
	for _, l := range strings.Split(staged, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			touched[l] = true
		}
	}
	for _, l := range strings.Split(g.Porcelain(), "\n") {
		if len(l) > 3 {
			touched[strings.Trim(strings.TrimSpace(l[3:]), `"`)] = true
		}
	}
	var issues []Issue
	for _, p := range sortedKeys(touched) {
		for _, b := range cfg.Ignore {
			if strings.Contains(p, b) {
				issues = append(issues, Issue{Path: p, Level: "fail", Why: "ignored path"})
			}
		}
		if i := strings.Index(p, "/"); i > 0 {
			d := p[:i]
			sm := filepath.Join(cfg.SkillsDir, d, "SKILL.md")
			if st, err := os.Stat(filepath.Join(cfg.SkillsDir, d)); err == nil && st.IsDir() {
				if _, err := os.Stat(sm); err != nil {
					issues = append(issues, Issue{Path: d + "/", Level: "fail", Why: "missing SKILL.md"})
					continue
				}
				if text, err := os.ReadFile(sm); err == nil {
					m := regexp.MustCompile(`(?m)^name:\s*["']?(.+?)["']?\s*$`).FindSubmatch(text)
					if m != nil && string(m[1]) != d {
						issues = append(issues, Issue{Path: d + "/", Level: "warn", Why: "frontmatter name != dir name"})
					}
				}
			}
		}
		if fp := filepath.Join(cfg.SkillsDir, p); fileExists(fp) {
			st, _ := os.Stat(fp)
			limit := int64(cfg.MaxFileMB) * 1024 * 1024
			if st.Size() >= limit {
				issues = append(issues, Issue{Path: p, Level: "fail", Why: "file too large"})
			} else if st.Size() < 500_000 {
				if data, err := os.ReadFile(fp); err == nil {
					for _, re := range secretPats {
						if re.MatchString(string(data)) {
							issues = append(issues, Issue{Path: p, Level: "fail", Why: "possible secret"})
							break
						}
					}
				}
			}
		}
	}
	return issues
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func manualKinds(r *Report) []Row {
	var out []Row
	for _, x := range r.Rows {
		if x.Kind == KindBoth || x.Kind == KindOneDel {
			out = append(out, x)
		}
	}
	return out
}

func writePending(cfg *config.Config, r *Report, reason string) string {
	name := filepath.Join(config.StateDir(),
		fmt.Sprintf("pending-%s.md", time.Now().Format("20060102-150405")))
	var b strings.Builder
	fmt.Fprintf(&b, "# skillsync needs a human (%s)\n\n- repo: %s\n- ahead %d / behind %d\n\n",
		reason, cfg.SkillsDir, r.Ahead, r.Behind)
	for _, x := range manualKinds(r) {
		fmt.Fprintf(&b, "- [%s] %s (local %v / remote %v)\n", x.Kind, x.Skill, x.Local, x.Remote)
	}
	for _, s := range r.NameClash {
		fmt.Fprintf(&b, "- [name clash] %s\n", s)
	}
	for _, h := range r.Hygiene {
		if h.Level == "fail" {
			fmt.Fprintf(&b, "- [hygiene] %s: %s\n", h.Path, h.Why)
		}
	}
	b.WriteString("\nFix: run `skillsync status`, then `skillsync resolve --keep-local A --keep-remote B`, `skillsync verify`, commit & push.\n")
	_ = os.WriteFile(name, []byte(b.String()), 0o644)
	return name
}

func logLine(msg string) {
	line := time.Now().Format("2006-01-02 15:04:05") + " " + msg
	fmt.Println(line)
	if f, err := os.OpenFile(filepath.Join(config.StateDir(), "skillsync.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintln(f, line)
		f.Close()
	}
}

func PreSync(cfg *config.Config) (string, bool) { return preSyncCheck(cfg) }

// preSyncCheck runs the repo's pre-sync gate (PII check etc).
// Config pre_sync_cmd wins; otherwise auto-detects scripts/pii-check.{sh,py}.
func preSyncCheck(cfg *config.Config) (string, bool) {
	cmdline := cfg.PreSyncCmd
	if cmdline == "" {
		for _, cand := range []string{"scripts/pii-check.sh", "scripts/pii_check.py", "scripts/pii-check.py"} {
			if fileExists(filepath.Join(cfg.SkillsDir, cand)) {
				cmdline = cand
				break
			}
		}
	}
	if cmdline == "" {
		return "", true
	}
	var cmd *exec.Cmd
	switch {
	case strings.HasSuffix(cmdline, ".sh"):
		cmd = exec.Command("sh", cmdline)
	case strings.HasSuffix(cmdline, ".py"):
		cmd = exec.Command("python3", cmdline)
	default:
		shell := "/bin/sh"
		if runtime.GOOS == "windows" {
			shell = "cmd"
		}
		cmd = exec.Command(shell, cmdline)
	}
	cmd.Dir = cfg.SkillsDir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err == nil
}

// Auto: snapshot local, reconcile, merge safe changes, push. Returns exit code.
func Auto(cfg *config.Config) int {
	g := gitx.Repo{Dir: cfg.SkillsDir}
	lock := filepath.Join(os.TempDir(), "skillsync.lock")
	if err := os.Mkdir(lock, 0o755); err != nil {
		if st, err2 := os.Stat(lock); err2 == nil && time.Since(st.ModTime()) > 30*time.Minute {
			_ = os.RemoveAll(lock)
			if err := os.Mkdir(lock, 0o755); err != nil {
				logLine("auto: lock contention, skip")
				return 0
			}
		} else {
			logLine("auto: previous run still active, skip")
			return 0
		}
	}
	defer os.RemoveAll(lock)

	_ = g.Fetch()
	if strings.TrimSpace(g.Porcelain()) != "" {
		_, _ = g.Run("add", "-A")
		if fails := Verify(cfg); hasFail(fails) {
			_, _ = g.Run("reset")
			p := writePending(cfg, &Report{}, "local snapshot failed verify")
			logLine("auto: local snapshot rejected by verify, see " + p)
			notify.Send("skillsync", "local changes failed checks, needs attention")
			return 1
		}
		ts := time.Now().Format("2006-01-02 15:04")
		if _, err := g.Run("commit", "-m", "chore(auto-sync): local snapshot "+ts); err != nil {
			logLine("auto: snapshot commit failed: " + err.Error())
			return 1
		}
		logLine("auto: local changes snapshotted")
		if out, ok := preSyncCheck(cfg); !ok {
			p := writePending(cfg, &Report{}, "pre-sync gate failed after snapshot")
			logLine("auto: pre-sync gate FAILED, not pushing. Output:\n" + out + "\nReport: " + p)
			notify.Send("skillsync", "pre-sync check failed, push blocked")
			return 1
		}
	}

	rep, err := Collect(cfg)
	if err != nil {
		logLine("auto: collect failed: " + err.Error())
		return 1
	}
	ref := "origin/" + cfg.Branch
	if len(manualKinds(rep)) > 0 || len(rep.NameClash) > 0 || hasFail(rep.Hygiene) {
		p := writePending(cfg, rep, "both-modified / deletion / clash / hygiene")
		logLine("auto: needs human decision, stopped. Report: " + p)
		notify.Send("skillsync", "sync conflict, needs reconciliation")
		return 2
	}
	if rep.Ahead == 0 && rep.Behind == 0 {
		logLine("auto: up to date, nothing to do")
		return 0
	}
	if _, err := g.MergeNoCommit(ref); err != nil {
		g.MergeAbort()
		p := writePending(cfg, rep, "merge conflict (race)")
		logLine("auto: merge conflict, rolled back. Report: " + p)
		notify.Send("skillsync", "sync conflict, needs reconciliation")
		return 2
	}
	if fails := Verify(cfg); hasFail(fails) {
		if g.HasMerge() {
			g.MergeAbort()
		}
		logLine("auto: merged tree failed verify, rolled back")
		notify.Send("skillsync", "merge result failed checks")
		return 1
	}
	if out, ok := preSyncCheck(cfg); !ok {
		if g.HasMerge() {
			g.MergeAbort()
		}
		p := writePending(cfg, rep, "pre-sync gate failed after merge")
		logLine("auto: pre-sync gate FAILED after merge, rolled back. Output:\n" + out + "\nReport: " + p)
		notify.Send("skillsync", "pre-sync check failed, merge rolled back")
		return 1
	}
	if g.HasMerge() || !g.StagedClean() {
		ts := time.Now().Format("2006-01-02 15:04")
		if _, err := g.Run("commit", "-m", "chore(auto-sync): merge "+ref+" "+ts); err != nil {
			logLine("auto: commit failed: " + err.Error())
			return 1
		}
		logLine("auto: merge committed")
	}
	if _, err := g.Run("push", "origin", cfg.Branch); err != nil {
		logLine("auto: push failed (merged locally, will retry next run): " + err.Error())
		return 1
	}
	logLine("auto: synced and pushed")
	return 0
}

func hasFail(issues []Issue) bool {
	for _, i := range issues {
		if i.Level == "fail" {
			return true
		}
	}
	return false
}

func Resolve(cfg *config.Config, keepLocal, keepRemote []string) int {
	g := gitx.Repo{Dir: cfg.SkillsDir}
	lset, rset := toSet(keepLocal), toSet(keepRemote)
	for s := range lset {
		if rset[s] {
			fmt.Printf("skill %q cannot be kept on both sides\n", s)
			return 1
		}
	}
	var rest []string
	for _, x := range g.Unmerged() {
		s := topName(x)
		switch {
		case lset[s]:
			_, _ = g.Run("checkout", "--ours", "--", x)
			_, _ = g.Run("add", "--", x)
			fmt.Println("keep local:", x)
		case rset[s]:
			_, _ = g.Run("checkout", "--theirs", "--", x)
			_, _ = g.Run("add", "--", x)
			fmt.Println("keep remote:", x)
		default:
			rest = append(rest, x)
		}
	}
	if len(rest) > 0 {
		fmt.Println("still needs manual merge:")
		for _, x := range rest {
			fmt.Println(" -", x)
		}
		return 2
	}
	fmt.Println("conflicts cleared; run verify then commit & push")
	return 0
}

func toSet(xs []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		if x = strings.TrimSpace(x); x != "" {
			m[x] = true
		}
	}
	return m
}

func min(a, b int) int { if a < b { return a }; return b }
