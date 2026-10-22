param(
    [Parameter(Mandatory)][string]$SourceDirectory,
    [Parameter(Mandatory)][string]$BaseRevision,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$GoExe='go',
    [string]$BunExe,
    [string[]]$Packages=@('provider','gateway','gui','plugin','pluginrt','middleware','redact','masking','pii','settings','usage','catalog','filememo','netproxy','sessions','maskrec','routes','ghtoken','proc','access','filelock')
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$source=(Resolve-Path -LiteralPath $SourceDirectory).Path
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if(Test-Path -LiteralPath $output){throw 'Choose a new upstream-test output directory'}
foreach($package in $Packages){if($package -notin @('provider','gateway','gui','plugin','pluginrt','middleware','redact','masking','pii','settings','usage','catalog','filememo','netproxy','sessions','maskrec','routes','ghtoken','proc','access','filelock')){throw 'Only AI service and its shared dependency packages are allowed'}}
$files=@(& git -C $source diff --name-only --diff-filter=AMR $BaseRevision HEAD -- 'internal/*/*_test.go')
if($LASTEXITCODE -ne 0){throw 'Cannot enumerate the selected upstream increment'}
New-Item -ItemType Directory -Path $output -Force|Out-Null
$cache=& $GoExe env -json GOPATH GOMODCACHE | ConvertFrom-Json
$environment=@{
    USERPROFILE=(Join-Path $output 'home');APPDATA=(Join-Path $output 'home/roaming');LOCALAPPDATA=(Join-Path $output 'home/local')
    XDG_CONFIG_HOME=(Join-Path $output 'config');XDG_CACHE_HOME=(Join-Path $output 'cache');CODEX_HOME=(Join-Path $output 'home/.codex')
    TEMP=(Join-Path $output 'temp');TMP=(Join-Path $output 'temp');MAGPIE_NO_STATS='1';MAGPIE_OTEL_ENABLED='false'
    GOPATH=$cache.GOPATH;GOMODCACHE=$cache.GOMODCACHE
}
if($BunExe){$environment.MAGPIE_BUN=(Resolve-Path -LiteralPath $BunExe).Path;$environment.PATH=(Split-Path -Parent $environment.MAGPIE_BUN)+';'+$env:PATH}
# No upstream test may discover real developer accounts or external app homes.
foreach($item in [Environment]::GetEnvironmentVariables().GetEnumerator()){
    if($item.Key -match '^(CODEX_|CLAUDE_|ANTHROPIC_|OPENAI_|GEMINI_|COPILOT_|CURSOR_|GROK_|KIRO_|DEVIN_|WORKBUDDY_|QODER_|ZED_|FACTORY_|DROID_|MIMO_|ZCODE_|AWS_|AZURE_|WINONE_AI_)|^(GH_TOKEN|GITHUB_TOKEN|GOOGLE_APPLICATION_CREDENTIALS)$'){
        if(-not $environment.ContainsKey([string]$item.Key)){$environment[[string]$item.Key]=$null}
    }
}
$previous=@{};$results=@()
try {
    foreach($name in $environment.Keys){$previous[$name]=[Environment]::GetEnvironmentVariable($name);[Environment]::SetEnvironmentVariable($name,$environment[$name])}
    New-Item -ItemType Directory -Force -Path $environment.USERPROFILE,$environment.APPDATA,$environment.LOCALAPPDATA,$environment.TEMP,$environment.XDG_CONFIG_HOME,$environment.XDG_CACHE_HOME|Out-Null
    Push-Location $source
    try {
        foreach($package in $Packages){
            # Test public behavior from every added/changed test file in this
            # increment, not just a hand-picked successful subset.
            $names=@(foreach($file in $files){if($file -match ('^internal/'+[regex]::Escape($package)+'/[^/]+_test\.go$')){
                [regex]::Matches([IO.File]::ReadAllText((Join-Path $source $file)),'(?m)^func (Test[A-Za-z0-9_]+)\(')|ForEach-Object{$_.Groups[1].Value}
            }})|Sort-Object -Unique
            if(-not $names){continue}
            # Two upstream cases expect auto-installing a missing vendor CLI.
            # That conflicts with WinOne's explicit-install boundary. Verify
            # the shipped refusal contract instead and report the substitution.
            $adapted=@()
            if($package -eq 'provider'){
                foreach($name in @('TestSignInInstallFails','TestSignInCanceledWhileInstalling')){
                    if($name -in $names){$adapted+=@{upstream=$name;replacement='TestWinOneOAuthMissingClientNeverRunsAnInstaller';reason='WinOne never installs another client during sign-in'};$names=@($names|Where-Object {$_ -ne $name})}
                }
                if($adapted.Count){$names+= 'TestWinOneOAuthMissingClientNeverRunsAnInstaller'}
            }
            $pattern='^('+($names -join '|')+')$'
            $ErrorActionPreference='Continue'
            & $GoExe test ('./internal/'+$package) -run $pattern -count=1 -parallel=2 -timeout=5m -v 2>&1 | Out-File (Join-Path $output ($package+'.log')) -Encoding utf8
            $code=$LASTEXITCODE
            $ErrorActionPreference='Stop'
            $results += [ordered]@{package=$package;tests=$names;adaptedCases=$adapted;exitCode=$code}
            Write-Output ($package+': '+$names.Count+' selected tests; exit '+$code)
        }
    } finally {Pop-Location}
    [IO.File]::WriteAllText((Join-Path $output 'results.json'),($results|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
    if(@($results|Where-Object exitCode -ne 0).Count){throw 'Upstream AI increment has failing tests; see results.json and package logs'}
} finally {foreach($name in $previous.Keys){[Environment]::SetEnvironmentVariable($name,$previous[$name])}}
