param(
    [Parameter(Mandatory)][string]$PythonExe,
    [Parameter(Mandatory)][string]$LibraryDirectory,
    [string]$OutputDirectory='_temp/anki-tests',
    [string]$GoExe=''
)
# This entry uses the actual Anki backend and temporary collections. It never
# opens a window, a listener, a real collection, or an online test endpoint.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
New-Item -ItemType Directory -Force $output | Out-Null
$priorTemp=$env:TEMP; $priorTmp=$env:TMP; $priorUTF8=$env:PYTHONUTF8; $priorBytecode=$env:PYTHONDONTWRITEBYTECODE
try {
    $env:TEMP=$output; $env:TMP=$output; $env:PYTHONUTF8='1'; $env:PYTHONDONTWRITEBYTECODE='1'
    $library=(Resolve-Path -LiteralPath $LibraryDirectory).Path
    $tests=Join-Path $root 'modules/anki'
    & $PythonExe -B -c 'import sys,unittest; sys.path[:0]=sys.argv[1:3]; suite=unittest.defaultTestLoader.discover(sys.argv[2]); result=unittest.TextTestRunner(stream=sys.stdout,verbosity=2).run(suite); sys.exit(not result.wasSuccessful())' $library $tests
    if($LASTEXITCODE -ne 0){throw 'Offline Anki tests failed'}
    if($GoExe){
        $priorPython=$env:WINONE_TEST_ANKI_PYTHON;$priorLibrary=$env:WINONE_TEST_ANKI_LIBRARY
        try {
            $env:WINONE_TEST_ANKI_PYTHON=(Resolve-Path -LiteralPath $PythonExe).Path
            $env:WINONE_TEST_ANKI_LIBRARY=$library
            Push-Location $root
            try { & $GoExe test ./internal/ankiengine -count=1; if($LASTEXITCODE -ne 0){throw 'Bundled Anki pipe/lifecycle tests failed'} } finally {Pop-Location}
        } finally {$env:WINONE_TEST_ANKI_PYTHON=$priorPython;$env:WINONE_TEST_ANKI_LIBRARY=$priorLibrary}
    }
} finally { $env:TEMP=$priorTemp; $env:TMP=$priorTmp; $env:PYTHONUTF8=$priorUTF8; $env:PYTHONDONTWRITEBYTECODE=$priorBytecode }
