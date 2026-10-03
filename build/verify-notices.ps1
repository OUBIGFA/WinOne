param([string]$GoExe='go', [string]$BundleDirectory, [string]$CatalogPath)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$root=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if (-not $CatalogPath) { $CatalogPath=Join-Path $root 'third_party/runtime-notices.json' }
$catalogPath=[IO.Path]::GetFullPath($CatalogPath)
if ($BundleDirectory) { $BundleDirectory=[IO.Path]::GetFullPath($BundleDirectory) }
$catalogText=Get-Content -LiteralPath $catalogPath -Raw -Encoding UTF8
$catalog=$catalogText | ConvertFrom-Json
if ($catalog.schemaVersion -ne 1) { throw 'Unsupported runtime notice catalog' }
$records=@{}
foreach ($component in $catalog.components) {
    if ($component.kind -notin @('go','toolchain','runtime','upstream','npm','nuget')) { throw 'Unsupported runtime notice kind' }
    $key=$component.kind+'|'+$component.name+'|'+$component.version
    if ($records.ContainsKey($key) -or @($component.notices).Count -eq 0) { throw "Invalid notice record: $key" }
    $records[$key]=$component
    foreach ($notice in $component.notices) {
        if ($notice.file -notmatch '^licenses/[A-Za-z0-9][A-Za-z0-9._-]*\.txt$' -or $notice.sha256 -notmatch '^[a-f0-9]{64}$') { throw "Invalid notice path or hash: $key" }
        $source=Join-Path $root ('third_party/'+$notice.file)
        if ((Get-FileHash -LiteralPath $source -Algorithm SHA256).Hash.ToLowerInvariant() -ne $notice.sha256) { throw "Notice content changed: $($notice.file)" }
        if ($BundleDirectory) {
            $published=Join-Path $BundleDirectory $notice.file
            if ((Get-FileHash -LiteralPath $published -Algorithm SHA256).Hash.ToLowerInvariant() -ne $notice.sha256) { throw "Published notice mismatch: $($notice.file)" }
        }
    }
}
Push-Location $root
try {
    & node (Join-Path $PSScriptRoot 'verify-frontend-notices.mjs') $catalogPath
    if ($LASTEXITCODE -ne 0) { throw 'Frontend dependency notice verification failed' }
    $template='{{if .Module}}{{.Module.Path}}|{{.Module.Version}}{{end}}'
    $lines=@(& $GoExe list -mod=readonly -deps -f $template ./apps/host ./modules/always-on-top/cmd/worker)
    if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect Host dependencies' }
    $lines+=@(& $GoExe list -mod=readonly -tags desktop,production -deps -f $template ./apps/desktop)
    if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect production desktop dependencies' }
    Push-Location modules/wintray
    try {
        $lines+=@(& $GoExe list -mod=readonly -deps -f $template ./cmd/worker)
        if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect WinTray dependencies' }
    } finally { Pop-Location }
    $actual=@{}
    foreach ($line in ($lines | Sort-Object -Unique)) {
        if (-not $line -or $line -like 'winone|*' -or $line -like 'winone/modules/*') { continue }
        $key='go|'+$line
        $actual[$key]=$true
        if (-not $records.ContainsKey($key)) { throw "Missing runtime notice: $line" }
    }
    foreach ($component in $catalog.components | Where-Object kind -eq 'go') {
        if (-not $actual.ContainsKey('go|'+$component.name+'|'+$component.version)) { throw "Stale Go notice record: $($component.name) $($component.version)" }
    }
    $goVersion=& $GoExe env GOVERSION
    if ($LASTEXITCODE -ne 0 -or -not $records.ContainsKey('toolchain|Go|'+$goVersion)) { throw 'Go toolchain notice version mismatch' }
    $sources=Get-Content -LiteralPath 'third_party/sources.json' -Raw -Encoding UTF8 | ConvertFrom-Json
    if (-not $records.ContainsKey('upstream|PowerToys|'+$sources.powertoys.commit)) { throw 'PowerToys notice revision mismatch' }
    if ($BundleDirectory) {
        $dependencies=Get-Content -LiteralPath (Join-Path $BundleDirectory 'capture/WinOne.Capture.deps.json') -Raw -Encoding UTF8 | ConvertFrom-Json
        $nuget=@{}
        foreach ($library in $dependencies.libraries.PSObject.Properties | Where-Object {$_.Value.type -eq 'package'}) {
            $parts=$library.Name.Split('/')
            if ($parts.Count -ne 2) { throw 'Invalid published NuGet identity' }
            $key='nuget|'+$parts[0]+'|'+$parts[1]
            if (-not $records.ContainsKey($key)) { throw "Missing NuGet notice: $($library.Name)" }
            $nuget[$key]=$true
        }
        foreach ($component in $catalog.components | Where-Object kind -eq 'nuget') {
            if (-not $nuget.ContainsKey('nuget|'+$component.name+'|'+$component.version)) { throw "Stale NuGet notice: $($component.name)" }
        }
        $runtime=Get-Content -LiteralPath (Join-Path $BundleDirectory 'capture/WinOne.Capture.runtimeconfig.json') -Raw -Encoding UTF8 | ConvertFrom-Json
        foreach ($framework in $runtime.runtimeOptions.includedFrameworks) {
            $name=$framework.name
            if ($name -in @('Microsoft.WindowsDesktop.App.WindowsForms','Microsoft.WindowsDesktop.App.WPF')) { $name='Microsoft.WindowsDesktop.App' }
            if (-not $records.ContainsKey('runtime|'+$name+'|'+$framework.version)) { throw "Missing runtime notice: $name $($framework.version)" }
        }
        foreach ($component in $catalog.components | Where-Object kind -eq 'runtime') {
            $matches=@($runtime.runtimeOptions.includedFrameworks | Where-Object { $_.name -like ($component.name+'*') -and $_.version -eq $component.version })
            if ($matches.Count -eq 0) { throw "Runtime notice version mismatch: $($component.name)" }
        }
        # The human-readable index travels with the same notices it describes.
        [IO.File]::WriteAllText((Join-Path $BundleDirectory 'licenses/runtime-notices.txt'),$catalogText,[Text.UTF8Encoding]::new($false))
    }
} finally { Pop-Location }
if ($BundleDirectory) {
    Write-Output "Dependency notices and published runtime versions verified: $($catalog.components.Count) records"
} else {
    Write-Output "Source notices and Go/npm versions verified: $($catalog.components.Count) records"
}
