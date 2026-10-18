package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The list built in is what a magpie that can't reach the market shows:
// every entry a real package name, said in English and Chinese, once.
func TestBuiltinMarket(t *testing.T) {
	l, err := parseMarket(builtinMarket)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, x := range l {
		if seen[x.Package] {
			t.Errorf("%s listed twice", x.Package)
		}
		seen[x.Package] = true
		if x.Summary["en"] == "" || x.Summary["zh"] == "" {
			t.Errorf("%s: summary needs en and zh", x.Package)
		}
		if !x.Community {
			t.Errorf("%s: only magpie's community's plugins are listed", x.Package)
		}
		// gateway middleware signs in to nothing
		if x.Kind == "middleware" {
			if len(x.Providers) != 0 || x.Replaces != "" {
				t.Errorf("%s: middleware with providers", x.Package)
			}
		} else if len(x.Providers) == 0 || x.Icon == "" {
			t.Errorf("%s: needs providers and an icon", x.Package)
		}
		if x.Kind != "" && x.Kind != "middleware" {
			t.Errorf("%s: kind %q", x.Package, x.Kind)
		}
	}
	if len(l) < 10 {
		t.Errorf("%d listed", len(l))
	}
}

// The market is fetched, the built-in list's plugins kept beside it; one
// that fails is the copy fetched last, and with none, the one built in.
// "off" never asks.
func TestMarketSources(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	reset := func() { marketMu.Lock(); marketList = nil; marketMu.Unlock() }
	up := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up {
			http.Error(w, "down", 500)
			return
		}
		w.Write([]byte(`{"version":1,"plugins":[{"package":"opencode-x-auth","name":"X","community":true},{"package":"not a name","community":true},{"package":"opencode-other-auth","name":"Other"}]}`))
	}))
	defer srv.Close()
	t.Setenv("MAGPIE_PLUGIN_MARKET", srv.URL)
	ctx := context.Background()

	builtin, _ := parseMarket(builtinMarket)
	has := func(l []Listing, pkg string) bool {
		for _, x := range l {
			if x.Package == pkg {
				return true
			}
		}
		return false
	}
	reset()
	l := Market(ctx)
	if !has(l, "opencode-x-auth") || len(l) != len(builtin)+1 {
		t.Fatalf("fetched, builtin kept beside it: %+v", l)
	}
	up = false
	reset()
	if l := Market(ctx); !has(l, "opencode-x-auth") || len(l) != len(builtin)+1 {
		t.Fatalf("down, the copy kept: %+v", l)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	reset()
	if l := Market(ctx); len(l) != len(builtin) {
		t.Fatalf("down, nothing kept: %d, want the %d built in", len(l), len(builtin))
	}
	up = true
	t.Setenv("MAGPIE_PLUGIN_MARKET", "off")
	reset()
	if l := Market(ctx); len(l) != len(builtin) {
		t.Fatalf("off: %d", len(l))
	}
	reset()
}

// A plugin's provider shows its vendor's icon: the one its package is
// listed with, else that of a listed plugin for the same provider.
func TestMarketIcon(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	marketMu.Lock()
	marketList = nil
	marketMu.Unlock()
	for _, c := range []struct{ spec, id, want string }{
		{"@cognitionai/opencode-devin@0.3.1", "devin", "devin"},
		{"opencode-gemini-auth@2.0.1", "google", "gemini-color"},
		{"opencode-copilot-auth", "github-copilot", "githubcopilot"},
		{"some-unlisted-plugin", "anthropic", "claude-color"},
		{"opencode-antigravity-auth", "google", "antigravity-color"},
		{"/Users/me/plugins/packages/zed", "zed", "zed"},
		{"some-unlisted-plugin", "commandcode-plan", "commandcode"},
		{"some-unlisted-plugin", "nobody", ""},
	} {
		if got := Icon(c.spec, c.id); got != c.want {
			t.Errorf("Icon(%q, %q) = %q, want %q", c.spec, c.id, got, c.want)
		}
	}
}

// A plugin added from a folder is read from that folder, where npm has
// nothing to say of it: README.md whatever its case, else a markdown or
// text one beside it, and a folder with none says so.
func TestReadmeFromFolder(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := Readme(ctx, dir); err == nil {
		t.Fatal("a folder with no README answered")
	}
	if _, err := Readme(ctx, filepath.Join(dir, "not-there")); err == nil {
		t.Fatal("a folder that isn't there answered")
	}

	// the lesser names are there first: the README.md is still the one read
	write("README.txt", "a text one")
	write("README", "an unsuffixed one")
	write("readme.md", "# the plugin's own\n\nInstall it.\n")
	p, err := Readme(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.Readme, "# the plugin's own") {
		t.Errorf("read %q, want the README.md", p.Readme)
	}
	if p.Updated.IsZero() {
		t.Error("no modified time")
	}

	// whatever case the folder spells it in, and through file://
	if err := os.Remove(filepath.Join(dir, "readme.md")); err != nil {
		t.Fatal(err)
	}
	write("ReadMe.MD", "# spelled otherwise\n")
	for _, spec := range []string{dir, "file://" + dir, filepath.Join(dir, ".")} {
		p, err := Readme(ctx, spec)
		if err != nil {
			t.Fatalf("%s: %v", spec, err)
		}
		if !strings.HasPrefix(p.Readme, "# spelled otherwise") {
			t.Errorf("%s: read %q, want the ReadMe.MD", spec, p.Readme)
		}
	}

	// the text one, once there is no markdown at all
	if err := os.Remove(filepath.Join(dir, "ReadMe.MD")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "README")); err != nil {
		t.Fatal(err)
	}
	p, err = Readme(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Readme != "a text one" {
		t.Errorf("read %q, want the README.txt", p.Readme)
	}
}

func TestMarketTakesGitSpecs(t *testing.T) {
	l, err := parseMarket([]byte(`{"plugins":[
		{"package":"Jindequan/magpie-plugin-bailian-free","name":"Git","community":true,"summary":{"en":"e","zh":"z"}},
		{"package":"github:owner/repo","name":"Git2","community":true,"summary":{"en":"e","zh":"z"}},
		{"package":"someone/else","name":"No community","summary":{"en":"e","zh":"z"}},
		{"package":"Bad Name","community":true}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(l) != 2 || l[0].Package != "Jindequan/magpie-plugin-bailian-free" || l[1].Package != "github:owner/repo" {
		t.Fatalf("kept %v", l)
	}
}

func TestMarketKeepsBuiltinBesideFetched(t *testing.T) {
	fetched, err := parseMarket([]byte(`{"plugins":[{"package":"@magpie-community/opencode-zen-free-auth","name":"Zen","community":true,"summary":{"en":"e","zh":"z"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	builtin, err := parseMarket(builtinMarket)
	if err != nil {
		t.Fatal(err)
	}
	merged := mergeBuiltin(fetched)
	if len(merged) != len(builtin) {
		t.Fatalf("merged %d, builtin %d: a fetched list must keep every builtin entry", len(merged), len(builtin))
	}
	seen := map[string]int{}
	for _, x := range merged {
		seen[x.Package]++
	}
	for p, n := range seen {
		if n > 1 {
			t.Errorf("%s listed twice", p)
		}
	}
	if seen["@magpie-community/opencode-zen-free-auth"] != 1 {
		t.Error("the fetched entry itself missing")
	}
}
