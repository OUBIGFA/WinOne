param([string]$GoExe='go',[Parameter(Mandatory)][string]$OutputDirectory,[string]$Version,[string]$UpdateRepository='',[string]$ModulePublishers='',[string]$TestOnboardingDirectory='')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if(-not $Version){$Version=(Get-Content -LiteralPath (Join-Path $root 'apps/desktop/frontend/package.json') -Raw -Encoding UTF8 | ConvertFrom-Json).version}
if($Version -notmatch '^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$'){throw 'Invalid Host version'}
if($UpdateRepository -and $UpdateRepository -notmatch '^[A-Za-z0-9][A-Za-z0-9-]{0,38}/[A-Za-z0-9_.-]+$'){throw 'Invalid update repository'}
if($ModulePublishers -and $ModulePublishers -notmatch '^[A-Za-z0-9+/]+={0,2}$'){throw 'Invalid module trust encoding'}
New-Item -ItemType Directory -Path $output -Force | Out-Null
$hostFlags="-H windowsgui -X main.updateRepository=$UpdateRepository -X main.updateVersion=$Version -X main.modulePublishers=$ModulePublishers"
if($TestOnboardingDirectory){
    $testOnboarding=[IO.Path]::GetFullPath($TestOnboardingDirectory)
    & (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $testOnboarding
    & git -C $root check-ignore -q --no-index -- (Join-Path $testOnboarding 'onboarding-test.probe')
    if($LASTEXITCODE-ne 0 -or $testOnboarding -match "['`r`n]"){throw 'Choose an ignored temporary onboarding path without quotes or newlines'}
    # The override exists only in this isolated test binary. Production builds
    # always resolve the machine-wide Windows known folder.
    $hostFlags+=" -X 'winone/internal/onboarding.machineDirectoryOverride=$($testOnboarding.Replace('\','/'))'"
}
Push-Location $root
try {
    & $GoExe build -trimpath -ldflags $hostFlags -o (Join-Path $output 'WinOne.Host.exe') ./apps/host
    if($LASTEXITCODE -ne 0){throw 'Host build failed'}
} finally { Pop-Location }
