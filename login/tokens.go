package login

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// TokenFileName is the raw-token file inside agx's config dir. It lives
// apart from config.yaml so the config stays safe to sync or share.
const TokenFileName = "tokens.yaml"

// File modes, mirroring ssh's rule for private keys: the token file is
// private to the user, and agx refuses one anybody else can read.
const (
	filePerm os.FileMode = 0o600
	dirPerm  os.FileMode = 0o700
	// otherBits are the group/other permission bits that make the file
	// readable or writable by someone else.
	otherBits os.FileMode = 0o077
)

// tokenFileHeader is written above the entries on every save.
const tokenFileHeader = "# agx raw login tokens — written by `agx login add`; mode 0600.\n# Never sync, back up to a shared place, or commit this file.\n"

// ErrInsecure means the token file can be read by group or others.
var ErrInsecure = errors.New("login: token file is readable by others")

// TokenFile is the private name → raw token store. The zero value has no
// path and reads as empty.
type TokenFile struct {
	Path string
}

// CheckPerm returns ErrInsecure (with the chmod that fixes it) when the file
// exists and group or others have any access; nil when it is private or
// absent.
func (t TokenFile) CheckPerm() error {
	info, err := os.Stat(t.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if info.Mode().Perm()&otherBits != 0 {
		return fmt.Errorf("%w: %s is mode %04o — fix with: chmod 600 %s", ErrInsecure, t.Path, info.Mode().Perm(), t.Path)
	}
	return nil
}

// Load reads every stored token. A missing file is empty, not an error; a
// file others can read is refused before it is opened.
func (t TokenFile) Load() (map[string]string, error) {
	out := map[string]string{}
	if t.Path == "" {
		return out, nil
	}
	if err := t.CheckPerm(); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(t.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return out, nil
	}
	// The decode error is replaced, not wrapped: yaml.v3 quotes the
	// offending line, which here would be a token.
	if err := yaml.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("login: %s is not a name: token map", t.Path)
	}
	return out, nil
}

// Set stores value under name, replacing any previous value.
func (t TokenFile) Set(name, value string) error {
	all, err := t.Load()
	if err != nil {
		return err
	}
	all[name] = value
	return t.save(all)
}

// Remove deletes name; removed is false when it was not stored.
func (t TokenFile) Remove(name string) (removed bool, err error) {
	all, err := t.Load()
	if err != nil {
		return false, err
	}
	if _, ok := all[name]; !ok {
		return false, nil
	}
	delete(all, name)
	return true, t.save(all)
}

// save writes all atomically: a 0600 temp file in the same dir, renamed
// over the old one, so a crash never leaves a half-written or wider file.
func (t TokenFile) save(all map[string]string) error {
	if t.Path == "" {
		return errors.New("login: token file path is not set")
	}
	dir := filepath.Dir(t.Path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)
	doc := &yaml.Node{Kind: yaml.MappingNode}
	// Keys are quoted like values: a plain `null:` or `~:` key decodes as a
	// null, so a login named "null" would be written but never read back.
	for _, n := range names {
		doc.Content = append(doc.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: n, Style: yaml.DoubleQuotedStyle},
			&yaml.Node{Kind: yaml.ScalarNode, Value: all[n], Style: yaml.DoubleQuotedStyle})
	}
	var buf bytes.Buffer
	buf.WriteString(tokenFileHeader)
	if len(names) > 0 {
		enc := yaml.NewEncoder(&buf)
		if err := enc.Encode(doc); err != nil {
			return fmt.Errorf("login: encode token file: %w", err)
		}
		if err := enc.Close(); err != nil {
			return fmt.Errorf("login: encode token file: %w", err)
		}
	}
	tmp, err := os.CreateTemp(dir, "."+TokenFileName+".*")
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op after a successful rename
	if err := tmp.Chmod(filePerm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("login: %w", err)
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("login: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if err := os.Rename(tmp.Name(), t.Path); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	return nil
}
