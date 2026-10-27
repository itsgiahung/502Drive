# Run from any directory after staging verified binaries in dist/windows.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$')][string]$Version,
    [string]$Compiler = '',
    [switch]$SmokeTest
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repo = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
if ($PSVersionTable.PSVersion.Major -ge 6 -and -not $IsWindows) { throw 'Inno Setup requires a Windows build host.' }
if (-not $Compiler) { $Compiler = Join-Path ${env:ProgramFiles(x86)} 'Inno Setup 6\ISCC.exe' }
if (-not (Test-Path -LiteralPath $Compiler -PathType Leaf)) { throw "Inno Setup compiler not found: $Compiler" }
foreach ($binary in @('502drive.exe', '502drive-tray.exe')) {
    if (-not (Test-Path -LiteralPath (Join-Path $repo "dist/windows/$binary") -PathType Leaf)) { throw "Stage the verified Windows archive first: missing $binary" }
}
& $Compiler "/DMyAppVersion=$Version" (Join-Path $PSScriptRoot '502drive.iss')
if ($LASTEXITCODE -ne 0) { throw "Inno Setup exited with $LASTEXITCODE" }
$installer = Join-Path $repo 'artifacts/windows/502drive-setup.exe'
if (-not (Test-Path -LiteralPath $installer -PathType Leaf)) { throw 'Installer was not produced.' }
$hash = (Get-FileHash -LiteralPath $installer -Algorithm SHA256).Hash.ToLowerInvariant()
"$hash  502drive-setup.exe" | Set-Content -LiteralPath "$installer.sha256" -Encoding ascii
if ($SmokeTest) { & (Join-Path $PSScriptRoot 'smoke-installer.ps1') -Installer $installer }
Write-Output $installer
