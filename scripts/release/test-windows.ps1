#Requires -Version 7.0
. "$PSScriptRoot\windows-common.ps1"

function Assert-Equal($Actual, $Expected) {
    if ($Actual -cne $Expected) { throw "Expected '$Expected', got '$Actual'." }
}
function Assert-Fails([scriptblock]$Action) {
    $failed = $false
    try { & $Action | Out-Null } catch { $failed = $true }
    if (-not $failed) { throw 'Expected an error, but the operation succeeded.' }
}

$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$scratch = Join-Path $root ('bin\packaging-tests-' + [guid]::NewGuid().ToString('N'))
try {
    foreach ($case in @(
        @('dev', 'dev', '0.0.0.0'),
        @('v0.3.0', '0.3.0', '0.3.0.0'),
        @('1.2.3-rc.1+build.7', '1.2.3-rc.1+build.7', '1.2.3.0'),
        @('65535.0.1', '65535.0.1', '65535.0.1.0')
    )) {
        $version = Get-ReleaseVersion $case[0]
        Assert-Equal $version.Display $case[1]
        Assert-Equal $version.Numeric $case[2]
        $metadata = Join-Path $scratch 'info.json'
        Write-WindowsVersionInfo -Version $case[0] -Template "$root\build\windows\info.json" -Output $metadata
        $info = Get-Content $metadata -Raw | ConvertFrom-Json -AsHashtable
        Assert-Equal $info.fixed.file_version $case[2]
        Assert-Equal $info.fixed.product_version $case[2]
        Assert-Equal $info.info.'0000'.ProductVersion $case[1]
        Assert-Equal $info.info.'0000'.FileVersion $case[1]
    }
    foreach ($invalid in @('v', '0.3', '01.2.3', '1.2.3.4', '65536.0.0', '1.2.3-01', '1.2.3;', '1.2.3"')) {
        Assert-Fails { Get-ReleaseVersion $invalid }
    }
    Assert-Equal (Get-WindowsSigningMode) 'unsigned'
    Assert-Equal (Get-WindowsSigningMode -Certificate 'configured') 'signed'
    Assert-Equal (Get-WindowsSigningMode -Certificate 'configured' -Password 'password') 'signed'
    Assert-Equal (Get-WindowsSigningMode -Thumbprint 'configured') 'signed'
    Assert-Fails { Get-WindowsSigningMode -Password 'orphan-password' }
    Assert-Fails { Get-WindowsSigningMode -Required }
    Assert-Fails { Get-WindowsSigningMode -TimestampServer 'https://timestamp.example' }
    Assert-Fails { Get-WindowsSigningMode -Certificate 'configured' -Thumbprint 'ambiguous' }
    Assert-Fails { Get-WindowsSigningMode -Thumbprint 'configured' -Password 'orphan-password' }
    Assert-Fails { Invoke-ReleaseTool pwsh @('-NoProfile', '-Command', 'exit 17') }
    Invoke-ReleaseTool pwsh @('-NoProfile', '-Command', 'exit 0')

    $project = Get-Content "$root\build\windows\nsis\project.nsi" -Raw
    if ($project -match 'MUI_FINISHPAGE_RUN|ExecShell|RMDir\s+/r') {
        throw 'Installer must not relaunch the app or recursively remove user files.'
    }
    if ($project -notmatch 'File "\.\.\\\.\.\\\.\.\\LICENSE"') {
        throw 'Installer must include LICENSE.'
    }
    Write-Host 'Windows packaging tests passed (versions, metadata, fail-closed signing, native exits, installer invariants).'
} finally {
    if (Test-Path $scratch) { Remove-Item -LiteralPath $scratch -Recurse -Force }
}
