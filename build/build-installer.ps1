[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$Package,
    [Parameter(Mandatory=$true)][string]$NsisExe,
    [string]$GoExe='go',
    [string]$OutputDirectory='_temp/installer-build/output',
    [string]$TestDirectory
)
$ErrorActionPreference='Stop'
$root=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$output=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if ($TestDirectory) { & (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path ([IO.Path]::GetFullPath($TestDirectory)) }
& git -C $root check-ignore --quiet --no-index -- (Join-Path $output 'installer.probe')
if($LASTEXITCODE -ne 0){throw 'Installer output must be Git-ignored'}
$preparation=Join-Path $root ('_temp/installer-build/'+[Guid]::NewGuid().ToString('N'))
& (Join-Path $PSScriptRoot 'prepare-installer.ps1') -Package $Package -OutputDirectory $preparation -GoExe $GoExe
$updateHelper=Join-Path $preparation 'WinOne.Update.exe'
Push-Location $root
try {
    & $GoExe build -trimpath -ldflags '-H windowsgui' -o $updateHelper ./build/installer-update
    if($LASTEXITCODE -ne 0){throw 'Installer update helper build failed'}
} finally { Pop-Location }
$manifest=Get-Content -LiteralPath (Join-Path $preparation 'payload/manifest.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$manifestHash=(Get-FileHash -LiteralPath (Join-Path $preparation 'payload/manifest.json') -Algorithm SHA256).Hash.ToLowerInvariant()
$suffix=if($TestDirectory){'-windows-x64-test-setup.exe'}else{'-windows-x64-setup.exe'}
$target=Join-Path $output ('WinOne-'+$manifest.version+$suffix)
$uninstallerTarget=$target -replace '-setup\.exe$','-uninstall.exe'
$artifactRecord=$target+'.artifacts.json'
foreach($path in @($target,$uninstallerTarget,$uninstallerTarget+'.partial',$artifactRecord,$target+'.sha256',$uninstallerTarget+'.sha256')){
    if(Test-Path -LiteralPath $path){throw 'Installer output already exists'}
}
[IO.Directory]::CreateDirectory($output)|Out-Null
$testDefinitions=@("/DWINONE_UPDATE_HELPER=$updateHelper")
if($TestDirectory){
    $testRoot=[IO.Path]::GetFullPath($TestDirectory)
    & git -C $root check-ignore --quiet --no-index -- (Join-Path $testRoot 'test.probe')
    if($LASTEXITCODE -ne 0){throw 'TestDirectory must be Git-ignored'}
    if(Test-Path -LiteralPath $testRoot){throw 'TestDirectory already exists'}
    [IO.Directory]::CreateDirectory($testRoot)|Out-Null
    $context=@{ registryKey=('Software\WinOneInstallerTests\'+[Guid]::NewGuid().ToString('N')); profile=(Join-Path $testRoot 'profile'); shortcut=(Join-Path $testRoot 'WinOne.lnk'); installation=(Join-Path $testRoot 'installed'); installer=$target; onboarding=(Join-Path $testRoot 'machine/WinOne/onboarding') }
    [IO.File]::WriteAllText((Join-Path $testRoot 'context.json'),($context|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
    $testDefinitions+=@('/DWINONE_TEST_BUILD',"/DWINONE_REGISTRY_KEY=$($context.registryKey)","/DWINONE_PROFILE_DIR=$($context.profile)","/DWINONE_SHORTCUT=$($context.shortcut)")
}
# A compile-time path keeps full installer tests out of real ProgramData while
# using the exact same initializer and ACL policy as the production helper.
$onboardingHelper=Join-Path $preparation 'WinOne.Onboarding.exe'
$onboardingFlags='-H windowsgui'
if($TestDirectory){$onboardingFlags+=" -X 'winone/internal/onboarding.machineDirectoryOverride="+$context.onboarding.Replace('\','/')+"'"}
Push-Location $root
try{
    & $GoExe build -trimpath -ldflags $onboardingFlags -o $onboardingHelper ./build/installer-onboarding
    if($LASTEXITCODE -ne 0){throw 'Onboarding initializer build failed'}
}finally{Pop-Location}
$testDefinitions+="/DWINONE_ONBOARDING_HELPER=$onboardingHelper"
$previousExport=$env:WINONE_UNINSTALLER_EXPORT
# NSIS reads File sources during compilation. The installer's long-path
# manifest cannot affect that compiler process, so use the documented Unicode
# filesystem namespace for the already absolute, verified payload directory.
$payloadSource=[IO.Path]::GetFullPath((Join-Path $preparation 'payload'))
if(-not $payloadSource.StartsWith('\\?\')) {
    $payloadSource=if($payloadSource.StartsWith('\\')){'\\?\UNC\'+$payloadSource.Substring(2)}else{'\\?\'+$payloadSource}
}
try {
    $env:WINONE_UNINSTALLER_EXPORT=$uninstallerTarget
    & $NsisExe /V3 /INPUTCHARSET UTF8 /DWINONE_EXPORT_UNINSTALLER "/DWINONE_PAYLOAD_DIR=$payloadSource" "/DWINONE_FILES_NSH=$preparation\files.nsh" "/DWINONE_INSTALLER_OUT=$target" "/DWINONE_VERSION=$($manifest.version)" "/DWINONE_MANIFEST_SHA256=$manifestHash" "/DWINONE_ICON=$root\internal\branding\icon.ico" @testDefinitions (Join-Path $PSScriptRoot 'windows/installer.nsi')
    if($LASTEXITCODE -ne 0){throw 'Installer compilation failed'}
} finally { $env:WINONE_UNINSTALLER_EXPORT=$previousExport }
$digest=(Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant()
$uninstallerDigest=(Get-FileHash -LiteralPath $uninstallerTarget -Algorithm SHA256).Hash.ToLowerInvariant()
[IO.File]::WriteAllText($target+'.sha256',$digest+'  '+[IO.Path]::GetFileName($target)+"`n",[Text.UTF8Encoding]::new($false))
[IO.File]::WriteAllText($uninstallerTarget+'.sha256',$uninstallerDigest+'  '+[IO.Path]::GetFileName($uninstallerTarget)+"`n",[Text.UTF8Encoding]::new($false))
$artifacts=@{
    schemaVersion=1; version=$manifest.version; platform=$manifest.platform; packageManifestSHA256=$manifestHash; testBuild=[bool]$TestDirectory
    installer=@{fileName=[IO.Path]::GetFileName($target);size=(Get-Item -LiteralPath $target).Length;sha256=$digest}
    uninstaller=@{fileName=[IO.Path]::GetFileName($uninstallerTarget);size=(Get-Item -LiteralPath $uninstallerTarget).Length;sha256=$uninstallerDigest}
}
[IO.File]::WriteAllText($artifactRecord,($artifacts|ConvertTo-Json -Depth 4),[Text.UTF8Encoding]::new($false))
Write-Output "Installer compiled (installation lifecycle validation still required): $target"
Write-Output "Matching uninstaller exported: $uninstallerTarget"
