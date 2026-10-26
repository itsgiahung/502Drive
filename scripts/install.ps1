# Install 502Drive on Windows from a GitHub release (checksum verified).
#   irm https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.ps1 | iex
# Parameters (as environment variables): VERSION (default latest), PREFIX (default %LOCALAPPDATA%\Programs\502Drive)
$ErrorActionPreference = 'Stop'
$repo = 'itsgiahung/502Drive'
$prefix = if ($env:PREFIX) { $env:PREFIX } else { Join-Path $env:LOCALAPPDATA 'Programs\502Drive' }
$base = if ($env:DRIVE502_RELEASE_BASE) { $env:DRIVE502_RELEASE_BASE }
        elseif ($env:VERSION) { "https://github.com/$repo/releases/download/$($env:VERSION)" }
        else { "https://github.com/$repo/releases/latest/download" }

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("502drive-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Invoke-WebRequest "$base/checksums.txt" -OutFile "$tmp\checksums.txt" -UseBasicParsing
    $line = Get-Content "$tmp\checksums.txt" | Where-Object { $_ -match '_windows_amd64\.zip$' } | Select-Object -First 1
    if (-not $line) { throw 'no windows/amd64 archive in this release' }
    $want, $name = $line -split '\s+', 2
    Invoke-WebRequest "$base/$name" -OutFile "$tmp\$name" -UseBasicParsing
    $got = (Get-FileHash "$tmp\$name" -Algorithm SHA256).Hash.ToLower()
    if ($got -ne $want.ToLower()) { throw "checksum mismatch for $name; nothing was installed" }

    New-Item -ItemType Directory -Force -Path $prefix | Out-Null
    Expand-Archive "$tmp\$name" -DestinationPath $tmp\x -Force
    Copy-Item "$tmp\x\502drive.exe", "$tmp\x\502drive-tray.exe" -Destination $prefix -Force
    # Short alias: `502d` runs 502drive.
    Set-Content -Path (Join-Path $prefix '502d.cmd') -Value "@echo off`r`n`"%~dp0502drive.exe`" %*" -Encoding ASCII

    $user = [string][Environment]::GetEnvironmentVariable('Path', 'User')
    if (($user -split ';') -notcontains $prefix) {
        $joined = if ($user) { $user.TrimEnd(';') + ';' + $prefix } else { $prefix }
        [Environment]::SetEnvironmentVariable('Path', $joined, 'User')
        Write-Host "Added $prefix to your PATH (open a new terminal)."
    }
    Write-Host "Installed 502drive and the short alias 502d in $prefix"
    Write-Host 'Start with:  502drive      (or just: 502d)'
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
