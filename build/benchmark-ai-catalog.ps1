param([string]$GoExe='go', [Parameter(Mandatory)][string]$OutputDirectory, [string]$Overlay)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
New-Item -ItemType Directory -Force -Path $output | Out-Null
$priorTemp=$env:TEMP;$priorTmp=$env:TMP
Push-Location $root
try {
    $env:TEMP=$output;$env:TMP=$output
    # An optional Go overlay compares prior algorithms against the same current
    # benchmark cases, without changing the checkout or the compiled product.
    $arguments=@('test')
    if($Overlay){
        $overlayPath=(Resolve-Path -LiteralPath $Overlay).Path
        & (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $overlayPath
        $arguments+=@('-overlay',$overlayPath)
    }
    $arguments+=@('./internal/ai','-run','^TestCatalogMergePreservesOrderAndOwnership$','-bench','^(BenchmarkCatalogReads|BenchmarkProviderSnapshot)$','-benchmem','-benchtime=200ms','-count=3')
    & $GoExe @arguments | Tee-Object -FilePath (Join-Path $output 'benchmark.txt')
    if($LASTEXITCODE){throw 'AI catalog benchmark failed'}
} finally { $env:TEMP=$priorTemp;$env:TMP=$priorTmp;Pop-Location }
