package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/provider"
)

// YAML keys the editor touches. They must match the struct tags above.
const (
	keyLogins    = "logins"
	keyProfiles  = "profiles"
	keyProviders = "providers"
	keyLogin     = "login"
	keyName      = "name"
	keyProvider  = "provider"
	keyHome      = "home"
	keyDefault   = "default"
)

// Edit file modes: a new config is private by default; an existing one
// keeps whatever mode the user gave it.
const (
	newConfigPerm os.FileMode = 0o600
	configDirPerm os.FileMode = 0o700
	yamlIndent                = 2
)

// unsetValue renders an absent key in a Change.
const unsetValue = "(unset)"

// Change describes one edit for the user: the key path and its value
// before and after. Notes carry anything else the edit did.
type Change struct {
	Where  string
	Before string
	After  string
	Notes  []string
}

// Editor edits config.yaml in place with the yaml.v3 node API, so the
// user's comments and key order survive. Every edit is validated by a full
// Decode + Resolve before it is written, so agx never writes a config it
// would then refuse to load.
type Editor struct {
	Path      string
	UserHome  string
	Providers []provider.Provider
}

// edit loads the document, applies fn, validates and writes it atomically.
func (e Editor) edit(fn func(root *yaml.Node) error) error {
	raw, err := os.ReadFile(e.Path)
	mode := newConfigPerm
	switch {
	case errors.Is(err, fs.ErrNotExist):
		raw = nil
	case err != nil:
		return fmt.Errorf("config: read %s: %w", e.Path, err)
	default:
		if info, err := os.Stat(e.Path); err == nil {
			mode = info.Mode().Perm()
		}
	}
	var doc yaml.Node
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("config: %s: %w", e.Path, err)
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("config: %s: top level is not a mapping", e.Path)
	}
	if err := fn(doc.Content[0]); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("config: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("config: encode: %w", err)
	}
	f, err := Decode(buf.Bytes())
	if err != nil {
		return fmt.Errorf("config: edit would produce an invalid file: %w", err)
	}
	if _, err := Resolve(f, e.UserHome, e.Providers); err != nil {
		return fmt.Errorf("config: edit would produce an invalid file: %w", err)
	}
	return writeAtomic(e.Path, buf.Bytes(), mode)
}

// writeAtomic replaces path with data via a temp file in the same dir.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, configDirPerm); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op after a successful rename
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("config: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return nil
}

// AddLogin appends spec to `logins:`, or replaces the entry of the same
// name when replace is set.
func (e Editor) AddLogin(spec LoginSpec, replace bool) error {
	return e.edit(func(root *yaml.Node) error {
		seq := ensureSeq(root, keyLogins)
		entry := loginNode(spec)
		for i, item := range seq.Content {
			if scalar(item, keyName) == spec.Name {
				if !replace {
					return fmt.Errorf("login %q already exists (use --replace to rotate its token)", spec.Name)
				}
				seq.Content[i] = entry
				return nil
			}
		}
		seq.Content = append(seq.Content, entry)
		return nil
	})
}

// loginNode renders spec with only its set fields, in schema order.
func loginNode(s LoginSpec) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode}
	for _, kv := range [][2]string{
		{keyName, s.Name}, {keyProvider, s.Provider}, {"kind", s.Kind},
		{"token", s.Token}, {"token_cmd", s.TokenCmd}, {"token_env", s.TokenEnv},
		{"account", s.Account}, {"added", s.Added},
	} {
		if kv[1] != "" {
			setScalar(m, kv[0], kv[1])
		}
	}
	return m
}

// RemoveLogin deletes login name and clears every `login:` that points at
// it (profiles and provider defaults), returning what was cleared.
func (e Editor) RemoveLogin(name string) ([]string, error) {
	var cleared []string
	err := e.edit(func(root *yaml.Node) error {
		seq := mapGet(root, keyLogins)
		found := false
		if seq != nil && seq.Kind == yaml.SequenceNode {
			for i, item := range seq.Content {
				if scalar(item, keyName) == name {
					seq.Content = append(seq.Content[:i], seq.Content[i+1:]...)
					found = true
					break
				}
			}
		}
		if !found {
			return fmt.Errorf("no login %q in %s", name, e.Path)
		}
		if profs := mapGet(root, keyProfiles); profs != nil && profs.Kind == yaml.SequenceNode {
			for _, item := range profs.Content {
				if scalar(item, keyLogin) == name {
					mapDelete(item, keyLogin)
					cleared = append(cleared, "profiles["+scalar(item, keyName)+"].login")
				}
			}
		}
		if provs := mapGet(root, keyProviders); provs != nil && provs.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(provs.Content); i += 2 {
				if scalar(provs.Content[i+1], keyLogin) == name {
					mapDelete(provs.Content[i+1], keyLogin)
					cleared = append(cleared, "providers."+provs.Content[i].Value+".login")
				}
			}
			pruneEmptyProviders(root)
		}
		return nil
	})
	return cleared, err
}

