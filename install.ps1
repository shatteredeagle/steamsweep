# steamsweep installer for Windows (PowerShell).
#   irm https://raw.githubusercontent.com/ShatteredEagle/steamsweep/master/install.ps1 | iex
# Env: $env:VERSION = "v0.1.0" to pin a release (default: latest).
$ErrorActionPreference = "Stop"
$repo = "ShatteredEagle/steamsweep"
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$file = "steamsweep_windows_$arch.zip"
$base = if ($env:VERSION) { "https://github.com/$repo/releases/download/$($env:VERSION)" } else { "https://github.com/$repo/releases/latest/download" }
$dir = Join-Path $env:LOCALAPPDATA "Programs\steamsweep"
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("steamsweep-" + [guid]::NewGuid())
New-Item -ItemType Directory -Force $tmp, $dir | Out-Null
try {
  Invoke-WebRequest "$base/$file" -OutFile "$tmp\$file"
  Invoke-WebRequest "$base/checksums.txt" -OutFile "$tmp\checksums.txt"
  $want = ((Get-Content "$tmp\checksums.txt" | Where-Object { $_ -match [regex]::Escape($file) + '$' }) -split '\s+')[0]
  $got = (Get-FileHash "$tmp\$file" -Algorithm SHA256).Hash.ToLower()
  if (-not $want -or $want -ne $got) { throw "checksum mismatch, aborting" }
  Expand-Archive "$tmp\$file" -DestinationPath $dir -Force
} finally { Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue }
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$dir*") { [Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User"); Write-Host "Added $dir to your PATH (open a new terminal)." }
Write-Host "Installed steamsweep to $dir"
