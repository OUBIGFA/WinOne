param(
    [Parameter(Mandatory)][string]$Archive,
    [Parameter(Mandatory)][string]$OutputDirectory
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$output=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot '../../build/assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'probe.json')
if($LASTEXITCODE -ne 0 -or (Test-Path -LiteralPath $output)){throw 'Choose a new ignored output directory'}
$source=Get-Content -LiteralPath (Join-Path $root 'third_party/sources.json') -Raw -Encoding UTF8 | ConvertFrom-Json
if((Get-FileHash -LiteralPath $Archive -Algorithm SHA256).Hash -ne $source.sharex.archiveSha256){throw 'ShareX archive hash mismatch'}
foreach($script in @('sharex-capture-resources.ps1','sharex-native-host.ps1','sharex-native-presentation.ps1')){. (Join-Path $root ('build/'+$script))}
# Exercise the production orchestration without replacing the shared dependency cache.
$tokens=$null;$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $root 'build/prepare-sharex.ps1'),[ref]$tokens,[ref]$errors)
if($errors.Count){throw 'Cannot parse ShareX preparation script'}
$definition=$ast.Find({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'ApplyLocalEditorOptions'},$true)
if(-not $definition){throw 'ShareX preparation entry missing'}
. ([scriptblock]::Create($definition.Extent.Text))
New-Item -ItemType Directory -Path $output | Out-Null
Add-Type -AssemblyName System.IO.Compression.FileSystem
[IO.Compression.ZipFile]::ExtractToDirectory((Resolve-Path -LiteralPath $Archive).Path,$output)
$checkout=Join-Path $output 'ShareX-21.0.0'
ApplyLocalEditorOptions $checkout
$hashes=@{}
Get-ChildItem -LiteralPath $checkout -Recurse -File | ForEach-Object {$hashes[$_.FullName]=(Get-FileHash -LiteralPath $_.FullName).Hash}
ApplyLocalEditorOptions $checkout
foreach($path in $hashes.Keys){if((Get-FileHash -LiteralPath $path).Hash -ne $hashes[$path]){throw "Repeated preparation changed $path"}}
'PASS: full ShareX preparation from locked archive and repeated preparation'
