#Requires -Version 7.0
[CmdletBinding()]
param(
    [string]$Version = 'dev',
    [switch]$BuildOnly,
    [switch]$MetadataOnly
)

. "$PSScriptRoot\windows-common.ps1"
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$release = Get-ReleaseVersion $Version
$bin = Join-Path $root 'bin'
$metadata = Join-Path $bin 'windows-info.json'
Write-WindowsVersionInfo -Version $Version -Template "$root\build\windows\info.json" -Output $metadata
if ($MetadataOnly) { return }
if (-not $IsWindows) { throw 'Windows release packaging must run on Windows.' }

$mode = Get-WindowsSigningMode -Certificate $env:WINDOWS_CERTIFICATE_BASE64 `
    -Password $env:WINDOWS_CERTIFICATE_PASSWORD -Thumbprint $env:WINDOWS_SIGN_THUMBPRINT `
    -TimestampServer $env:WINDOWS_TIMESTAMP_SERVER -Required:($env:WINDOWS_REQUIRE_SIGNING -eq 'true')
if ($env:GITHUB_OAUTH_CLIENT_ID -and $env:GITHUB_OAUTH_CLIENT_ID -notmatch '^[A-Za-z0-9._-]+$') {
    throw 'GITHUB_OAUTH_CLIENT_ID contains invalid characters.'
}
foreach ($tool in @('go', 'wails3', 'npm')) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        throw "$tool is missing from PATH. Install the versions used in .github\workflows\release.yml."
    }
}
if (-not (Test-Path "$root\frontend\node_modules")) {
    throw 'Frontend dependencies are missing. Run npm ci --prefix frontend first.'
}
$makeNSIS = $null
if (-not $BuildOnly) {
    $makeNSIS = Get-Command makensis -ErrorAction SilentlyContinue
    if (-not $makeNSIS) {
        foreach ($candidate in @("${env:ProgramFiles(x86)}\NSIS\makensis.exe", "$root\bin\tools\nsis\makensis.exe")) {
            if (Test-Path $candidate) { $makeNSIS = Get-Item $candidate; break }
        }
    }
    if (-not $makeNSIS) { throw 'makensis is missing. Install NSIS and add it to PATH.' }
}

$certificatePath = Join-Path $bin 'windows-codesign.pfx'
$syso = Join-Path $root 'wails_windows_amd64.syso'
if (Test-Path $certificatePath) { throw "Refusing to overwrite $certificatePath." }
if (Test-Path $syso) { throw "Remove the stale generated resource $syso before building." }
$signTool = $null
$signArgs = @()
if ($mode -eq 'signed') {
    $signTool = Get-Command signtool -ErrorAction SilentlyContinue
    if (-not $signTool) {
        $signTool = Get-ChildItem "${env:ProgramFiles(x86)}\Windows Kits\10\bin\*\x64\signtool.exe" |
            Sort-Object FullName -Descending | Select-Object -First 1
    }
    if (-not $signTool) { throw 'Signing is configured but the Windows SDK signtool is missing.' }
    $signTool = if ($signTool -is [System.IO.FileInfo]) { $signTool.FullName } else { $signTool.Source }
    $timestamp = if ($env:WINDOWS_TIMESTAMP_SERVER) { $env:WINDOWS_TIMESTAMP_SERVER } else { 'http://timestamp.digicert.com' }
    $signArgs = @('sign', '/fd', 'SHA256', '/td', 'SHA256', '/tr', $timestamp)
    if ($env:WINDOWS_CERTIFICATE_BASE64) {
        $signArgs += @('/f', $certificatePath)
        if ($env:WINDOWS_CERTIFICATE_PASSWORD) { $signArgs += @('/p', $env:WINDOWS_CERTIFICATE_PASSWORD) }
    } else {
        $signArgs += @('/sha1', $env:WINDOWS_SIGN_THUMBPRINT)
    }
} else {
    Write-Warning 'Building an UNSIGNED Windows release. SmartScreen may warn; SHA256SUMS.txt checks integrity, not publisher identity.'
}

