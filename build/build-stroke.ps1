param(
 [Parameter(Mandatory)][string]$OutputDirectory,
 [string]$DotnetExe='dotnet',
 [string]$CompilerPath,
 [string]$LegacyReferenceDirectory,
 [string]$FrameworkDirectory=(Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319'),
 [switch]$Test
)
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'stroke-build.probe')
if($LASTEXITCODE-ne 0){throw 'Stroke output must be ignored'}
$manifest=Get-Content -LiteralPath (Join-Path $root 'third_party/stroke.json') -Raw -Encoding UTF8|ConvertFrom-Json
if($manifest.revision-ne 'cc72a7a73f98e6f9c5af0f9f1871bd006118b9c9'){throw 'Unexpected Stroke revision'}
foreach($file in $manifest.files){$actual=Get-FileHash -LiteralPath (Join-Path $root $file.vendored) -Algorithm SHA256;if($actual.Hash.ToLowerInvariant()-ne$file.sha256){throw ('Pinned Stroke source changed: '+$file.path)}}
if($manifest.releaseDefaults){$actual=Get-FileHash -LiteralPath (Join-Path $root $manifest.releaseDefaults.decodedPath) -Algorithm SHA256;if($actual.Hash.ToLowerInvariant()-ne$manifest.releaseDefaults.decodedSHA256){throw 'Decoded official Stroke defaults changed'}}
if(-not(Test-Path -LiteralPath (Join-Path $FrameworkDirectory 'mscorlib.dll'))){throw '.NET Framework 4.x references are unavailable'}
if(-not $CompilerPath){$sdk=& $DotnetExe --list-sdks|Where-Object {$_-match '^\d+\.\d+\.\d+ \['}|Select-Object -Last 1;if($sdk-match '^(\S+) \[(.+)\]$'){$CompilerPath=Join-Path $Matches[2] ($Matches[1]+'/Roslyn/bincore/csc.dll')}}
if(-not $CompilerPath -or -not(Test-Path -LiteralPath $CompilerPath)){throw 'Pass an installed Roslyn csc.dll path; no compiler will be downloaded'}
New-Item -ItemType Directory -Force -Path $output,(Join-Path $output 'sources')|Out-Null
$module=Join-Path $root 'modules/stroke'
$patched=Join-Path $output 'sources'
$encoding=[Text.UTF8Encoding]::new($false)
# Apply only reviewed embedding changes; canonical upstream files remain intact.
$api=[IO.File]::ReadAllText((Join-Path $module 'upstream/API.cs'))
$inputLayout=[regex]::Match($api,'(?s)public struct INPUT\s*\{.*?\n        \}').Value
if(([regex]::Matches($inputLayout,'\[FieldOffset\(4\)\]')).Count-ne 3){throw 'Upstream INPUT layout changed'}
$api=$api.Replace($inputLayout,$inputLayout.Replace('[FieldOffset(4)]','[FieldOffset(8)]'))
$api=[regex]::Replace($api,'\[DllImport\("user32.dll", CharSet = CharSet.Auto, SetLastError = true\)\]\s+public static extern uint SendInput\(uint cInputs, ref INPUT pInput, int cbSize\);','[DllImport("user32.dll", EntryPoint="SendInput", SetLastError=true)] public static extern uint NativeSendInput(uint count, ref INPUT input, int size); public static uint SendInput(uint count, ref INPUT input, int size) { if(count!=1 || size!=Marshal.SizeOf(typeof(INPUT))) throw new ArgumentException("Invalid INPUT"); return InputRouting.Send(input); }')
[IO.File]::WriteAllText((Join-Path $patched 'API.cs'),$api,$encoding)
$draw=[IO.File]::ReadAllText((Join-Path $module 'upstream/Draw.cs')).Replace('originalPen = API.SelectObject(canvas, pen);','IntPtr previous = API.SelectObject(canvas, pen); if (originalPen == IntPtr.Zero) originalPen = previous;').Replace('if (canvas != IntPtr.Zero)', 'if (pen != IntPtr.Zero) API.DeleteObject(pen);'+[Environment]::NewLine+'                if (canvas != IntPtr.Zero)')
[IO.File]::WriteAllText((Join-Path $patched 'Draw.cs'),$draw,$encoding)
$hook=[IO.File]::ReadAllText((Join-Path $module 'upstream/MouseHook.cs')).Replace('Enable = true;','if (hook == IntPtr.Zero) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error(), "Mouse hook registration failed"); Enable = true;').Replace('API.UnhookWindowsHookEx(hook);','if (!API.UnhookWindowsHookEx(hook)) { int code=Marshal.GetLastWin32Error(); if(code!=1404 && code!=6) throw new System.ComponentModel.Win32Exception(code, "Mouse hook removal failed"); } hook = IntPtr.Zero;')
$hook=$hook.Replace('private static IntPtr hook = IntPtr.Zero;','private static IntPtr hook = IntPtr.Zero; private static long lastEventTimestamp; public static long LastEventTimestamp {get{return System.Threading.Interlocked.Read(ref lastEventTimestamp);}}')
$hook=[regex]::Replace($hook,'if \(nCode >= 0\)\s*\{',('$&'+"`n                System.Threading.Interlocked.Exchange(ref lastEventTimestamp, Stopwatch.GetTimestamp());"))
[IO.File]::WriteAllText((Join-Path $patched 'MouseHook.cs'),$hook,$encoding)
$base=[IO.File]::ReadAllText((Join-Path $module 'upstream/Base.cs')).Replace('Enable = true;','if (hook == IntPtr.Zero) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error(), "Keyboard hook registration failed"); Enable = true;').Replace('API.UnhookWindowsHookEx(hook);','if (!API.UnhookWindowsHookEx(hook)) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error(), "Keyboard hook removal failed"); hook = IntPtr.Zero;')
[IO.File]::WriteAllText((Join-Path $patched 'Base.cs'),$base,$encoding)
$references=@('mscorlib.dll','System.dll','System.Core.dll','System.Drawing.dll','System.Windows.Forms.dll','System.Web.Extensions.dll','System.Xml.dll','Microsoft.CSharp.dll')|ForEach-Object {'/reference:'+(Join-Path $FrameworkDirectory $_)}
$common=@('/nologo','/noconfig','/nostdlib+','/langversion:latest','/platform:x64','/optimize+','/debug-')+$references
$core=@('Gesture.cs','Pen.cs','Action.cs','ActionPackage.cs','SpecialGesture.cs')|ForEach-Object {Join-Path $module ('upstream/'+$_)}
$core+=@('API.cs','Draw.cs','MouseHook.cs','Base.cs')|ForEach-Object {Join-Path $patched $_}
$core+=@('AssemblyInfo.cs','CoreState.cs','Configuration.cs','ScopePolicy.cs','GestureEngine.cs','HookRecovery.cs')|ForEach-Object {Join-Path $module $_}
& $DotnetExe $CompilerPath @common /target:library ('/out:'+(Join-Path $output 'Stroke.Base.dll')) @core
if($LASTEXITCODE-ne 0){throw 'Stroke base compilation failed'}
foreach($name in @('Base','Stroke')){
 & $DotnetExe $CompilerPath @common ('/reference:'+(Join-Path $output 'Stroke.Base.dll')) /target:library ('/out:'+(Join-Path $output ($name+'.dll'))) (Join-Path $module ('compatibility/'+$name+'Forwarders.cs'))
 if($LASTEXITCODE-ne 0){throw 'Stroke compatibility facade compilation failed'}
}
Copy-Item -LiteralPath (Join-Path $module 'defaults.json') -Destination $output
Copy-Item -LiteralPath (Join-Path $module 'upstream/LICENSE') -Destination (Join-Path $output 'Stroke-LICENSE.txt')
$runtime=@('NativeWindow.cs','Protocol.cs','InputLedger.cs','ScriptHost.cs','ScriptSupervisor.cs','MouseHookLoop.cs','Program.cs')|ForEach-Object {Join-Path $module $_}
 & $DotnetExe $CompilerPath @common ('/reference:'+(Join-Path $output 'Stroke.Base.dll')) ('/win32manifest:'+(Join-Path $module 'app.manifest')) /target:winexe /main:WinOne.StrokeRuntime.Program ('/out:'+(Join-Path $output 'WinOne.Stroke.exe')) @runtime
 if($LASTEXITCODE-ne 0){throw 'Stroke worker compilation failed'}
