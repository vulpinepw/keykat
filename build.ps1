param(
  [string]$Version = "1.3.0",
  [string]$FirefoxDir = "",
  [string]$LinuxFirefox = "",
  [switch]$SkipTests,
  [switch]$Installer,
  [switch]$Linux
)

$ErrorActionPreference = "Stop"
$root = $PSScriptRoot
$dist = Join-Path $root "dist\Keycat"
$utf8 = New-Object System.Text.UTF8Encoding $false

if ($Version -notmatch '^(\d+)\.(\d+)\.(\d+)') {
  throw "Version must start with MAJOR.MINOR.PATCH, got '$Version'"
}
$numeric = "$($Matches[1]),$($Matches[2]),$($Matches[3]),0"
$year = (Get-Date).Year

function Invoke-Go {
  & go @args
  if ($LASTEXITCODE -ne 0) { throw "go $($args -join ' ') failed" }
}

function Write-Resources([string]$dir, [string]$icon, [string]$manifest, [string]$description, [string]$fileName) {
  $rc = @"
1 ICON "$icon"
1 24 "$manifest"

1 VERSIONINFO
FILEVERSION $numeric
PRODUCTVERSION $numeric
FILEFLAGSMASK 0x3f
FILEFLAGS 0x0
FILEOS 0x40004
FILETYPE 0x1
FILESUBTYPE 0x0
BEGIN
  BLOCK "StringFileInfo"
  BEGIN
    BLOCK "040904b0"
    BEGIN
      VALUE "CompanyName", "tordev"
      VALUE "FileDescription", "$description"
      VALUE "FileVersion", "$Version"
      VALUE "InternalName", "Keycat"
      VALUE "LegalCopyright", "\251 $year tordev"
      VALUE "OriginalFilename", "$fileName"
      VALUE "ProductName", "Keycat"
      VALUE "ProductVersion", "$Version"
      VALUE "Comments", "Keycat by tordev, github.com/vulpinepw. Built on the Mozilla Firefox engine (MPL 2.0)."
    END
  END
  BLOCK "VarFileInfo"
  BEGIN
    VALUE "Translation", 0x409, 1200
  END
END
"@
  [IO.File]::WriteAllText((Join-Path $dir "resources.rc"), $rc, $utf8)
  $syso = Join-Path $dir "rsrc_windows_amd64.syso"
  $windres = Get-Command windres -ErrorAction SilentlyContinue
  if ($windres) {
    Push-Location $dir
    try {
      & $windres.Source --target=pe-x86-64 -i resources.rc -O coff -o rsrc_windows_amd64.syso
      if ($LASTEXITCODE -ne 0) { throw "windres failed in $dir" }
    } finally {
      Pop-Location
    }
  } elseif (Test-Path $syso) {
    Write-Warning "windres not found; using the existing $syso"
  } else {
    Write-Warning "windres not found; $fileName will have no icon or version information"
  }
}
Get-ChildItem (Join-Path $root "cmd") -Recurse -Filter "keycat.rc" | Remove-Item -Force
Write-Resources (Join-Path $root "cmd\keycat") "keycat.ico" "keycat.manifest" "Keycat" "Keycat.exe"
Write-Resources (Join-Path $root "cmd\keycat-setup") "../keycat/keycat.ico" "setup.manifest" "Keycat Setup" "KeycatSetup.exe"
Write-Host "resources written"

$env:GOFLAGS = "-mod=mod"
Push-Location $root
try {
  if (-not $SkipTests) {
    Invoke-Go vet ./...
    Invoke-Go test ./internal/... ./tools/... ./cmd/...
    if ($Linux) {
      $env:GOOS = "linux"
      try { Invoke-Go vet ./... } finally { $env:GOOS = $null }
    }
  }

  $src = $root
  if ($Installer -or $Linux) {
    $src = Join-Path $root "build\stage"
    if (Test-Path $src) { Remove-Item -Recurse -Force $src }
    New-Item -ItemType Directory -Force $src | Out-Null
    foreach ($item in "go.mod", "go.sum", "assets.go", "cmd", "internal", "tools") {
      Copy-Item -Path (Join-Path $root $item) -Destination $src -Recurse
    }
    Invoke-Go run ./tools/strip -src hub -dst (Join-Path $src "hub")
    Invoke-Go run ./tools/strip -src browser -dst (Join-Path $src "browser")
    $node = Get-Command node -ErrorAction SilentlyContinue
    if ($node) {
      $check = Join-Path $root "tools\check-js.mjs"
      & $node.Source $check (Join-Path $src "hub") (Join-Path $src "browser")
      if ($LASTEXITCODE -ne 0) { throw "stripped JavaScript does not parse" }
    } else {
      Write-Warning "node not found; stripped JavaScript was not syntax-checked"
    }
  }
} finally {
  Pop-Location
}