$savedEnvironment = @{}
foreach ($name in @('GOOS', 'GOARCH', 'CGO_ENABLED', 'GOFLAGS')) {
    $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name)
}
$manifests = @('go.mod', 'go.sum', 'frontend\package.json', 'frontend\package-lock.json')
$manifestHashes = @{}
foreach ($path in $manifests) { $manifestHashes[$path] = (Get-FileHash "$root\$path").Hash }
$exe = Join-Path $bin 'SyncHub.exe'
$installer = Join-Path $bin 'SyncHub-for-Agents-Setup-x64.exe'
Push-Location $root
try {
    if ($env:WINDOWS_CERTIFICATE_BASE64) {
        [IO.File]::WriteAllBytes($certificatePath, [Convert]::FromBase64String($env:WINDOWS_CERTIFICATE_BASE64))
    }
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    $env:GOFLAGS = '-mod=readonly'
    # Do not invoke the generated common tasks: they run go mod tidy and npm install.
    Invoke-ReleaseTool wails3 @('generate', 'bindings', '-f', '-tags production -mod=readonly', '-clean=true', '-ts', '-i')
    Invoke-ReleaseTool npm @('--prefix', 'frontend', 'run', 'build')
    Invoke-ReleaseTool wails3 @('generate', 'syso', '-arch', 'amd64', '-icon', 'build\windows\icon.ico',
        '-manifest', 'build\windows\wails.exe.manifest', '-info', $metadata, '-out', $syso)
    $ldflags = "-w -s -H windowsgui -X github.com/qinqingxu/synchub-for-agents/internal/appversion.Version=$($release.Display)"
    if ($env:GITHUB_OAUTH_CLIENT_ID) { $ldflags += " -X main.githubOAuthClientID=$env:GITHUB_OAUTH_CLIENT_ID" }
    Invoke-ReleaseTool go @('build', '-mod=readonly', '-tags', 'production', '-trimpath', '-buildvcs=false',
        '-ldflags', $ldflags, '-o', $exe, '.')
    $versionOutput = Join-Path $bin 'windows-version.txt'
    $versionProcess = Start-Process -FilePath $exe -ArgumentList '--version' -NoNewWindow -Wait -PassThru -RedirectStandardOutput $versionOutput
    if ($versionProcess.ExitCode -ne 0 -or (Get-Content $versionOutput -Raw).Trim() -ne $release.Display) {
        throw 'The built executable did not report the requested version via --version.'
    }
    $exeInfo = [Diagnostics.FileVersionInfo]::GetVersionInfo($exe)
    if ($exeInfo.ProductVersion -ne $release.Display) { throw 'Executable version resource did not match the release.' }
    if ($mode -eq 'signed') {
        Invoke-ReleaseTool $signTool ($signArgs + @($exe))
        Invoke-ReleaseTool $signTool @('verify', '/pa', '/all', $exe)
    }
    if (-not $BuildOnly) {
        Invoke-ReleaseTool wails3 @('generate', 'webview2bootstrapper', '-dir', 'build\windows\nsis')
        Push-Location 'build\windows\nsis'
        try {
            $nsisPath = if ($makeNSIS -is [System.IO.FileInfo]) { $makeNSIS.FullName } else { $makeNSIS.Source }
            Invoke-ReleaseTool $nsisPath @('-V2', '-WX', "-DINFO_PRODUCTVERSION=$($release.Display)",
                "-DINFO_FILEVERSION=$($release.Numeric)", "-DARG_WAILS_AMD64_BINARY=$exe", 'project.nsi')
        } finally { Pop-Location }
        if (-not (Test-Path $installer)) { throw 'NSIS did not create the expected installer.' }
        if ($mode -eq 'signed') {
            Invoke-ReleaseTool $signTool ($signArgs + @($installer))
            Invoke-ReleaseTool $signTool @('verify', '/pa', '/all', $installer)
        }
        if ([Diagnostics.FileVersionInfo]::GetVersionInfo($installer).ProductVersion -ne $release.Display) {
            throw 'Installer version resource did not match the release.'
        }
        $hash = (Get-FileHash $installer -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  SyncHub-for-Agents-Setup-x64.exe" | Set-Content "$bin\SHA256SUMS.txt" -Encoding ascii
        $notice = if ($mode -eq 'signed') {
            'Windows x64: the application and installer are Authenticode signed.'
        } else {
            '**Windows x64 is unsigned:** no signing certificate was configured. Windows SmartScreen may show an unknown-publisher warning. SHA256SUMS.txt verifies download integrity, not publisher identity.'
        }
        if (-not $env:GITHUB_OAUTH_CLIENT_ID) { $notice += "`n`nGitHub OAuth is not configured in this build; SSH authentication remains available." }
        $notice | Set-Content "$bin\WINDOWS-RELEASE-NOTES.txt" -Encoding utf8NoBOM
        Write-Host "Installer: $installer"
        Write-Host "Checksums: $bin\SHA256SUMS.txt"
    }
} finally {
    Pop-Location
    Remove-Item -LiteralPath $syso, $certificatePath -Force -ErrorAction SilentlyContinue
    foreach ($name in $savedEnvironment.Keys) { [Environment]::SetEnvironmentVariable($name, $savedEnvironment[$name]) }
    foreach ($path in $manifests) {
        if ((Get-FileHash "$root\$path").Hash -ne $manifestHashes[$path]) {
            throw "Release packaging unexpectedly changed $path."
        }
    }
}
