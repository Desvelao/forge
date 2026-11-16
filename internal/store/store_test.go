package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"app/internal/gitcmd"
)

func TestClassify(t *testing.T) {
	cases := map[string]Kind{
		"https://github.com/a/b":   KindURL,
		"file:///tmp/x/remote.git": KindURL,
		"git@github.com:a/b":       KindURL,
		"ssh://git@host/a/b":       KindURL,
		"foo.git":                  KindURL,
		"./foo.git":                KindURL,
		"./tpl":                    KindPath,
		"../tpl":                   KindPath,
		"/abs/tpl":                 KindPath,
		"~/tpl":                    KindPath,
		".":                        KindPath,
		"go-service":               KindName,
		"my.template":              KindName,
	}
	for in, want := range cases {
		if got := Classify(in); got != want {
			t.Errorf("Classify(%q) = %v, want %v", in, got, want)
		}
	}
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func manifestYML(name, version string) string {
	return "schema: 1\nname: " + name + "\ndescription: d\nversion: " + version + "\n"
}

func newStore(t *testing.T) *Store {
	t.Helper()
	return &Store{Root: filepath.Join(t.TempDir(), "templates"), Git: gitcmd.New(gitcmd.ExecRunner{})}
}

func localTemplate(t *testing.T, name, version string) string {
	dir := t.TempDir()
	write(t, dir, "template.yml", manifestYML(name, version))
	write(t, dir, "template/hello.txt", "hi")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// remote creates a work repository with a bare "remote" next to it and returns both.
func remote(t *testing.T, name string) (url, work string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "t")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "t@example.com")
	}
	work = localTemplate(t, name, "1.0.0")
	bare := filepath.Join(t.TempDir(), "remote.git")
	git(t, "", "init", "-q", "--bare", "-b", "main", bare)
	git(t, work, "init", "-q", "-b", "main")
	git(t, work, "add", ".")
	git(t, work, "commit", "-qm", "init")
	git(t, work, "remote", "add", "origin", "file://"+bare)
	git(t, work, "push", "-q", "origin", "main")
	return "file://" + bare, work
}

func TestAddLocal(t *testing.T) {
	s := newStore(t)
	src := localTemplate(t, "tpl", "1.0.0")
	write(t, src, ".git/config", "ignored")
	write(t, src, "README.md", "about")

	name, err := s.Add(src, AddOptions{})
	if err != nil || name != "tpl" {
		t.Fatalf("name=%q err=%v", name, err)
	}
	dir, err := s.Path("tpl")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"template.yml", "template/hello.txt", "README.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		t.Error(".git was copied")
	}
	if es, _ := os.ReadDir(s.Root); len(es) != 1 {
		t.Errorf("leftovers in store: %v", es)
	}

	if _, err := s.Add(src, AddOptions{}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("collision err = %v", err)
	}
	write(t, src, "template/extra.txt", "x")
	if _, err := s.Add(src, AddOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "template/extra.txt")); err != nil {
		t.Error("force did not replace the template")
	}
	if es, _ := os.ReadDir(s.Root); len(es) != 1 {
		t.Errorf("leftovers in store: %v", es)
	}

	if n, err := s.Add(src, AddOptions{Name: "renamed"}); err != nil || n != "renamed" {
		t.Errorf("rename: %q %v", n, err)
	}
	if _, err := s.Add(src, AddOptions{Name: "Bad Name"}); err == nil {
		t.Error("want invalid name error")
	}
	if _, err := s.Add(src, AddOptions{Ref: "main", Name: "x"}); err == nil {
		t.Error("want error for --ref with a local directory")
	}
}

