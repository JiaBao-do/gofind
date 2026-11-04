<#
.SYNOPSIS
  gofind installer for Windows (x64 / ARM64).

.DESCRIPTION
  irm https://raw.githubusercontent.com/JiaBao-do/gofind/main/install.ps1 | iex

  Installs gofind.exe into %LOCALAPPDATA%\Programs\gofind, adds it to the user PATH. No admin
  rights required.

  Environment variables (useful with "irm | iex"):
    GOFIND_VERSION      release tag to install (default: latest)
    GOFIND_INSTALL_DIR  install directory
    GOFIND_BINARY       install this local binary instead of downloading
    GOFIND_UNINSTALL=1  remove gofind and its PATH entry
    GOFIND_NO_MODIFY_PATH=1  don't change the user PATH

  Offline: put gofind-windows-<arch>.exe next to this script and run
  .\install.ps1 - it is used instead of downloading. Go is never required.
#>
param(
    [string]$Version = $(if ($env:GOFIND_VERSION) { $env:GOFIND_VERSION } else { 'latest' }),
    [string]$InstallDir = $(if ($env:GOFIND_INSTALL_DIR) { $env:GOFIND_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\gofind' }),
    [string]$Binary = $env:GOFIND_BINARY,
    [switch]$Uninstall = [bool]$env:GOFIND_UNINSTALL
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'   # makes Invoke-WebRequest much faster on PS 5.1
$Repo = 'JiaBao-do/gofind'

function Write-Step($msg) { Write-Host '==> ' -ForegroundColor Blue -NoNewline; Write-Host $msg }

function Get-Arch {
    $arch = $null
    try { $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString() } catch {}
    if (-not $arch) {
        # 32-bit/emulated processes report the host arch in PROCESSOR_ARCHITEW6432.
        $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    }
    switch -Regex ($arch) {
        '^(X64|AMD64)$' { return 'amd64' }
        '^(Arm64|ARM64)$' { return 'arm64' }
        default { throw "Unsupported architecture: $arch (gofind supports x64 and ARM64)" }
    }
}

function Get-UserPath { [Environment]::GetEnvironmentVariable('Path', 'User') }

function Test-InPath($dir, $pathValue) {
    $norm = $dir.TrimEnd('\')
    foreach ($p in ($pathValue -split ';')) {
        if ($p -and ([Environment]::ExpandEnvironmentVariables($p).TrimEnd('\') -ieq $norm)) { return $true }
    }
    return $false
}

function Add-ToUserPath($dir) {
    $userPath = Get-UserPath
    if (-not (Test-InPath $dir $userPath)) {
        $new = if ($userPath) { "$($userPath.TrimEnd(';'));$dir" } else { $dir }
        # Setting the User variable through .NET also broadcasts WM_SETTINGCHANGE,
        # so newly opened terminals pick it up without logging out.
        [Environment]::SetEnvironmentVariable('Path', $new, 'User')
        Write-Step "added $dir to your user PATH"
    }
    if (-not (Test-InPath $dir $env:Path)) { $env:Path = "$env:Path;$dir" }
}

function Remove-FromUserPath($dir) {
    $userPath = Get-UserPath
    if (-not $userPath) { return }
    $kept = ($userPath -split ';') | Where-Object { $_ -and ($_.TrimEnd('\') -ine $dir.TrimEnd('\')) }
    $new = $kept -join ';'
    if ($new -ne $userPath) {
        [Environment]::SetEnvironmentVariable('Path', $new, 'User')
        Write-Step "removed $dir from your user PATH"
    }
}

function Install-Gofind {
    if ($Uninstall) {
        $exe = Join-Path $InstallDir 'gofind.exe'
        if (Test-Path $exe) { Remove-Item -Force $exe; Write-Step "removed $exe" }
        if ((Test-Path $InstallDir) -and -not (Get-ChildItem $InstallDir -Force)) { Remove-Item $InstallDir }
        Remove-FromUserPath $InstallDir
        Write-Step 'gofind uninstalled'
        return
    }

    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("gofind-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        $downloaded = Join-Path $tmp 'gofind.exe'

        if (-not $Binary -and $PSScriptRoot) {
            # Offline install: use a matching binary next to this script, or in
            # dist\ when run from a source checkout after "make dist".
            foreach ($dir in @($PSScriptRoot, (Join-Path $PSScriptRoot 'dist'))) {
                $bundled = Join-Path $dir "gofind-windows-$(Get-Arch).exe"
                if (Test-Path $bundled) { $Binary = $bundled; break }
            }
        }

        if ($Binary) {
            if (-not (Test-Path $Binary)) { throw "Binary not found: $Binary" }
            Write-Step "installing local build $Binary"
            Copy-Item $Binary $downloaded
        }
        else {
            $asset = "gofind-windows-$(Get-Arch).exe"
            $base = if ($Version -eq 'latest') { "https://github.com/$Repo/releases/latest/download" }
                    else { "https://github.com/$Repo/releases/download/$Version" }

            Write-Step "downloading $asset ($Version)"
            try {
                Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile $downloaded
            }
            catch {
                $msg = "Could not download $base/$asset`n  ($($_.Exception.Message))`n"
                $msg += "  - Check that https://github.com/$Repo/releases has a published release ($Version) with this file.`n"
                $msg += "  - From a source checkout you can install your own build instead: make install, or .\install.ps1 -Binary <path\to\gofind.exe>"
                throw $msg
            }

            try {
                $sums = (Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt").Content
                if ($sums -is [byte[]]) { $sums = [Text.Encoding]::UTF8.GetString($sums) }
                $line = ($sums -split "`n") | Where-Object { $_ -match "\s\*?$([regex]::Escape($asset))\s*$" } | Select-Object -First 1
                if ($line) {
                    $expected = ($line -split '\s+')[0].ToLower()
                    $actual = (Get-FileHash -Algorithm SHA256 $downloaded).Hash.ToLower()
                    if ($expected -ne $actual) { throw "Checksum mismatch for $asset" }
                    Write-Step 'checksum verified'
                }
            }
            catch {
                if ($_.Exception.Message -like 'Checksum mismatch*') { throw }
                Write-Warning 'checksums.txt not available, skipping verification'
            }
        }

        New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
        $target = Join-Path $InstallDir 'gofind.exe'
        try {
            Copy-Item -Force $downloaded $target
        }
        catch {
            throw "Could not write $target - is gofind currently running? Close it and retry."
        }
        Unblock-File $target -ErrorAction SilentlyContinue
        Write-Step "installed $target ($(& $target version))"
    }
    finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }

    if ($env:GOFIND_NO_MODIFY_PATH) { Write-Warning "skipping PATH update; add $InstallDir yourself" }
    else { Add-ToUserPath $InstallDir }

    # Machine PATH is searched before user PATH, so an older copy elsewhere would win.
    $target = Join-Path $InstallDir 'gofind.exe'
    $effective = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' + (Get-UserPath)
    foreach ($p in ($effective -split ';')) {
        if (-not $p) { continue }
        $candidate = Join-Path ([Environment]::ExpandEnvironmentVariables($p)) 'gofind.exe'
        if (Test-Path $candidate) {
            if ((Resolve-Path $candidate).Path -ine (Resolve-Path $target).Path) {
                Write-Warning "another gofind.exe at $candidate comes first on PATH and will be used instead. Remove it to use this install."
            }
            break
        }
    }

    Write-Host ''
    Write-Host 'Run: gofind   (open a new terminal if the command is not found yet)'
}

Install-Gofind
