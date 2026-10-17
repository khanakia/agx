"""doclint: checks the markdown under docs/ and examples/ for the four
mistakes the docs keep making by hand.

    python3 scripts/docs/doclint.py [PATH ...]    (default: docs examples)

Exit 0 when clean, 1 when any finding is printed (file:line: rule: detail).

Rules (each one is pinned by a case in test_doclint.py):

  wrap   Hard-wrapped prose. Every paragraph, list item, blockquote and table
         cell must be ONE physical line; the renderer wraps. A line is a
         hard wrap when it follows a prose line (paragraph, list item or
         blockquote text) with no blank line between and does not itself
         start a new block. New blocks are: a list item (`-`, `*`, `+`,
         `1.`, `1)`, at any indentation, so nested lists are fine), a
         heading, a table row (`|`), an HTML line (`<`), a fence, a link
         reference definition (`[x]: url`). Exempt regions: fenced code
         (line breaks are meaningful there), YAML frontmatter, multi-line
         HTML comments, tables (a row after a row is a new row). Indented
         (4-space) code blocks are not recognised: use fences.
  link   A relative link or image whose file does not exist, or whose
         `#anchor` is not a heading of the target .md (GitHub's slug rule:
         lowercase, punctuation dropped except `-` and `_`, spaces to `-`,
         `-N` suffix for repeats). http(s), mailto and pure `#top`-style
         links to the same page are checked the same way; URLs with a
         scheme are not fetched. Links inside code spans and fences are
         ignored.
  path   A machine-local path in a public file, fences included: /Users/,
         /Volumes/, /space/, /private/, /khanakia, or /home/<name> for any
         name other than the placeholders `you`, `demo` and `user`.
  word   Authorship and vendor wording: the word `AI` (case-sensitive, so
         `ai-title` is fine), the capitalised company name `Anthropic`
         (so `ANTHROPIC_API_KEY` and `anthropic-ratelimit-*` headers are
         fine), `Co-Authored-By`, and any "generated/written/created by
         (or with) <a model or assistant>" claim. Plain "generated" stays
         allowed: agx really generates shell code, and docs say so.
         "Claude Code" is allowed as the product agx launches; only the
         authorship phrasing above is refused.
"""

import os
import re
import sys

SKIP_DIRS = {".git", "node_modules", "vendor", "dist", "bin"}

FENCE = re.compile(r"^\s*(```|~~~)")
BLOCK_START = re.compile(
    r"^\s*("
    r"[-*+]\s|[-*+]$"  # bullet
    r"|\d+[.)]\s"  # ordered item
    r"|#{1,6}(\s|$)"  # heading
    r"|\|"  # table row
    r"|<"  # HTML block or comment
    r"|```|~~~"  # fence
    r"|\[[^\]]+\]:\s"  # link reference definition
    r")"
)
TABLE = re.compile(r"^\s*\|")
HEADING = re.compile(r"^\s{0,3}(#{1,6})\s+(.*?)\s*#*\s*$")
HTML_LINE = re.compile(r"^\s*<")
QUOTE = re.compile(r"^\s*>\s?")

LOCAL_PATH = re.compile(r"/Users/|/Volumes/|/space/|/private/|(?<![\w.])/khanakia\b|/home/(?!(?:you|demo|user)\b)[A-Za-z_][\w.-]*")
WORDS = [
    (re.compile(r"(?<![\w-])AI(?![\w-])"), "the word AI"),
    (re.compile(r"(?<![\w.-])Anthropic(?![\w.-])"), "the company name Anthropic"),
    (re.compile(r"(?i)co-authored-by"), "a Co-Authored-By line"),
    (
        re.compile(
            r"(?i)\b(generated|written|drafted|created|authored|produced)\s+(by|with|using)\s+"
            r"(an?\s+|the\s+)?(ai\b|llm|claude|chatgpt|gpt|copilot|gemini|language model|assistant|model\b)"
        ),
        "an authorship claim",
    ),
]

INLINE_CODE = re.compile(r"(`+)(.+?)\1")
MD_LINK = re.compile(r"!?\[(?:[^\]\\]|\\.)*\]\(\s*<?([^)\s>]+)>?(?:\s+\"[^\"]*\")?\s*\)")
HTML_REF = re.compile(r"""<(?:img|a|source)\b[^>]*?\b(?:src|href|srcset)=["']([^"']+)["']""")
SCHEME = re.compile(r"^[a-zA-Z][a-zA-Z0-9+.-]*:")


def slug(text):
    """GitHub's heading anchor for one heading text."""
    text = re.sub(r"!?\[([^\]]*)\]\([^)]*\)", r"\1", text)
    text = text.replace("`", "").strip().lower()
    text = re.sub(r"[^\w\- ]", "", text, flags=re.UNICODE)
    return text.replace(" ", "-")


_anchor_cache = {}


