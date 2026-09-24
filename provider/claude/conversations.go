package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/khanakia/agx/provider"
)

// EncodePath maps a working directory to the name of its history folder
// under <home>/projects: every byte that is not an ASCII letter or digit
// becomes '-'. Verified against every existing project dir on a real
// machine (158/158), including paths with '_', '.' and spaces.
func EncodePath(dir string) string {
	b := []byte(dir)
	for i, c := range b {
		if !isASCIIAlnum(c) {
			b[i] = '-'
		}
	}
	return string(b)
}

// isASCIIAlnum reports whether c is an ASCII letter or digit.
func isASCIIAlnum(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}

// HistoryDir is where Claude Code keeps conversations started in dir.
func HistoryDir(home, dir string) string {
	return filepath.Join(home, projectsDir, EncodePath(dir))
}

// Conversations lists the resumable conversations stored in home, newest
// first. With q.Dir set only that directory's history folder is read (a
// direct lookup via EncodePath); otherwise every project folder is scanned.
//
// Each transcript is read only at its head (for the cwd) and tail (for the
// title), never whole — see headScanBytes / tailScanBytes. A transcript that
// cannot be read is skipped rather than failing the listing: one corrupt
// file must not hide every other conversation.
func (p *Provider) Conversations(ctx context.Context, home string, q provider.ConversationQuery) ([]provider.Conversation, error) {
	var dirs []string
	if q.Dir != "" {
		dirs = []string{HistoryDir(home, q.Dir)}
	} else {
		entries, err := os.ReadDir(filepath.Join(home, projectsDir))
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("claude: list projects: %w", err)
		}
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(home, projectsDir, e.Name()))
			}
		}
	}

	type candidate struct {
		path string
		info os.FileInfo
	}
	var cands []candidate
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue // missing history folder = no conversations there
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), transcriptExt) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			cands = append(cands, candidate{path: filepath.Join(d, e.Name()), info: info})
		}
	}
	// Sort by mtime first so a Limit reads only the newest files.
	sort.Slice(cands, func(i, j int) bool { return cands[i].info.ModTime().After(cands[j].info.ModTime()) })

	var out []provider.Conversation
	for _, c := range cands {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
		conv, ok := readConversation(c.path)
		if !ok {
			continue
		}
		if q.Dir != "" && filepath.Clean(conv.Dir) != filepath.Clean(q.Dir) {
			continue // encoded-name collision (e.g. "a_b" vs "a.b"): keep only exact cwd
		}
		conv.Provider = provider.Claude
		conv.Home = home
		conv.Updated = c.info.ModTime()
		out = append(out, conv)
	}
	return out, nil
}

// transcriptLine is the subset of a transcript record read for listing.
type transcriptLine struct {
	Type        string `json:"type"`
	Cwd         string `json:"cwd"`
	SessionID   string `json:"sessionId"`
	AITitle     string `json:"aiTitle"`
	CustomTitle string `json:"customTitle"`
}

// readConversation extracts id, cwd and title from one transcript. ok is
// false when no cwd is found (a transcript that never ran a turn cannot be
// resumed in a known directory).
func readConversation(path string) (provider.Conversation, bool) {
	f, err := os.Open(path)
	if err != nil {
		return provider.Conversation{}, false
	}
	// Read-only: a close error cannot lose data, and the read already succeeded or failed.
	defer func() { _ = f.Close() }()

	conv := provider.Conversation{ID: strings.TrimSuffix(filepath.Base(path), transcriptExt)}

	head := io.LimitReader(f, headScanBytes)
	sc := bufio.NewScanner(head)
	sc.Buffer(make([]byte, 0, 64<<10), headScanBytes)
	for sc.Scan() {
		var l transcriptLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		if l.Cwd != "" {
			conv.Dir = l.Cwd
			break
		}
	}
	if conv.Dir == "" {
		return provider.Conversation{}, false
	}
	conv.Title = lastTitle(f)
	return conv, true
}

// lastTitle scans the file's tail for the newest title record, preferring a
// user-set custom title over the generated one.
func lastTitle(f *os.File) string {
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	start := info.Size() - tailScanBytes
	if start < 0 {
		start = 0
	}
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return ""
	}
	var custom, ai string
	for _, line := range bytes.Split(buf, []byte{'\n'}) {
		if !bytes.Contains(line, []byte(`"type":"`+recordAITitle+`"`)) && !bytes.Contains(line, []byte(`"type":"`+recordCustomTitle+`"`)) {
			continue
		}
		var l transcriptLine
		if json.Unmarshal(line, &l) != nil {
			continue
		}
		switch l.Type {
		case recordCustomTitle:
			if l.CustomTitle != "" {
				custom = l.CustomTitle
			}
		case recordAITitle:
			if l.AITitle != "" {
				ai = l.AITitle
			}
		}
	}
	if custom != "" {
		return custom
	}
	return ai
}
