package lua

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
)

// assetRoot builds a curated directory holding the named files, and returns it.
//
// A real directory rather than a fake filesystem, because two of the things
// being tested — a symlink that leaves the root, and a root that is itself
// reached through one, which is what a temporary directory is on macOS — exist
// only on a real one.
func assetRoot(t *testing.T, files map[string]string) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), "assets")

	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatalf("creating the asset root: %v", err)
	}

	for name, body := range files {
		path := filepath.Join(root, name)

		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("creating %v: %v", filepath.Dir(path), err)
		}

		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("writing %v: %v", path, err)
		}
	}

	return root
}

// testAssetsConfig is the shipped configuration rooted at a test's own
// directory. Only the root moves: the extension allowlist and the byte cap a
// real server runs under are what these tests measure.
func testAssetsConfig(root string) assetsConfig {
	cfg := defaultAssetsConfig()
	cfg.root = root

	return cfg
}

// readThrough resolves the root the way the installer does and then reads one
// name through it, which is what a script's call amounts to.
func readThrough(t *testing.T, cfg assetsConfig, name string) (string, error) {
	t.Helper()

	root, err := resolveAssetRoot(cfg.root)
	if err != nil {
		t.Fatalf("resolving the asset root %q: %v", cfg.root, err)
	}

	return readAsset(root, name, cfg)
}

// TestAssetsRead_ReturnsACuratedFile is the happy path, including the one a
// naive prefix test gets wrong: a file in a subdirectory of the root is inside
// the root.
func TestAssetsRead_ReturnsACuratedFile(t *testing.T) {
	t.Parallel()

	root := assetRoot(t, map[string]string{
		"words.txt":        "otter\nbadger\n",
		"board/tiles.json": `{"tiles":3}`,
	})

	for name, want := range map[string]string{
		"words.txt":          "otter\nbadger\n",
		"board/tiles.json":   `{"tiles":3}`,
		"./words.txt":        "otter\nbadger\n",
		"board/../words.txt": "otter\nbadger\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := readThrough(t, testAssetsConfig(root), name)
			if err != nil {
				t.Fatalf("reading %q: %v", name, err)
			}

			if got != want {
				t.Fatalf("reading %q gave %q, want %q", name, got, want)
			}
		})
	}
}

// TestAssetsRead_RefusesAnEscape is the whole point of a curated root.
//
// The two escapes are different mechanisms and one check does not catch both. A
// parent-directory name is visible in the text and is collapsed before anything
// is opened; a symlink is a name that stays inside the root and a file that does
// not, so only resolving the real path finds it.
func TestAssetsRead_RefusesAnEscape(t *testing.T) {
	t.Parallel()

	root := assetRoot(t, map[string]string{"words.txt": "otter"})

	outside := filepath.Join(filepath.Dir(root), "secret.txt")
	if err := os.WriteFile(outside, []byte("the private data"), 0o600); err != nil {
		t.Fatalf("writing %v: %v", outside, err)
	}

	if err := os.Symlink(outside, filepath.Join(root, "link.txt")); err != nil {
		t.Fatalf("linking to %v: %v", outside, err)
	}

	if err := os.Symlink(filepath.Dir(root), filepath.Join(root, "up")); err != nil {
		t.Fatalf("linking to %v: %v", filepath.Dir(root), err)
	}

	tests := map[string]string{
		"a parent-directory name":          "../secret.txt",
		"a parent-directory name, buried":  "board/../../secret.txt",
		"an absolute path":                 outside,
		"a symlink to a file outside":      "link.txt",
		"a symlink to a directory outside": "up/secret.txt",
		"the root itself":                  ".",
	}

	for name, asset := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body, err := readThrough(t, testAssetsConfig(root), asset)
			if err == nil {
				t.Fatalf("reading %q returned %q, want it refused", asset, body)
			}

			if strings.Contains(body, "private") {
				t.Fatalf("reading %q leaked %q", asset, body)
			}
		})
	}
}

// TestAssetsRead_AllowsTextExtensionsOnly keeps the capability to files an
// operator meant to publish. The check is on the extension, and it is made
// before the file is opened.
func TestAssetsRead_AllowsTextExtensionsOnly(t *testing.T) {
	t.Parallel()

	root := assetRoot(t, map[string]string{
		"words.txt":  "otter",
		"tiles.json": "{}",
		"rules.md":   "# rules",
		"scores.csv": "a,b",
		"logo.png":   "not really a png",
		"game.lua":   "return 1",
		"secrets":    "no extension at all",
	})

	tests := map[string]bool{
		"words.txt":  true,
		"tiles.json": true,
		"rules.md":   true,
		"scores.csv": true,
		"logo.png":   false,
		"game.lua":   false,
		"secrets":    false,
	}

	for name, allowed := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := readThrough(t, testAssetsConfig(root), name)

			switch {
			case allowed && err != nil:
				t.Fatalf("reading %q: %v", name, err)
			case !allowed && err == nil:
				t.Fatalf("reading %q was allowed, want the extension refused", name)
			case !allowed:
				requireErrorMentions(t, err, "refusing the file extension")
			}
		})
	}
}

