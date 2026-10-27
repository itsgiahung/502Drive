# Intended for an isolated Windows CI runner. Never starts the daemon or logs in.
[CmdletBinding()]
param([Parameter(Mandatory = $true)][string]$Installer)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if (-not (Test-Path -LiteralPath $Installer -PathType Leaf)) { throw 'Installer not found.' }
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('502drive-installer-' + [Guid]::NewGuid().ToString('N'))
$appDir = Join-Path $testRoot 'app'
New-Item -ItemType Directory -Path $testRoot | Out-Null
try {
    # Suppress optional tasks: no PATH, startup or desktop changes in the smoke run.
    $arguments = @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/SP-', "/DIR=`"$appDir`"", '/TASKS=""')
    $process = Start-Process -FilePath $Installer -ArgumentList $arguments -Wait -PassThru
    if ($process.ExitCode -ne 0) { throw "Install exited with $($process.ExitCode)" }
    foreach ($file in @('502drive.exe', '502drive-tray.exe', '502d.cmd', 'unins000.exe')) {
        if (-not (Test-Path -LiteralPath (Join-Path $appDir $file) -PathType Leaf)) { throw "Installed file missing: $file" }
    }
    $version = & (Join-Path $appDir '502drive.exe') version
    if ($LASTEXITCODE -ne 0 -or -not ($version -match '502drive')) { throw 'Installed binary failed version smoke check.' }
    # A synthetic sentinel stands in for unrelated data and must survive uninstall.
    $sentinel = Join-Path $testRoot 'keep.txt'
    'synthetic fixture' | Set-Content -LiteralPath $sentinel
    $process = Start-Process -FilePath (Join-Path $appDir 'unins000.exe') -ArgumentList @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART') -Wait -PassThru
    if ($process.ExitCode -ne 0) { throw "Uninstall exited with $($process.ExitCode)" }
    if (Test-Path -LiteralPath (Join-Path $appDir '502drive.exe')) { throw 'Uninstall left the application binary.' }
    if (-not (Test-Path -LiteralPath $sentinel)) { throw 'Uninstall removed unrelated data.' }
    Write-Output 'PASS: install, version, uninstall; unrelated sentinel preserved.'
} finally {
    Remove-Item -LiteralPath $testRoot -Recurse -Force
}
