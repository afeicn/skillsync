package gitx

import (
	"os/exec"
	"strings"
)

type Repo struct{ Dir string }

func (r Repo) Run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, &GitError{Args: args, Out: s, Err: err}
	}
	return s, nil
}

func (r Repo) Ok(args ...string) bool {
	_, err := r.Run(args...)
	return err == nil
}

type GitError struct {
	Args []string
	Out  string
	Err  error
}

func (e *GitError) Error() string {
	return "git " + strings.Join(e.Args, " ") + ": " + e.Out
}

func (r Repo) Fetch() error              { _, err := r.Run("fetch", "origin"); return err }
func (r Repo) Head() string             { s, _ := r.Run("rev-parse", "HEAD"); return s }
func (r Repo) MergeBase(ref string) (string, error) {
	return r.Run("merge-base", "HEAD", ref)
}
func (r Repo) Count(ref string) int {
	s, _ := r.Run("rev-list", "--count", ref)
	var n int
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			n = n*10 + int(ch-'0')
		}
	}
	return n
}
func (r Repo) NameStatus(a, b string) string {
	s, _ := r.Run("diff", "--name-status", a, b)
	return s
}
func (r Repo) Porcelain() string {
	s, _ := r.Run("status", "--porcelain=v1")
	return s
}
func (r Repo) LsTree(ref string) string {
	s, _ := r.Run("ls-tree", "-r", "--name-only", ref)
	return s
}
func (r Repo) Show(revPath string) ([]byte, error) {
	cmd := exec.Command("git", "show", revPath)
	cmd.Dir = r.Dir
	return cmd.Output()
}
func (r Repo) BlobSize(revPath string) (int64, error) {
	s, err := r.Run("rev-parse", revPath)
	if err != nil {
		return 0, err
	}
	s2, err := r.Run("cat-file", "-s", s)
	if err != nil {
		return 0, err
	}
	var n int64
	for _, ch := range s2 {
		if ch >= '0' && ch <= '9' {
			n = n*10 + int64(ch-'0')
		}
	}
	return n, nil
}
func (r Repo) Unmerged() []string {
	s, _ := r.Run("diff", "--name-only", "--diff-filter=U")
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
func (r Repo) MergeNoCommit(ref string) (string, error) {
	return r.Run("merge", ref, "--no-commit", "--no-ff")
}
func (r Repo) MergeAbort()  { _, _ = r.Run("merge", "--abort") }
func (r Repo) HasMerge() bool { return r.Ok("rev-parse", "--verify", "MERGE_HEAD") }
func (r Repo) StagedClean() bool {
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = r.Dir
	return cmd.Run() == nil
}