New-Item -ItemType Directory -Force $dist | Out-Null
Push-Location $src
try {
  Invoke-Go build -trimpath -ldflags "-H=windowsgui -s -w -X main.version=$Version" -o (Join-Path $dist "Keycat.exe") ./cmd/keycat
} finally {
  Pop-Location
}
Write-Host "built: $(Join-Path $dist 'Keycat.exe')"

$scratch = Join-Path ([IO.Path]::GetTempPath()) ("keycat-build-" + [Guid]::NewGuid().ToString("N"))
$env:KEYCAT_DATA_DIR = $scratch
try {
  $core = Join-Path $dist "core"
  if (-not $FirefoxDir -and -not (Test-Path (Join-Path $core "firefox.exe"))) {
    $FirefoxDir = Join-Path $env:ProgramFiles "Mozilla Firefox"
  }
  if ($FirefoxDir) {
    if (-not (Test-Path (Join-Path $FirefoxDir "firefox.exe"))) {
      throw "No Firefox found in '$FirefoxDir'. Install Firefox or pass -FirefoxDir."
    }
    Write-Host "copying the Firefox engine from $FirefoxDir"
    $proc = Start-Process -FilePath (Join-Path $dist "Keycat.exe") -ArgumentList "--no-hub", "--setup-core", "`"$FirefoxDir`"" -Wait -PassThru
    if ($proc.ExitCode -ne 0) { throw "engine setup failed (exit code $($proc.ExitCode))" }
  } else {
    $proc = Start-Process -FilePath (Join-Path $dist "Keycat.exe") -ArgumentList "--no-hub", "--apply-layer" -Wait -PassThru
    if ($proc.ExitCode -ne 0) { throw "installing the Keycat layer failed (exit code $($proc.ExitCode))" }
  }
  Write-Host "engine ready: $core"
} finally {
  $env:KEYCAT_DATA_DIR = $null
  if (Test-Path $scratch) { Remove-Item -Recurse -Force $scratch -ErrorAction SilentlyContinue }
}

if ($Installer) {
  Copy-Item (Join-Path $root "LICENSE") (Join-Path $dist "LICENSE.txt") -Force
  $notices = Get-ChildItem (Join-Path $root "licenses") -Filter *.txt | Sort-Object Name | ForEach-Object {
    "==== $($_.BaseName) ====`r`n`r`n" + ((Get-Content $_.FullName -Raw) -replace "`r?`n", "`r`n")
  }
  [IO.File]::WriteAllText((Join-Path $dist "THIRD-PARTY-NOTICES.txt"), ($notices -join "`r`n`r`n"), $utf8)
  Push-Location $src
  try {
    Invoke-Go build -trimpath -ldflags "-H=windowsgui -s -w -X main.version=$Version" -o (Join-Path $dist "uninstall.exe") ./cmd/keycat-setup
    Invoke-Go run ./tools/mkpayload -src $dist -out (Join-Path $src "cmd\keycat-setup\payload.bin") -version $Version
    $setup = Join-Path $root "dist\KeycatSetup.exe"
    Invoke-Go build -tags payload -trimpath -ldflags "-H=windowsgui -s -w -X main.version=$Version" -o $setup ./cmd/keycat-setup
  } finally {
    Pop-Location
    Remove-Item (Join-Path $src "cmd\keycat-setup\payload.bin") -Force -ErrorAction SilentlyContinue
  }
  $mb = [Math]::Round((Get-Item $setup).Length / 1MB, 1)
  Write-Host "installer: $setup ($mb MB)"
}

if ($Linux) {
  if (-not $LinuxFirefox -or -not (Test-Path $LinuxFirefox)) {
    throw "Pass -LinuxFirefox with Firefox for Linux: firefox-<version>.tar.xz from archive.mozilla.org."
  }
  $firefoxTarball = (Resolve-Path $LinuxFirefox).Path
  $program = Join-Path $root "build\linux\keycat"
  $tarball = Join-Path $root "dist\keycat-linux-x86_64.tar.gz"
  Push-Location $src
  try {
    $env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
    try {
      Invoke-Go build -trimpath -ldflags "-s -w -X main.version=$Version" -o $program ./cmd/keycat
    } finally {
      $env:GOOS = $null; $env:GOARCH = $null; $env:CGO_ENABLED = $null
    }
    Invoke-Go run ./tools/mktarball -firefox $firefoxTarball -keycat $program -root $root -dir (Join-Path $root "dist\linux\keycat") -out $tarball
  } finally {
    Pop-Location
  }
  $mb = [Math]::Round((Get-Item $tarball).Length / 1MB, 1)
  Write-Host "linux package: $tarball ($mb MB)"
}
