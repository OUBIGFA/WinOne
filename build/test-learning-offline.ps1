param(
    [string]$GoExe='go',
    [string]$ResultGoExe='dist/resources/tools/go-1.27.3/bin/go.exe',
    [string]$PythonExe='dist/resources/runtimes/ocr-3.10.0/python/python.exe',
    [string]$LibraryDirectory='dist/resources/runtimes/anki-26.09.4/lib',
    [string]$WorkerDirectory='modules/anki',
    [string]$ConnectSource='third_party/anki-connect/plugin',
    [string]$OutputDirectory='_temp/learning-offline'
)
# User-selectable offline gate. No network listeners, elevation, visible windows,
# audio playback, installer execution, Explorer actions, or real profile changes.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
New-Item -ItemType Directory -Force $output|Out-Null
$prior=@{};foreach($key in @('TEMP','TMP','GOPROXY','GOSUMDB','GOTOOLCHAIN','WINONE_TEST_LEARNING_DICTIONARY','WINONE_TEST_ANKI_PYTHON','WINONE_TEST_ANKI_LIBRARY','WINONE_TEST_ANKI_WORKER','WINONE_TEST_ANKI_SOURCE','WINONE_SHOTS','WINONE_TEST_SPEECH_ONLINE','WINONE_TEST_SPEECH_PLAYBACK','WINONE_AI_NATIVE_WINDOW','WINONE_RESULT_PROBE_EXE')){$prior[$key]=[Environment]::GetEnvironmentVariable($key,'Process')}
try {
    $env:TEMP=$output;$env:TMP=$output;$env:GOPROXY='off';$env:GOSUMDB='off';$env:GOTOOLCHAIN='local'
    $env:WINONE_TEST_SPEECH_ONLINE='';$env:WINONE_TEST_SPEECH_PLAYBACK='';$env:WINONE_AI_NATIVE_WINDOW='';$env:WINONE_RESULT_PROBE_EXE=''
    $env:WINONE_TEST_LEARNING_DICTIONARY=Join-Path $root 'dist/resources/assets/learning/ecdict.db'
    $env:WINONE_TEST_ANKI_PYTHON=(Resolve-Path -LiteralPath $PythonExe).Path
    $env:WINONE_TEST_ANKI_LIBRARY=(Resolve-Path -LiteralPath $LibraryDirectory).Path
    $env:WINONE_TEST_ANKI_WORKER=(Resolve-Path -LiteralPath (Join-Path $WorkerDirectory 'worker.py')).Path
    $env:WINONE_TEST_ANKI_SOURCE=(Resolve-Path -LiteralPath $ConnectSource).Path
    & (Join-Path $PSScriptRoot 'test-anki.ps1') -PythonExe $PythonExe -LibraryDirectory $LibraryDirectory -OutputDirectory (Join-Path $output 'engine') -GoExe $GoExe
    Push-Location $root
    try {
        & $GoExe test ./internal/learning ./internal/resultwindow ./internal/quickassistant ./internal/distribution -run '^(TestClassify|TestDefault|TestOptions|TestPrompt|TestTemplate|TestDictionary|TestExtract|TestLearning|TestTerm|TestAnkiPayload|TestCorruptTerm|TestIgnoredVocabulary|Test.*Learning)' -count=1
        if($LASTEXITCODE -ne 0){throw 'Offline learning/data/distribution regressions failed'}
        & $GoExe test ./apps/host -run '^TestResultLearning' -count=1
        if($LASTEXITCODE -ne 0){throw 'Result learning ownership checks failed'}
    } finally {Pop-Location}
    $env:WINONE_SHOTS=Join-Path $output 'ui'
    New-Item -ItemType Directory -Force $env:WINONE_SHOTS|Out-Null
    & (Join-Path $PSScriptRoot 'test-native-ui.ps1') -Modules desktop-mygo -Packages ./studybook,./pages/selection,./i18n -GoExe $ResultGoExe
    & (Join-Path $PSScriptRoot 'test-native-ui.ps1') -Modules desktop-mygo -Packages ./pages/aiservices -Run '^TestAIHome' -GoExe $ResultGoExe
    & (Join-Path $PSScriptRoot 'test-native-ui.ps1') -Modules result-ui -Packages . -Run '^TestLearning|^TestStudy|^Test.*Study|^Test.*Learning' -GoExe $ResultGoExe
} finally {foreach($key in $prior.Keys){[Environment]::SetEnvironmentVariable($key,$prior[$key],'Process')}}
