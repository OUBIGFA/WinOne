[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$Package,
    [Parameter(Mandatory=$true)][string]$NsisExe,
    [string]$GoExe='go',
    [string]$OutputDirectory='dist/installers',
    [string]$TestDirectory
)
$ErrorActionPreference='Stop'
$root=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$output=[IO.Path]::GetFullPath($OutputDirectory)
& git -C $root check-ignore --quiet --no-index -- (Join-Path $output 'installer.probe')
if($LASTEXITCODE -ne 0){throw 'Installer output must be Git-ignored'}
$preparation=Join-Path $root ('_temp/installer-build/'+[Guid]::NewGuid().ToString('N'))
& (Join-Path $PSScriptRoot 'prepare-installer.ps1') -Package $Package -OutputDirectory $preparation -GoExe $GoExe
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
$testDefinitions=@()
if($TestDirectory){
    $testRoot=[IO.Path]::GetFullPath($TestDirectory)
    & git -C $root check-ignore --quiet --no-index -- (Join-Path $testRoot 'test.probe')
    if($LASTEXITCODE -ne 0){throw 'TestDirectory must be Git-ignored'}
    if(Test-Path -LiteralPath $testRoot){throw 'TestDirectory already exists'}
    [IO.Directory]::CreateDirectory($testRoot)|Out-Null
    $context=@{ registryKey=('Software\WinOneInstallerTests\'+[Guid]::NewGuid().ToString('N')); profile=(Join-Path $testRoot 'profile'); shortcut=(Join-Path $testRoot 'WinOne.lnk'); installation=(Join-Path $testRoot 'installed'); installer=$target }
    [IO.File]::WriteAllText((Join-Path $testRoot 'context.json'),($context|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
    $testDefinitions=@('/DWINONE_TEST_BUILD',"/DWINONE_REGISTRY_KEY=$($context.registryKey)","/DWINONE_PROFILE_DIR=$($context.profile)","/DWINONE_SHORTCUT=$($context.shortcut)")
}
$previousExport=$env:WINONE_UNINSTALLER_EXPORT
try {
    $env:WINONE_UNINSTALLER_EXPORT=$uninstallerTarget
    & $NsisExe /V3 /INPUTCHARSET UTF8 /DWINONE_EXPORT_UNINSTALLER "/DWINONE_PAYLOAD_DIR=$preparation\payload" "/DWINONE_FILES_NSH=$preparation\files.nsh" "/DWINONE_INSTALLER_OUT=$target" "/DWINONE_VERSION=$($manifest.version)" "/DWINONE_MANIFEST_SHA256=$manifestHash" "/DWINONE_ICON=$root\internal\branding\icon.ico" @testDefinitions (Join-Path $PSScriptRoot 'windows/installer.nsi')
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
