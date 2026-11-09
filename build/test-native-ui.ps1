param(
    [ValidateSet('desktop-mygo','result-ui')][string[]]$Modules=@('desktop-mygo','result-ui'),
    [string[]]$Packages=@('./...'),
    [string]$Run='.',
    [string]$TerminalLibrary=$env:WINONE_TERMINAL_LIBRARY,
    [string]$PDFWorker=$env:WINONE_PDF_WORKER,
    [string]$GoExe='dist/resources/tools/go-1.27.3/bin/go.exe'
)
# Targeted native regression entry; production builds still run the full suite.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$goPath=if([IO.Path]::IsPathRooted($GoExe)){$GoExe}else{Join-Path $root $GoExe}
$go=(Get-Command $goPath).Source
$priorRoot=$env:GOROOT; $priorCache=$env:GOCACHE; $priorTerminal=$env:WINONE_TERMINAL_LIBRARY; $priorPDF=$env:WINONE_PDF_WORKER
try {
    $env:GOROOT=Split-Path -Parent (Split-Path -Parent $go)
    $env:GOCACHE=Join-Path $root 'dist/resources/cache/go'
    & (Join-Path $PSScriptRoot 'prepare-mygo-filedrop.ps1') -GoExe $go
    if($Modules -contains 'desktop-mygo'){
        if(-not $TerminalLibrary){
            $terminal=Get-Content -LiteralPath (Join-Path $root 'third_party/native-terminal.json') -Raw -Encoding UTF8|ConvertFrom-Json
            $candidate=Join-Path $root ('dist/resources/runtimes/native-terminal/'+$terminal.library.sha256+'/'+$terminal.library.name)
            if(Test-Path -LiteralPath $candidate){$TerminalLibrary=$candidate}
        }
        if($PDFWorker){
            $resolved=(Resolve-Path -LiteralPath $PDFWorker).Path
            & node (Join-Path $PSScriptRoot 'verify-native-pdfium.mjs') --bundle (Split-Path -Parent $resolved)
            if($LASTEXITCODE-ne 0){throw 'Native PDF test bundle is not the reviewed version'}
            $env:WINONE_PDF_WORKER=$resolved
        }
        if($TerminalLibrary){
            & node (Join-Path $PSScriptRoot 'verify-native-terminal.mjs') --library $TerminalLibrary
            if($LASTEXITCODE-ne 0){throw 'Native terminal test library is not the reviewed version'}
            $env:WINONE_TERMINAL_LIBRARY=(Resolve-Path -LiteralPath $TerminalLibrary).Path
        }
    }
    foreach($module in $Modules) {
        Push-Location (Join-Path $root ('apps/'+$module))
        try {
            # PowerShell 5 turns redirected compiler stderr into errors. Keep
            # the real diagnostics, then fail on Go's exit code as other gates do.
            $previousErrorPreference=$ErrorActionPreference
            try {
                $ErrorActionPreference='Continue'
                & $go test $Packages -run $Run -count=1 -timeout 180s 2>&1 | ForEach-Object { $_.ToString() }
                $testExitCode=$LASTEXITCODE
            } finally { $ErrorActionPreference=$previousErrorPreference }
            if($testExitCode -ne 0){throw "$module tests failed"}
        } finally { Pop-Location }
    }
} finally { $env:GOROOT=$priorRoot; $env:GOCACHE=$priorCache; $env:WINONE_TERMINAL_LIBRARY=$priorTerminal; $env:WINONE_PDF_WORKER=$priorPDF }
