# Dot-source this file to select the registered, reusable local toolchain.
# Paths are repository-relative; nothing is installed and no build is started.
$resourceRepository = Split-Path -Parent $PSScriptRoot
$resourcePaths = @{
    GoExe = 'dist/resources/tools/go-1.26.2/bin/go.exe'
    ResultGoExe = 'dist/resources/tools/go-1.27.1/bin/go.exe'
    CargoExe = 'dist/resources/tools/rust-1.99.0/bin/cargo.exe'
    ShareXSourceDirectory = 'dist/resources/sources/ShareX-native'
    OcrRuntimeDirectory = 'dist/resources/runtimes/ocr-3.10.0'
    MagpieSourceDirectory = 'dist/resources/sources/magpie-50f52b5'
    PluginSourceDirectory = 'dist/resources/sources/ai-plugins-652f06d'
    BunExe = 'dist/resources/tools/bun-1.4.2/bun.exe'
    RustTargetDirectory = 'dist/resources/cache/rust-desktop'
    DiagramTargetDirectory = 'dist/resources/cache/rust-diagrams'
    SelectionTargetDirectory = 'dist/resources/cache/rust-selection'
}
$WinOneBuildParameters = @{}
# An already installed copy of the exact compiler is also a reusable resource.
# Never install a toolchain or save the machine's path in repository config.
if (-not (Test-Path -LiteralPath (Join-Path $resourceRepository $resourcePaths.GoExe))) {
    $availableGo = Get-Command go -ErrorAction Stop
    if ((& $availableGo.Source version) -notmatch '^go version go1\.26\.2 windows/amd64$') { throw 'Pass or register Go 1.26.2 for this repository' }
    $resourcePaths.GoExe = $availableGo.Source
}
foreach ($resourceKey in $resourcePaths.Keys) {
    $resourcePath = if ([IO.Path]::IsPathRooted($resourcePaths[$resourceKey])) { $resourcePaths[$resourceKey] } else { Join-Path $resourceRepository $resourcePaths[$resourceKey] }
    if (-not (Test-Path -LiteralPath $resourcePath)) { throw "Missing registered local resource: $resourcePath" }
    $WinOneBuildParameters[$resourceKey] = $resourcePath
}
$WinOneNsisExe = Join-Path $resourceRepository 'dist/resources/tools/nsis-3.13/makensis.exe'
if (-not (Test-Path -LiteralPath $WinOneNsisExe)) { throw "Missing registered installer compiler: $WinOneNsisExe" }
$env:GOROOT = Split-Path -Parent (Split-Path -Parent $WinOneBuildParameters.GoExe)
$env:CARGO_HOME = Join-Path $resourceRepository 'dist/resources/cache/cargo'
$env:GOCACHE = Join-Path $resourceRepository 'dist/resources/cache/go'
$env:npm_config_cache = Join-Path $resourceRepository 'dist/resources/cache/npm'

# Icon generation fills this cache only when local source inputs change.
$WinOneProviderIconCacheDirectory = Join-Path $resourceRepository 'dist/resources/assets/provider-icons'
$WinOneBrowserToolsDirectory = Join-Path $resourceRepository 'dist/resources/tools/playwright-1.58.2'
$playwrightModule = Join-Path $WinOneBrowserToolsDirectory 'node_modules/playwright'
if (Test-Path -LiteralPath (Join-Path $playwrightModule 'package.json')) {
    # Browser probes already accept this module path; reuse the locked package
    # without installing another copy in each task's temporary directory.
    # Windows PowerShell 5 cannot deserialize the empty root key in an npm lock
    # file. Both Playwright packages use the exact version in package.json.
    $browserPackage = Get-Content -LiteralPath (Join-Path $resourceRepository 'build/browser-tools/package.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    foreach ($package in @('playwright', 'playwright-core')) {
        $packagePath = Join-Path $WinOneBrowserToolsDirectory ("node_modules/$package/package.json")
        $installed = Get-Content -LiteralPath $packagePath -Raw -Encoding UTF8 | ConvertFrom-Json
        if ($installed.version -ne $browserPackage.dependencies.playwright) { throw "Local browser tool version differs from the lock: $package" }
    }
    $env:WINONE_PLAYWRIGHT_MODULE = $playwrightModule
}