def anchors(path):
    """Every anchor GitHub creates for the headings of one markdown file."""
    if path in _anchor_cache:
        return _anchor_cache[path]
    out, seen, fence = set(), {}, False
    with open(path, encoding="utf-8") as f:
        for line in f:
            if FENCE.match(line):
                fence = not fence
                continue
            if fence:
                continue
            m = HEADING.match(line)
            if not m:
                continue
            base = slug(m.group(2))
            n = seen.get(base, 0)
            out.add(base if n == 0 else f"{base}-{n}")
            seen[base] = n + 1
    _anchor_cache[path] = out
    return out


def regions(lines):
    """Yield (index, line, kind) with kind one of text, fence, front, comment."""
    fence = None
    comment = False
    front = bool(lines) and lines[0].strip() == "---"
    for i, line in enumerate(lines):
        if front:
            yield i, line, "front"
            if i > 0 and line.strip() == "---":
                front = False
            continue
        if fence is not None:
            yield i, line, "fence"
            if line.strip().startswith(fence):
                fence = None
            continue
        if comment:
            yield i, line, "comment"
            if "-->" in line:
                comment = False
            continue
        m = FENCE.match(line)
        if m:
            fence = m.group(1)
            yield i, line, "fence"
            continue
        if line.lstrip().startswith("<!--") and "-->" not in line:
            comment = True
            yield i, line, "comment"
            continue
        yield i, line, "text"


def is_prose(line):
    """A line whose next physical line, if not a new block, is a hard wrap."""
    s = QUOTE.sub("", line) if QUOTE.match(line) else line
    if not s.strip():
        return False
    if TABLE.match(s) or HEADING.match(s) or HTML_LINE.match(s):
        return False
    if re.match(r"^\s*\[[^\]]+\]:\s", s):
        return False
    return True


def check_wrap(lines):
    prev = None  # previous text line, None after a blank or a non-text region
    for i, line, kind in regions(lines):
        if kind != "text" or not line.strip():
            prev = None
            continue
        if prev is not None and is_prose(prev):
            body = line
            if QUOTE.match(line):
                if not QUOTE.match(prev):
                    body = None  # a quote right after prose starts a new block
                else:
                    body = QUOTE.sub("", line)
                    if not body.strip():
                        body = None  # `>` alone is a blank line in a quote
            if body is not None and not BLOCK_START.match(body):
                yield i, "wrap", "hard-wrapped line: join it to the line above"
        prev = line


def link_targets(line):
    scrubbed = INLINE_CODE.sub(lambda m: " " * len(m.group(0)), line)
    for m in MD_LINK.finditer(scrubbed):
        yield m.group(1)
    for m in HTML_REF.finditer(scrubbed):
        yield m.group(1)


def check_links(path, lines):
    base = os.path.dirname(path)
    for i, line, kind in regions(lines):
        if kind != "text":
            continue
        for target in link_targets(line):
            if SCHEME.match(target) or target.startswith("//"):
                continue
            file_part, _, frag = target.partition("#")
            file_part = file_part.split("?", 1)[0]
            dest = os.path.normpath(os.path.join(base, file_part)) if file_part else path
            if not os.path.exists(dest):
                yield i, "link", f"{target}: no such file"
                continue
            if frag and dest.endswith(".md"):
                if frag not in anchors(dest):
                    yield i, "link", f"{target}: no heading with anchor #{frag}"
            elif frag and not re.match(r"^L\d+(-L\d+)?$", frag) and os.path.isfile(dest):
                yield i, "link", f"{target}: anchors only resolve in .md files (or #L<n> line links)"


def check_text(lines):
    for i, line, kind in regions(lines):
        if kind == "front":
            continue
        m = LOCAL_PATH.search(line)
        if m:
            yield i, "path", f"machine-local path {m.group(0)!r}: use ~ or a placeholder home"
        for rx, what in WORDS:
            m = rx.search(line)
            if m:
                yield i, "word", f"{what} ({m.group(0)!r})"


def lint_file(path):
    with open(path, encoding="utf-8") as f:
        lines = f.read().split("\n")
    findings = list(check_wrap(lines)) + list(check_links(path, lines)) + list(check_text(lines))
    return sorted(findings)


def md_files(paths):
    for p in paths:
        if os.path.isfile(p):
            yield p
            continue
        for root, dirs, files in os.walk(p):
            dirs[:] = sorted(d for d in dirs if d not in SKIP_DIRS)
            for name in sorted(files):
                if name.endswith(".md"):
                    yield os.path.join(root, name)


def main(argv):
    paths = argv or ["docs", "examples"]
    files = list(md_files(paths))
    bad = 0
    for path in files:
        for i, rule, detail in lint_file(path):
            print(f"{path}:{i + 1}: {rule}: {detail}")
            bad += 1
    print(f"doclint: {len(files)} files, {bad} finding(s)", file=sys.stderr)
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