// SetProfileLogin sets `login:` on profile p to name ("home" is written
// literally: it overrides a provider default). A profile that exists only
// by discovery is first written into the file together with every other
// discovered profile of its provider — configuring one hides the rest
// (package doc), so writing only one would make the others vanish.
func (e Editor) SetProfileLogin(cfg Config, p provider.Profile, name string) (Change, error) {
	ch := Change{Where: "profiles[" + p.Name + "].login", Before: unsetValue, After: name}
	err := e.edit(func(root *yaml.Node) error {
		profs := ensureSeq(root, keyProfiles)
		if p.Source == provider.SourceDiscovered {
			for _, d := range cfg.Profiles {
				if d.Provider != p.Provider || d.Source != provider.SourceDiscovered {
					continue
				}
				m := &yaml.Node{Kind: yaml.MappingNode}
				setScalar(m, keyName, d.Name)
				setScalar(m, keyProvider, string(d.Provider))
				setScalar(m, keyHome, tildePath(d.Home, e.UserHome))
				if d.Default {
					setScalar(m, keyDefault, "true")
				}
				profs.Content = append(profs.Content, m)
				ch.Notes = append(ch.Notes, "wrote discovered profile "+d.Name+" into the config")
			}
		}
		for _, item := range profs.Content {
			if scalar(item, keyName) != p.Name {
				continue
			}
			if v := mapGet(item, keyLogin); v != nil {
				ch.Before = v.Value
			}
			setScalar(item, keyLogin, name)
			return nil
		}
		return fmt.Errorf("profile %q is not in %s", p.Name, e.Path)
	})
	return ch, err
}

// SetProviderLogin sets providers.<id>.login to name; "home" removes the
// key, since there is no lower level for it to override.
func (e Editor) SetProviderLogin(id provider.ID, name string) (Change, error) {
	ch := Change{Where: "providers." + string(id) + ".login", Before: unsetValue, After: name}
	err := e.edit(func(root *yaml.Node) error {
		provs := mapGet(root, keyProviders)
		if provs == nil {
			provs = &yaml.Node{Kind: yaml.MappingNode}
			setNode(root, keyProviders, provs)
		}
		if provs.Kind != yaml.MappingNode {
			return fmt.Errorf("%s: providers is not a mapping", e.Path)
		}
		entry := mapGet(provs, string(id))
		if entry == nil || entry.Kind != yaml.MappingNode {
			entry = &yaml.Node{Kind: yaml.MappingNode}
			setNode(provs, string(id), entry)
		}
		if v := mapGet(entry, keyLogin); v != nil {
			ch.Before = v.Value
		}
		if name == login.Home {
			mapDelete(entry, keyLogin)
			pruneEmptyProviders(root)
			ch.After = unsetValue
			return nil
		}
		setScalar(entry, keyLogin, name)
		return nil
	})
	return ch, err
}

// tildePath writes p relative to the user's home when it is inside it.
func tildePath(p, userHome string) string {
	if rel, err := filepath.Rel(userHome, p); err == nil && !strings.HasPrefix(rel, "..") {
		if rel == "." {
			return "~"
		}
		return "~/" + filepath.ToSlash(rel)
	}
	return p
}

// mapGet returns the value node for key in mapping m, or nil.
func mapGet(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// scalar returns the scalar value of key in mapping m, or "".
func scalar(m *yaml.Node, key string) string {
	if v := mapGet(m, key); v != nil && v.Kind == yaml.ScalarNode {
		return v.Value
	}
	return ""
}

// setNode sets key to v in mapping m, keeping the key's position (and the
// comments on it) when it already exists.
func setNode(m *yaml.Node, key string, v *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			v.LineComment = m.Content[i+1].LineComment
			m.Content[i+1] = v
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, v)
}

// setScalar sets key to a plain string scalar. "true" is tagged as a bool
// so `default: true` stays a boolean.
func setScalar(m *yaml.Node, key, value string) {
	n := &yaml.Node{Kind: yaml.ScalarNode, Value: value}
	if value == "true" {
		n.Tag = "!!bool"
	}
	setNode(m, key, n)
}

// pruneEmptyProviders drops every providers.<id> mapping left with no keys,
// then `providers:` itself when nothing is left in it, so removing the last
// setting leaves no `claude: {}` / `providers: {}` behind. Only empty
// mappings go: a null, a scalar or a non-empty entry is the user's and is
// kept as written.
func pruneEmptyProviders(root *yaml.Node) {
	provs := mapGet(root, keyProviders)
	if provs == nil || provs.Kind != yaml.MappingNode {
		return
	}
	kept := provs.Content[:0]
	for i := 0; i+1 < len(provs.Content); i += 2 {
		if v := provs.Content[i+1]; v.Kind == yaml.MappingNode && len(v.Content) == 0 {
			continue
		}
		kept = append(kept, provs.Content[i], provs.Content[i+1])
	}
	provs.Content = kept
	if len(provs.Content) == 0 {
		mapDelete(root, keyProviders)
	}
}

// mapDelete removes key from mapping m.
func mapDelete(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

// ensureSeq returns the sequence under key, creating it when absent.
func ensureSeq(m *yaml.Node, key string) *yaml.Node {
	if v := mapGet(m, key); v != nil && v.Kind == yaml.SequenceNode {
		return v
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode}
	setNode(m, key, seq)
	return seq
}
