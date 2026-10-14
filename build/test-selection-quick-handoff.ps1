[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$BundleDirectory,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$DotnetExe = 'dotnet'
)

# Explicit foreground check: use generated text and isolated configuration.
# Relative output paths resolve from the repository, as in the build scripts.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$bundle = (Resolve-Path -LiteralPath $BundleDirectory).Path
$output = if ([IO.Path]::IsPathRooted($OutputDirectory)) { [IO.Path]::GetFullPath($OutputDirectory) } else { [IO.Path]::GetFullPath((Join-Path $root $OutputDirectory)) }
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if (Test-Path -LiteralPath $output) { throw '测试输出目录已存在，请指定新的 _temp/<任务>/ 子目录。' }
foreach ($relative in @('WinOne.Host.exe', 'selection/WinOne.Selection.exe', 'shared/ui/WinOne.Results.exe')) {
    if (-not (Test-Path -LiteralPath (Join-Path $bundle $relative) -PathType Leaf)) { throw "测试包缺少 $relative，请先构建对应组件。" }
}
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'run/quick-handoff.json')
if ($LASTEXITCODE -ne 0) { throw '测试输出未被 Git 忽略，请检查 .gitignore。' }
New-Item -ItemType Directory -Path $output | Out-Null
$probe = Join-Path $output 'probe'
& $DotnetExe build (Join-Path $root 'tests/selection-probe/SelectionProbe.csproj') -c Release -p:RestoreLockedMode=true -o $probe --nologo -v:q
if ($LASTEXITCODE -ne 0) { throw '划词测试程序构建失败，请检查构建输出。' }
$oldBundle = $env:WINONE_SELECTION_PIPELINE_BUNDLE
$oldUrl = $env:WINONE_SELECTION_PIPELINE_URL
$oldMode = $env:WINONE_SELECTION_QUICK_HANDOFF
try {
    $env:WINONE_SELECTION_PIPELINE_BUNDLE = $bundle
    # No AI request is made in handoff-only mode. Keep the common fixture's
    # endpoint constrained to loopback even if a future regression sends one.
    $env:WINONE_SELECTION_PIPELINE_URL = 'http://127.0.0.1:1/v1'
    $env:WINONE_SELECTION_QUICK_HANDOFF = '1'
    & (Join-Path $probe 'WinOne.Selection.Probe.exe') --pipeline (Join-Path $output 'run')
    if ($LASTEXITCODE -ne 0) { throw '划词到快捷助手的实窗检查失败，请查看输出目录中的 failure.txt。' }
    Get-Content -LiteralPath (Join-Path $output 'run/quick-handoff.json') -Raw -Encoding UTF8
} finally {
    $env:WINONE_SELECTION_PIPELINE_BUNDLE = $oldBundle
    $env:WINONE_SELECTION_PIPELINE_URL = $oldUrl
    $env:WINONE_SELECTION_QUICK_HANDOFF = $oldMode
}