// TestAssetsRead_TreatsTheByteCapAsAnError matches the http capability's cap for
// the same reason: half a word list is a word list that parses and is wrong.
func TestAssetsRead_TreatsTheByteCapAsAnError(t *testing.T) {
	t.Parallel()

	const bodyCap = 16

	tests := map[string]struct {
		size    int
		refused bool
	}{
		"under the cap":   {size: bodyCap - 1},
		"exactly the cap": {size: bodyCap},
		"one byte over":   {size: bodyCap + 1, refused: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body := strings.Repeat("x", test.size)
			root := assetRoot(t, map[string]string{"words.txt": body})

			cfg := testAssetsConfig(root)
			cfg.maxBytes = bodyCap

			got, err := readThrough(t, cfg, "words.txt")

			if test.refused {
				requireErrorMentions(t, err, "over the 16 byte cap")

				return
			}

			if err != nil {
				t.Fatalf("reading a %d byte file under a %d byte cap: %v", test.size, bodyCap, err)
			}

			if got != body {
				t.Fatalf("the file read back as %d bytes, want %d", len(got), test.size)
			}
		})
	}
}

// TestAssetsRead_RefusesWhatIsNotAFile keeps a directory, and anything else
// that is not an ordinary file, from being read as one.
func TestAssetsRead_RefusesWhatIsNotAFile(t *testing.T) {
	t.Parallel()

	root := assetRoot(t, map[string]string{"pages.txt/index.txt": "a directory named like a file"})

	_, err := readThrough(t, testAssetsConfig(root), "pages.txt")
	requireErrorMentions(t, err, "not a regular file")
}

// TestAssetsRead_RefusesAMissingFile reports the ordinary mistake as itself,
// rather than as an escape.
func TestAssetsRead_RefusesAMissingFile(t *testing.T) {
	t.Parallel()

	root := assetRoot(t, map[string]string{"words.txt": "otter"})

	_, err := readThrough(t, testAssetsConfig(root), "missing.txt")
	requireErrorMentions(t, err, "missing.txt")
}

// TestAssetsCapability_RefusesAnUnusableRoot fails the install, and so boot: a
// script granted assets against a root that is not there is a deployment that is
// already wrong, and saying so at boot is cheaper than saying so on a player's
// first move.
func TestAssetsCapability_RefusesAnUnusableRoot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	file := filepath.Join(dir, "not-a-directory.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("writing %v: %v", file, err)
	}

	tests := map[string]struct {
		root  string
		wants []string
	}{
		"a root that does not exist": {root: filepath.Join(dir, "absent"), wants: []string{"resolving the asset root"}},
		"a root that is a file":      {root: file, wants: []string{"not a directory"}},
		"no root at all":             {root: "   ", wants: []string{"asset root is empty"}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cfg := defaultAssetsConfig()
			cfg.root = test.root

			_, err := newAssetsEngine(t, cfg, `indri.on("noop", function(req) end)`)
			requireErrorMentions(t, err, append(test.wants, CapabilityAssets)...)
		})
	}
}

// newAssetsEngine builds an engine over one script that was granted assets,
// with assets as the only capability that exists.
func newAssetsEngine(t *testing.T, cfg assetsConfig, src string) (*Engine, error) {
	t.Helper()

	path := writeScript(t, "game.lua", src)

	e, err := newEngine(
		[]models.ScriptFile{granted(path, CapabilityAssets)},
		nil,
		capabilitySet{CapabilityAssets: assetsCapability(cfg)},
	)

	if e != nil {
		t.Cleanup(e.Close)
	}

	return e, err
}

// TestAssetsCapability_IsReachableFromAnActionHandler is the difference between
// assets and http, and the reason they are separate grants.
//
// Reading a curated file does not block on anything outside the process, so
// there is no lock to hold and no retry to pay for: a script may do it on a
// player's request path, and the refusal it gets back for a bad name is the
// script's own error rather than a missing capability.
func TestAssetsCapability_IsReachableFromAnActionHandler(t *testing.T) {
	t.Parallel()

	root := assetRoot(t, map[string]string{"words.txt": "otter"})

	e, err := newAssetsEngine(t, testAssetsConfig(root), `
indri.on("move", function(req)
  assert(indri.assets.read("words.txt") == "otter", "the curated file did not read back")

  local ok, err = pcall(function() indri.assets.read("../secret.txt") end)

  assert(not ok, "an escape was allowed")
  assert(string.find(err, "outside the asset root", 1, true), "the error was " .. tostring(err))
end)
`)
	if err != nil {
		t.Fatalf("building an engine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if _, err := splitScriptError(e.Invoke(ctx, "move", actions.Request{})); err != nil {
		t.Fatalf("invoking a handler that reads an asset: %v", err)
	}
}
