package lua

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

const (
	// defaultAssetRoot is the directory a script's assets are read from,
	// relative to the process's working directory.
	//
	// There is exactly one, and it is not named by the script: a root a caller
	// could choose would make every other check here decoration.
	defaultAssetRoot = "assets"

	// defaultAssetMaxBytes caps one file. A script reads a word list or a board
	// definition; a cap is what keeps a file that grew by accident from becoming
	// a Lua string the size of the disk.
	defaultAssetMaxBytes int64 = 1 << 20 // 1 MiB
)

// allowedAssetExtensions is what a script may read, lower-cased.
//
// An allowlist rather than a deny list, and on extension rather than on sniffed
// content, because the question being answered is "did an operator mean to
// publish this to scripts", and a curated directory of text files is an answer
// an operator can check by looking.
var allowedAssetExtensions = []string{".csv", ".json", ".md", ".txt"}

// assetsConfig is everything the assets capability is bounded by.
type assetsConfig struct {
	root       string
	maxBytes   int64
	extensions []string
}

// defaultAssetsConfig is the configuration a real server's assets capability
// runs under.
func defaultAssetsConfig() assetsConfig {
	return assetsConfig{
		root:       defaultAssetRoot,
		maxBytes:   defaultAssetMaxBytes,
		extensions: allowedAssetExtensions,
	}
}

// assetsCapability builds the installer for indri.assets.
//
// The root is resolved once here rather than on every read, and a root that is
// missing or is not a directory fails the install and therefore boot. A script
// granted assets against a root that does not exist is a deployment that is
// wrong now and would otherwise only say so the first time a player triggered
// the handler that reads one.
func assetsCapability(cfg assetsConfig) capabilityInstaller {
	return func(L *lua.LState) (lua.LValue, error) {
		root, err := resolveAssetRoot(cfg.root)
		if err != nil {
			return nil, err
		}

		if cfg.maxBytes <= 0 {
			return nil, errors.New("the asset byte cap must be positive")
		}

		if len(cfg.extensions) == 0 {
			return nil, errors.New("no file extensions are allowed, so no asset could ever be read")
		}

		tbl := L.NewTable()
		tbl.RawSetString("read", L.NewFunction(func(L *lua.LState) int {
			return hostAssetsRead(L, root, cfg)
		}))

		return tbl, nil
	}
}

// hostAssetsRead is indri.assets.read(name).
//
// It returns the file's contents as a string and raises on anything else, for
// the same reason indri.mutate does: the error then carries the script's own
// file and line.
func hostAssetsRead(L *lua.LState, root string, cfg assetsConfig) int {
	name := L.CheckString(1)

	body, err := readAsset(root, name, cfg)
	if err != nil {
		L.RaiseError("indri.assets.read(%q): %s", name, err.Error())
	}

	L.Push(lua.LString(body))

	return 1
}

// resolveAssetRoot turns the configured root into the absolute, symlink-free
// path every read is measured against.
//
// The symlinks are evaluated here as well as on each file because otherwise the
// comparison would be between a resolved file path and an unresolved root, and
// a root reached through a link — which is what a temporary directory is on
// macOS, and what a container mount often is — would make every read look like
// an escape.
func resolveAssetRoot(configured string) (string, error) {
	if strings.TrimSpace(configured) == "" {
		return "", errors.New("the asset root is empty")
	}

	abs, err := filepath.Abs(configured)
	if err != nil {
		return "", fmt.Errorf("resolving the asset root %q: %w", configured, err)
	}

	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolving the asset root %q: %w", configured, err)
	}

	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("reading the asset root %q: %w", configured, err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("the asset root %q is not a directory", configured)
	}

	return root, nil
}

// readAsset resolves one name against the root and reads it, refusing anything
// that would leave the root or would not be text.
//
// The checks are ordered so each one gets to say what is wrong in its own
// terms. The lexical check comes first because it is the only one that works on
// a path that does not exist, so "../../etc/shadow" is reported as an escape
// rather than as a missing file. The extension check comes before the file is
// opened. The symlink check comes last because it needs the file to exist:
// resolving the real path is the only way to catch a name that stays inside the
// root and points outside it.
func readAsset(root, name string, cfg assetsConfig) (string, error) {
	path, err := resolveAssetPath(root, name)
	if err != nil {
		return "", err
	}

	if err := checkAssetExtension(path, cfg.extensions); err != nil {
		return "", err
	}

	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("reading the asset: %w", err)
	}

	if err := checkUnderRoot(root, real); err != nil {
		return "", err
	}

	return readCappedFile(real, cfg.maxBytes)
}

// resolveAssetPath joins name onto the root and refuses a name that climbs out
// of it.
//
// filepath.Join cleans as it goes, so "a/../../b" is already collapsed by the
// time it is compared: the comparison is on the result, never on the text the
// script supplied.
func resolveAssetPath(root, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("the asset name is empty")
	}

	if filepath.IsAbs(name) {
		return "", errors.New("an asset name is relative to the asset root and cannot be an absolute path")
	}

	path := filepath.Join(root, name)

	if err := checkUnderRoot(root, path); err != nil {
		return "", err
	}

	return path, nil
}

// checkUnderRoot refuses a path that is not the root or inside it.
//
// The separator is appended to the root before the prefix test so that a
// sibling directory whose name merely starts with the root's — "/srv/assets-old"
// against "/srv/assets" — is not read as being inside it.
func checkUnderRoot(root, path string) error {
	if path == root {
		return errors.New("an asset name must name a file inside the asset root, not the root itself")
	}

	if !strings.HasPrefix(path, root+string(filepath.Separator)) {
		return errors.New("the asset is outside the asset root")
	}

	return nil
}

// checkAssetExtension refuses a file a script may not read.
func checkAssetExtension(path string, allowed []string) error {
	ext := strings.ToLower(filepath.Ext(path))

	if !slices.Contains(allowed, ext) {
		return fmt.Errorf("refusing the file extension %q; a script may read %v", ext, allowed)
	}

	return nil
}

// readCappedFile reads a regular file and fails if it is over the cap.
//
// Read through a limit rather than checked by its size first, so a file that
// grows between the check and the read cannot get past it, and so a named pipe
// or a device that reports a size of zero cannot stream forever. Both are
// refused outright anyway; the limit is what makes that belt-and-braces rather
// than a single point of failure.
func readCappedFile(path string, maxBytes int64) (string, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("opening the asset: %w", err)
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("reading the asset: %w", err)
	}

	if !info.Mode().IsRegular() {
		return "", errors.New("the asset is not a regular file")
	}

	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("reading the asset: %w", err)
	}

	if int64(len(body)) > maxBytes {
		return "", fmt.Errorf("the asset is over the %d byte cap", maxBytes)
	}

	return string(body), nil
}
