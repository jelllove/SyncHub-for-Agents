param(
    [Parameter(Mandatory = $true)]
    [string]$Installer
)

$ErrorActionPreference = "Stop"
$installDir = Join-Path $env:TEMP "AgentConfigSync-Smoke-$PID"
$exe = Join-Path $installDir "AgentConfigSync.exe"
$uninstaller = Join-Path $installDir "uninstall.exe"
$shortcut = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\AgentConfigSync.lnk"
$runKey = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
$runName = "io.github.qinqingxu.agentconfigsync"
$appProcess = $null
$smokeProfile = Join-Path $env:TEMP "AgentConfigSync-Smoke-Profile-$PID"
$originalUserProfile = $env:USERPROFILE
$originalHome = $env:HOME

try {
    $install = Start-Process -FilePath (Resolve-Path $Installer) -ArgumentList "/S", "/D=$installDir" -Wait -PassThru
    if ($install.ExitCode -ne 0) {
        throw "Installer exited with code $($install.ExitCode)"
    }
    if (!(Test-Path $exe)) {
        throw "Installed executable is missing: $exe"
    }
    if (!(Test-Path $shortcut)) {
        throw "Start menu shortcut is missing: $shortcut"
    }

    New-Item -ItemType Directory -Path $smokeProfile -Force | Out-Null
    $env:USERPROFILE = $smokeProfile
    $env:HOME = $smokeProfile
    $appProcess = Start-Process -FilePath $exe -ArgumentList "--hidden" -PassThru
    Start-Sleep -Seconds 5
    $appProcess.Refresh()
    if ($appProcess.HasExited) {
        throw "Installed application exited during startup"
    }

    $second = Start-Process -FilePath $exe -ArgumentList "--hidden" -PassThru
    if (!$second.WaitForExit(10000)) {
        throw "Second application launch did not hand off to the first instance"
    }
    $matching = @(Get-Process | Where-Object {
        try { $_.Path -eq $exe } catch { $false }
    })
    if ($matching.Count -ne 1) {
        throw "Expected one installed application process, found $($matching.Count)"
    }
} finally {
    $env:USERPROFILE = $originalUserProfile
    $env:HOME = $originalHome
    if ($null -ne $appProcess) {
        $appProcess.Refresh()
        if (!$appProcess.HasExited) {
            Stop-Process -Id $appProcess.Id
            Wait-Process -Id $appProcess.Id -ErrorAction SilentlyContinue
        }
    }
    if (Test-Path $uninstaller) {
        $uninstall = Start-Process -FilePath $uninstaller -ArgumentList "/S" -Wait -PassThru
        if ($uninstall.ExitCode -ne 0) {
            throw "Uninstaller exited with code $($uninstall.ExitCode)"
        }
        if (Test-Path $smokeProfile) {
            Remove-Item -LiteralPath $smokeProfile -Recurse -Force
        }
    }
}

if (Test-Path $exe) {
    throw "Application files remain after uninstall"
}
if (Test-Path $shortcut) {
    throw "Start menu shortcut remains after uninstall"
}
if ((Get-ItemProperty -Path $runKey -Name $runName -ErrorAction SilentlyContinue).$runName) {
    throw "Autostart registration remains after uninstall"
}

Write-Output "Windows installer smoke test passed"