func TestAddLocalRejects(t *testing.T) {
	s := newStore(t)
	if _, err := s.Add(t.TempDir(), AddOptions{}); err == nil {
		t.Error("directory without manifest accepted")
	}
	src := localTemplate(t, "tpl", "1")
	if err := os.Symlink("/etc/passwd", filepath.Join(src, "template", "link")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err := s.Add(src, AddOptions{}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("err = %v", err)
	}
	if _, err := s.Path("tpl"); err == nil {
		t.Error("rejected template ended up in the store")
	}
	if _, err := s.Add("plain-name", AddOptions{}); err == nil {
		t.Error("bare name accepted")
	}
}

func TestGitRoundTrip(t *testing.T) {
	url, work := remote(t, "remote-tpl")
	s := newStore(t)

	name, err := s.Add(url, AddOptions{Name: "rt"})
	if err != nil || name != "rt" {
		t.Fatalf("add: %q %v", name, err)
	}
	if _, err := s.Add(url, AddOptions{Name: "rt"}); err == nil {
		t.Error("want collision error")
	}

	es, err := s.List(true)
	if err != nil || len(es) != 1 {
		t.Fatalf("list: %v %v", es, err)
	}
	if es[0].Name != "rt" || es[0].Manifest.Version != "1.0.0" || es[0].Source != url || es[0].Rev == "" || es[0].Rev == "?" {
		t.Errorf("entry = %+v", es[0])
	}

	// push a new version, then update
	write(t, work, "template.yml", manifestYML("remote-tpl", "2.0.0"))
	git(t, work, "commit", "-qam", "v2")
	git(t, work, "push", "-q", "origin", "main")
	res, err := s.Update(nil)
	if err != nil || len(res) != 1 || res[0].Status != "updated" || res[0].Err != nil {
		t.Fatalf("update: %+v %v", res, err)
	}
	if es, _ = s.List(false); es[0].Manifest.Version != "2.0.0" {
		t.Errorf("version after update = %s", es[0].Manifest.Version)
	}
	if res, _ = s.Update([]string{"rt"}); res[0].Status != "unchanged" {
		t.Errorf("second update = %+v", res[0])
	}

	if err := s.Remove("rt"); err != nil {
		t.Fatal(err)
	}
	if es, _ = s.List(false); len(es) != 0 {
		t.Errorf("not removed: %v", es)
	}
	if err := s.Remove("rt"); err == nil {
		t.Error("removing a missing template should fail")
	}
}

func TestAddPinnedAndUpdateSkips(t *testing.T) {
	url, work := remote(t, "pin")
	git(t, work, "tag", "v1")
	git(t, work, "push", "-q", "origin", "v1")
	s := newStore(t)
	if _, err := s.Add(url, AddOptions{Ref: "v1", Name: "pinned"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(localTemplate(t, "loc", "1"), AddOptions{}); err != nil {
		t.Fatal(err)
	}
	res, err := s.Update(nil)
	if err != nil || len(res) != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	for _, r := range res {
		if r.Status != "skipped" || r.Err != nil {
			t.Errorf("%+v", r)
		}
	}
	if _, err := s.Update([]string{"missing"}); err == nil {
		t.Error("want not-found error")
	}
}

func TestAddByCommit(t *testing.T) {
	url, work := remote(t, "bycommit")
	out, err := exec.Command("git", "-C", work, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	s := newStore(t)
	if _, err := s.Add(url, AddOptions{Ref: strings.TrimSpace(string(out))}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Path("bycommit"); err != nil {
		t.Error(err)
	}
}

func TestFetchLeavesNothingOnFailure(t *testing.T) {
	url, _ := remote(t, "x")
	s := newStore(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	if _, err := s.Fetch(url, "", "../escape"); err == nil {
		t.Error("want subdir error")
	}
	if _, err := s.Fetch(url, "", "nope"); err == nil {
		t.Error("want missing manifest error")
	}
	if es, _ := os.ReadDir(tmp); len(es) != 0 {
		t.Errorf("temporary files left behind: %v", es)
	}
}

func TestRemoveRejectsTraversal(t *testing.T) {
	s := newStore(t)
	if err := s.Remove("../x"); err == nil {
		t.Error("want error")
	}
}

func TestDefaultRoot(t *testing.T) {
	root, err := DefaultRoot(func(string) string { return "/custom" })
	if err != nil || root != filepath.Join("/custom", "templates") {
		t.Errorf("%q %v", root, err)
	}
}
