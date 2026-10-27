# Installs the giode CLI on Windows:
#
#   irm https://raw.githubusercontent.com/Mehran9675/giode-cli/main/install.ps1 | iex
#
# It downloads the latest release (or $env:GIODE_VERSION, like v0.2.0)
# into %LOCALAPPDATA%\Programs\giode (or $env:GIODE_INSTALL_DIR) and adds
# that folder to your PATH.
$ErrorActionPreference = 'Stop'
$repo = 'Mehran9675/giode-cli'
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') { 'arm64' } else { 'amd64' }
$name = "giode_windows_$arch"
$url = if ($env:GIODE_VERSION) {
  "https://github.com/$repo/releases/download/$($env:GIODE_VERSION)/$name.zip"
} else {
  "https://github.com/$repo/releases/latest/download/$name.zip"
}
$dir = if ($env:GIODE_INSTALL_DIR) { $env:GIODE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\giode' }
New-Item -ItemType Directory -Force -Path $dir | Out-Null

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("giode-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
  Write-Host "Downloading $url"
  Invoke-WebRequest -Uri $url -OutFile (Join-Path $tmp 'giode.zip') -UseBasicParsing
  Expand-Archive -Path (Join-Path $tmp 'giode.zip') -DestinationPath $tmp -Force
  Copy-Item -Force (Join-Path $tmp 'giode.exe') (Join-Path $dir 'giode.exe')
} finally {
  Remove-Item -Recurse -Force $tmp
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not ($userPath -split ';' | Where-Object { $_ -eq $dir })) {
  [Environment]::SetEnvironmentVariable('Path', ($userPath.TrimEnd(';') + ';' + $dir), 'User')
  $env:Path += ";$dir"
  Write-Host "Added $dir to your PATH; open a new terminal to use giode there."
}
& (Join-Path $dir 'giode.exe') version
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  Write-Host 'giode builds apps with Go 1.26 or later: install it from https://go.dev/dl'
}