if($Test){
 New-Item -ItemType Directory -Force -Path (Join-Path $output 'tests')|Out-Null
 & $DotnetExe $CompilerPath @common /target:library ('/out:'+(Join-Path $output 'tests/FixturePlugin.dll')) (Join-Path $module 'tests/FixturePlugin.cs')
 if($LASTEXITCODE-ne 0){throw 'Stroke plugin fixture did not compile'}
 & $DotnetExe $CompilerPath @common /target:winexe ('/out:'+(Join-Path $output 'tests/FixtureChild.exe')) (Join-Path $module 'tests/FixtureChild.cs')
 if($LASTEXITCODE-ne 0){throw 'Stroke independent-process fixture did not compile'}
 if($LegacyReferenceDirectory){
  foreach($reference in $manifest.releaseReferences){$actual=Get-FileHash -LiteralPath (Join-Path $LegacyReferenceDirectory $reference.name) -Algorithm SHA256;if($actual.Hash.ToLowerInvariant()-ne$reference.sha256){throw 'Legacy plugin reference differs from the pinned release'}}
  & $DotnetExe $CompilerPath @common ('/reference:'+(Join-Path $LegacyReferenceDirectory 'Base.dll')) ('/reference:'+(Join-Path $LegacyReferenceDirectory 'Stroke.exe')) /target:library ('/out:'+(Join-Path $output 'tests/FixtureLegacyPlugin.dll')) (Join-Path $module 'tests/FixtureLegacyPlugin.cs')
  if($LASTEXITCODE-ne 0){throw 'Legacy Stroke plugin fixture did not compile'}
 }
 $testSources=@((Join-Path $module 'NativeWindow.cs'),(Join-Path $module 'InputLedger.cs'),(Join-Path $module 'Tests.cs'))
 & $DotnetExe $CompilerPath @common ('/reference:'+(Join-Path $output 'Stroke.Base.dll')) ('/win32manifest:'+(Join-Path $module 'app.manifest')) /target:exe /main:WinOne.StrokeRuntime.StrokeTests ('/out:'+(Join-Path $output 'WinOne.Stroke.Tests.exe')) @testSources
 if($LASTEXITCODE-ne 0){throw 'Stroke tests did not compile'}
 & (Join-Path $output 'WinOne.Stroke.Tests.exe')
 if($LASTEXITCODE-ne 0){throw 'Stroke behavior tests failed'}
}
foreach($file in @('schema.json','migrations.json','README.md')){if(Test-Path -LiteralPath (Join-Path $module $file)){Copy-Item -LiteralPath (Join-Path $module $file) -Destination $output}}
