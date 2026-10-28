Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-ReleaseVersion {
    param([Parameter(Mandatory)][string]$Version)

    if ($Version -eq 'dev') {
        return @{ Display = 'dev'; Numeric = '0.0.0.0' }
    }
    $display = $Version -creplace '^v', ''
    if ($display -cnotmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$') {
        throw "Expected a semantic version (for example v0.3.0), got '$Version'."
    }
    foreach ($identifier in ($Matches[4] -split '\.')) {
        if ($identifier -match '^0[0-9]+$') {
            throw "Numeric prerelease identifiers must not have leading zeroes."
        }
    }
    $parts = ($display -split '[-+]')[0] -split '\.'
    foreach ($part in $parts) {
        if ([decimal]$part -gt 65535) {
            throw "Windows version components must fit in 16 bits."
        }
    }
    return @{ Display = $display; Numeric = ($parts -join '.') + '.0' }
}

function Write-WindowsVersionInfo {
    param(
        [Parameter(Mandatory)][string]$Version,
        [Parameter(Mandatory)][string]$Template,
        [Parameter(Mandatory)][string]$Output
    )

    $release = Get-ReleaseVersion $Version
    $info = Get-Content -LiteralPath $Template -Raw | ConvertFrom-Json -AsHashtable
    $info.fixed.file_version = $release.Numeric
    $info.fixed.product_version = $release.Numeric
    foreach ($translation in $info.info.Values) {
        $translation.ProductVersion = $release.Display
        $translation.FileVersion = $release.Display
    }
    New-Item -ItemType Directory -Force (Split-Path -Parent $Output) | Out-Null
    $info | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $Output -Encoding utf8NoBOM
}

function Invoke-ReleaseTool {
    param(
        [Parameter(Mandatory)][string]$FilePath,
        [string[]]$Arguments = @()
    )

    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$(Split-Path -Leaf $FilePath) failed with exit code $LASTEXITCODE."
    }
}

function Get-WindowsSigningMode {
    param(
        [string]$Certificate,
        [string]$Password,
        [string]$Thumbprint,
        [string]$TimestampServer,
        [switch]$Required
    )

    if ($Certificate -and $Thumbprint) {
        throw "Configure either WINDOWS_CERTIFICATE_BASE64 or WINDOWS_SIGN_THUMBPRINT, not both."
    }
    if (-not $Certificate -and -not $Thumbprint) {
        if ($Required -or $Password -or $TimestampServer) {
            throw "Windows signing is configured but no certificate or thumbprint was supplied."
        }
        return 'unsigned'
    }
    if ($Thumbprint -and $Password) {
        throw "WINDOWS_CERTIFICATE_PASSWORD requires WINDOWS_CERTIFICATE_BASE64."
    }
    return 'signed'
}

function Write-WindowsPortableBundle {
    param(
        [Parameter(Mandatory)][string]$Executable,
        [Parameter(Mandatory)][string]$License,
        [Parameter(Mandatory)][string]$Output
    )

    foreach ($file in @($Executable, $License)) {
        $item = Get-Item -LiteralPath $file
        if ($item.PSIsContainer -or $item.Length -eq 0) { throw "Missing or empty portable bundle input: $file." }
    }
    Compress-Archive -LiteralPath $Executable, $License -DestinationPath $Output -Force
}

function Write-ReleaseChecksums {
    param(
        [Parameter(Mandatory)][string[]]$Files,
        [Parameter(Mandatory)][string]$Output
    )

    $lines = foreach ($file in $Files) {
        $item = Get-Item -LiteralPath $file
        if ($item.PSIsContainer -or $item.Length -eq 0) { throw "Missing or empty release asset: $file." }
        (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + $item.Name
    }
    $lines | Set-Content -LiteralPath $Output -Encoding ascii
}
