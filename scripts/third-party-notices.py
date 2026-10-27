#!/usr/bin/env python3
"""Generate THIRD_PARTY_NOTICES.md from the Go module graph.

Licenses are read from each module's LICENSE/COPYING file in the module cache.
A module that is not downloaded, or whose license cannot be recognised, is
listed as "unidentified" instead of guessing.
"""
import glob
import os
import subprocess
import sys

MARKERS = [
    ('Apache-2.0', lambda h: 'apache license' in h and 'version 2.0' in h),
    ('MIT', lambda h: 'mit license' in h or 'permission is hereby granted, free of charge' in h),
    ('MPL-2.0', lambda h: 'mozilla public license' in h and '2.0' in h),
    ('GPL-3.0', lambda h: 'gnu general public license' in h and 'version 3' in h),
    ('ISC', lambda h: 'isc license' in h or 'permission to use, copy, modify, and/or distribute' in h),
    ('Unlicense', lambda h: 'free and unencumbered software released into the public domain' in h),
    ('BSD-3-Clause', lambda h: 'redistribution and use in source and binary forms' in h and 'neither the name' in h),
    ('BSD-2-Clause', lambda h: 'redistribution and use in source and binary forms' in h),
]


def license_of(directory):
    if not directory or not os.path.isdir(directory):
        return 'unidentified'
    names = ('LICENSE*', 'License*', 'COPYING*', 'Copying*', 'NOTICE*', 'Notice*')
    for pattern in names:
        for path in sorted(glob.glob(os.path.join(directory, pattern))):
            try:
                with open(path, encoding='utf-8', errors='ignore') as fh:
                    head = fh.read(6000).lower()
            except OSError:
                continue
            for name, test in MARKERS:
                if test(head):
                    return name
    return 'unidentified'


def modules():
    fmt = '{{.Path}}\t{{.Dir}}\t{{.Version}}'
    out = subprocess.run(['go', 'list', '-m', '-f', fmt, 'all'],
                         capture_output=True, text=True, check=True).stdout
    for line in out.splitlines():
        parts = line.split('\t')
        if len(parts) != 3 or parts[0] == '':
            continue
        yield parts[0], parts[1], parts[2]


def main():
    rows = list(modules())
    main_module = rows[0][0]
    print('# Third Party Notices\n')
    print('502Drive is licensed under GPL-3.0-only; see [LICENSE](LICENSE). This list')
    print('comes from `go list -m all`; licenses are identified from files in the module')
    print('cache. Unavailable or unrecognized licenses are marked **unidentified**.\n')
    print('Regenerate with `python3 scripts/third-party-notices.py > THIRD_PARTY_NOTICES.md`')
    print('after downloading the module graph.\n')
    print('| Module | Version | License |')
    print('| --- | --- | --- |')
    for path, directory, version in rows:
        if path == main_module:
            continue
        print(f'| `{path}` | {version or "—"} | {license_of(directory)} |')
    print()
    print('The graph includes test and build-tool dependencies as well as runtime modules.')
    print('Release binaries contain only packages imported by `cmd/502drive` and')
    print('`cmd/502drive-tray`; for example, the Windows resource generator is a build tool.')
    return 0


if __name__ == '__main__':
    sys.exit(main())
