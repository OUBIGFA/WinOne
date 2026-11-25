param([string]$Downloader,[switch]$BuildDependencies,[string]$VcpkgExe)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$sourceRoot=Join-Path $root 'dist/resources/sources'
$source=Join-Path $sourceRoot 'Tools3000-v1.4.11'
$lock=Get-Content -LiteralPath (Join-Path $root 'third_party/tools3000/source.json') -Raw -Encoding utf8|ConvertFrom-Json
$archive=Join-Path $sourceRoot 'Tools3000-v1.4.11.tar.gz'
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $source -ReusableCache
New-Item -ItemType Directory -Force -Path $sourceRoot|Out-Null
if(-not(Test-Path -LiteralPath $source)){
    if(-not(Test-Path -LiteralPath $archive)){
        if(-not $Downloader){$found=Get-Command aria2-next -ErrorAction SilentlyContinue;if($found){$Downloader=$found.Source}}
        if(-not $Downloader){throw 'Pass the aria2-next downloader path to prepare the pinned community source'}
        & $Downloader -d $sourceRoot -o 'Tools3000-v1.4.11.tar.gz' --auto-file-renaming=false --allow-overwrite=true $lock.archiveURL
        if($LASTEXITCODE){throw 'Community source download failed'}
    }
    if((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $lock.archiveSHA256){throw 'Community source archive checksum differs; preserve it for inspection'}
    New-Item -ItemType Directory -Path $source|Out-Null
    & tar -xf $archive -C $source --strip-components=1
    if($LASTEXITCODE){throw 'Community source extraction failed'}
}
foreach($file in $lock.files){
    if((Get-FileHash -LiteralPath (Join-Path $source $file.path) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $file.sha256){throw "Pinned community source differs: $($file.path)"}
}
if($BuildDependencies){
    if(-not $VcpkgExe){
        $vswhere=Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
        $vs=& $vswhere -latest -prerelease -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
        $VcpkgExe=Join-Path $vs 'VC/vcpkg/vcpkg.exe'
    }
    $cache=Join-Path $root 'dist/resources/cache/capture-native'
    New-Item -ItemType Directory -Force -Path $cache|Out-Null
    $mapping=$null;$workingRoot=$root
    $keys=@('VCPKG_ROOT','VCPKG_DOWNLOADS','X_VCPKG_REGISTRIES_CACHE','VCPKG_BINARY_SOURCES','VCPKG_MAX_CONCURRENCY','VCPKG_DISABLE_METRICS')
    $prior=@{};foreach($key in $keys){$prior[$key]=[Environment]::GetEnvironmentVariable($key,'Process')}
    try {
        # x264's upstream configure/make requires a path without spaces. Use
        # one temporary alias, never relocate the checkout or disable assembly.
        if($root.Contains(' ')){
            foreach($letter in @('Z','Y','X','V','U','T','S','R','Q')){
                $drive=$letter+':'
                if(-not(Test-Path -LiteralPath ($drive+'\'))){
                    & subst $drive $root
                    if($LASTEXITCODE){throw 'Cannot create the temporary native dependency path'}
                    $mapping=$drive;$workingRoot=$drive+'\';break
                }
            }
            if(-not $mapping){throw 'A free drive letter is needed by the upstream dependency build'}
        }
        $workingCache=Join-Path $workingRoot 'dist/resources/cache/capture-native'
        $env:VCPKG_ROOT=Split-Path -Parent $VcpkgExe
        $env:VCPKG_DOWNLOADS=Join-Path $workingCache 'downloads'
        $env:X_VCPKG_REGISTRIES_CACHE=Join-Path $workingCache 'registries'
        $env:VCPKG_BINARY_SOURCES='clear;files,'+(Join-Path $workingCache 'binary')+',readwrite'
        $env:VCPKG_MAX_CONCURRENCY='4';$env:VCPKG_DISABLE_METRICS='1'
        foreach($folder in @($env:VCPKG_DOWNLOADS,$env:X_VCPKG_REGISTRIES_CACHE,(Join-Path $workingCache 'binary'))){New-Item -ItemType Directory -Force -Path $folder|Out-Null}
        & $VcpkgExe install --triplet=x64-windows-winone "--overlay-triplets=$(Join-Path $workingRoot 'build/capture-native-triplets')" `
            "--x-manifest-root=$(Join-Path $workingRoot 'dist/resources/sources/Tools3000-v1.4.11')" "--x-install-root=$workingCache/installed" `
            "--x-buildtrees-root=$workingCache/buildtrees" "--x-packages-root=$workingCache/packages" --disable-metrics
        if($LASTEXITCODE){throw 'Pinned native dependency build failed'}
    } finally {
        foreach($key in $keys){[Environment]::SetEnvironmentVariable($key,$prior[$key],'Process')}
        if($mapping){
            $line=@(& subst)|Where-Object {$_ -like ($mapping+'\:*')}
            if($line -and ($line -split ' => ',2)[1].TrimEnd('\') -eq $root.TrimEnd('\')){& subst $mapping /D}
            else{Write-Warning 'Temporary drive mapping changed; it was preserved'}
        }
    }
}
Write-Output $source
