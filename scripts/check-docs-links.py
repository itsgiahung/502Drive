#!/usr/bin/env python3
"""Check that every relative Markdown link resolves to a real file or directory."""
import os
import re
import sys

LINK = re.compile(r'(?<!\!)\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)')
SKIP_DIRS = {'.git', 'legacy-backup', 'node_modules', '.trash', '.local-notes',
             'dist', 'build', 'assets-out', '.video_agent'}


def markdown_files(root):
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS and not d.startswith('.')]
        for name in filenames:
            if name.endswith('.md'):
                yield os.path.join(dirpath, name)


def check(root):
    broken = []
    total = 0
    for path in markdown_files(root):
        base = os.path.dirname(path)
        with open(path, encoding='utf-8') as fh:
            text = fh.read()
        for match in LINK.finditer(text):
            target = match.group(1)
            if target.startswith(('http://', 'https://', 'mailto:', '#')):
                continue
            target = target.split('#', 1)[0]
            if not target:
                continue
            total += 1
            resolved = os.path.normpath(os.path.join(base, target))
            if not os.path.exists(resolved):
                broken.append((path, target))
    return total, broken


def main():
    root = sys.argv[1] if len(sys.argv) > 1 else '.'
    total, broken = check(root)
    for path, target in broken:
        print(f'BROKEN {path}: {target}')
    print(f'checked {total} relative links, {len(broken)} broken')
    return 1 if broken else 0


if __name__ == '__main__':
    sys.exit(main())
