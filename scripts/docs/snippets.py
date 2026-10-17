"""snippets: keep the Go code and program output quoted in the docs equal to
the real source and the real output of the example programs.

    python3 scripts/docs/snippets.py --write [PATH ...]   regenerate in place
    python3 scripts/docs/snippets.py --check [PATH ...]   exit 1 if any is stale

Run from the repository root (default PATH: docs examples). A quoted region
is marked in the markdown itself, so the page stays the only source:

    <!-- snip: examples/autopick/main.go func:run -->
    (generated: a "Source:" link line and a ```go block)
    <!-- /snip -->

    <!-- out: autopick -->
    (generated: a ```text block with the output of `go run ./examples/autopick`)
    <!-- /out -->

Symbols: FILE (the whole file), func:<name> (a function or method, with its
doc comment), type:<name>, const:<name>, var:<name> (a single spec, or a
parenthesised group whose first spec is <name>; var:_ picks the
`var ( _ ... )` interface-check group). Everything between a pair of markers
is replaced on --write, so never edit inside one by hand.

The examples are deterministic on purpose (temp dirs shown as ~, fixed
clocks, httptest servers), which is what makes --check meaningful; an
example that prints a timestamp or a random port would make every check
stale.
"""

import difflib
import os
import re
import subprocess
import sys

# A marker pair; group 1 is the kind, group 2 its arguments.
REGION = re.compile(r"<!-- (snip|out): ([^>]*?) -->\n.*?<!-- /\1 -->", re.S)
SKIP_DIRS = {".git", "node_modules", "vendor", "dist", "bin"}

_outputs = {}


def example_output(name):
    """Combined output of one example program, run once per invocation."""
    if name not in _outputs:
        proc = subprocess.run(
            ["go", "run", "./examples/" + name],
            capture_output=True,
            text=True,
        )
        if proc.returncode != 0:
            raise SystemExit(f"snippets: go run ./examples/{name} failed:\n{proc.stdout}{proc.stderr}")
        _outputs[name] = (proc.stdout + proc.stderr).rstrip("\n")
    return _outputs[name]


def extract(path, symbol):
    """The verbatim source of one top-level declaration, doc comment included."""
    with open(path, encoding="utf-8") as f:
        lines = f.read().split("\n")
    if symbol == "FILE":
        return "\n".join(lines).rstrip("\n")
    kind, _, name = symbol.partition(":")
    if kind not in ("func", "type", "const", "var") or not name:
        raise SystemExit(f"snippets: {path}: bad symbol {symbol!r} (want FILE or func|type|const|var:<name>)")
    start = None
    for i, line in enumerate(lines):
        if kind == "func" and re.match(r"^func (\([^)]*\) )?" + re.escape(name) + r"[(\[]", line):
            start = i
        elif kind == "type" and line.startswith("type " + name + " "):
            start = i
        elif kind in ("const", "var") and line.startswith(kind + " " + name + " "):
            start = i
        elif kind in ("const", "var") and line == kind + " (":
            j = i + 1
            while j < len(lines) and lines[j].strip().startswith("//"):
                j += 1
            if j < len(lines) and lines[j].strip().split(" ")[0] == name:
                start = i
        if start is not None:
            break
    if start is None:
        raise SystemExit(f"snippets: {path}: {symbol} not found")
    s = start
    while s > 0 and lines[s - 1].startswith("//"):
        s -= 1
    first = lines[start].rstrip()
    if first.endswith("{") or first.endswith("("):
        closer = "}" if first.endswith("{") else ")"
        e = start
        while lines[e] != closer:
            e += 1
    elif first.count("`") % 2 == 1:
        e = start + 1
        while "`" not in lines[e]:
            e += 1
    else:
        e = start
    return "\n".join(lines[s : e + 1])


def render_region(md_path, kind, args):
    args = args.split()
    if kind == "snip":
        if len(args) != 2:
            raise SystemExit(f"snippets: {md_path}: snip wants <path> <symbol>, got {args}")
        src, symbol = args
        name = symbol.split(":")[-1]
        if symbol == "FILE":
            label = "the whole file"
        elif name == "_":
            label = "the compile-time interface checks"
        else:
            label = f"`{name}`"
        rel = os.path.relpath(src, os.path.dirname(md_path))
        body = f"Source: [`{src}`]({rel}), {label}, quoted verbatim.\n\n```go\n{extract(src, symbol)}\n```"
    else:
        if len(args) != 1:
            raise SystemExit(f"snippets: {md_path}: out wants <example>, got {args}")
        body = "```text\n" + example_output(args[0]) + "\n```"
    return f"<!-- {kind}: {' '.join(args)} -->\n\n{body}\n\n<!-- /{kind} -->"


def render(md_path, text):
    return REGION.sub(lambda m: render_region(md_path, m.group(1), m.group(2)), text)


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
    if not argv or argv[0] not in ("--write", "--check"):
        print(__doc__.strip().split("\n\n")[1], file=sys.stderr)
        return 2
    mode, paths = argv[0], argv[1:] or ["docs", "examples"]
    stale = regions = 0
    for path in md_files(paths):
        with open(path, encoding="utf-8") as f:
            old = f.read()
        n = len(REGION.findall(old))
        if not n:
            continue
        regions += n
        new = render(path, old)
        if new == old:
            continue
        if mode == "--write":
            with open(path, "w", encoding="utf-8") as f:
                f.write(new)
            print(f"snippets: rewrote {path}")
        else:
            stale += 1
            sys.stdout.writelines(difflib.unified_diff(old.splitlines(True), new.splitlines(True), path, path + " (regenerated)"))
    print(f"snippets: {regions} region(s), {stale} stale file(s)", file=sys.stderr)
    if stale:
        print("snippets: run `task docs:snippets` to regenerate", file=sys.stderr)
    return 1 if stale else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
