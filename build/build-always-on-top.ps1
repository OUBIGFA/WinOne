param([string]$MSBuild, [string]$OutputDirectory = 'dist/build/always-on-top')
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$checkout = & (Join-Path $PSScriptRoot 'prepare-powertoys.ps1')
$source = (Get-Content -LiteralPath (Join-Path $root 'third_party/sources.json') -Raw -Encoding UTF8 | ConvertFrom-Json).powertoys
$native = Join-Path $root 'modules/always-on-top/native'
$module = 'src/modules/alwaysontop/AlwaysOnTop/'
function Original([string]$path) {
    $lines = & git -C $checkout show ($source.commit + ':' + $path)
    if ($LASTEXITCODE -ne 0) { throw "Cannot read pinned source: $path" }
    return ($lines -join "`n") + "`n"
}
function WriteSource([string]$path, [string]$content) {
    $target = Join-Path $checkout $path
    if ([IO.File]::ReadAllText($target) -ne $content) { [IO.File]::WriteAllText($target, $content, [Text.UTF8Encoding]::new($false)) }
}
function Section([string]$text, [string]$start, [string]$end, [string]$replacement) {
    $first = $text.IndexOf($start); $last = $text.IndexOf($end, $first + $start.Length)
    if ($first -lt 0 -or $last -lt 0) { throw "Upstream section changed: $start" }
    return $text.Substring(0, $first) + $replacement + "`n`n" + $text.Substring($last)
}
$header = Original ($module + 'AlwaysOnTop.h')
$header = $header.Replace('    ~AlwaysOnTop();', @'
    ~AlwaysOnTop();
    bool IsReadyForWinOne() const { return m_window != nullptr; }
    HRESULT SetOwnedPin(HWND window, bool enabled);
    HRESULT ClearOwnedPins();
'@)
foreach ($handle in 'm_hPinEvent','m_hTerminateEvent','m_hIncreaseOpacityEvent','m_hDecreaseOpacityEvent') { $header = $header.Replace("HANDLE $handle;", "HANDLE $handle{ nullptr };") }
WriteSource ($module + 'AlwaysOnTop.h') $header
$settings = (Original ($module + 'Settings.h')).Replace('    void LoadSettings();', "    void LoadSettings();`n    void ApplyWinOneSettings(const Settings& settings);")
WriteSource ($module + 'Settings.h') $settings
$core = Original ($module + 'AlwaysOnTop.cpp')
$core = $core.Replace('L"AlwaysOnTopWindow"', 'L"WinOneAlwaysOnTopWindow"').Replace('const static wchar_t* WINDOW_IS_PINNED_PROP = L"AlwaysOnTop_Pinned";', 'static std::wstring WINDOW_IS_PINNED_PROP = L"WinOne_AlwaysOnTop_Pinned";')
$core = $core.Replace('NonLocalizable::WINDOW_IS_PINNED_PROP,', 'NonLocalizable::WINDOW_IS_PINNED_PROP.c_str(),').Replace('NonLocalizable::WINDOW_IS_PINNED_PROP)', 'NonLocalizable::WINDOW_IS_PINNED_PROP.c_str())')
$core = $core.Replace('0xEFE0', '0xEFE1').Replace('0x414F5450', '0x574F5450')
$core = $core.Replace('        AlwaysOnTopSettings::instance().InitFileWatcher();', '').Replace('        AlwaysOnTopSettings::instance().LoadSettings();', '')
$core = $core.Replace('    m_thread.join();', '    if (m_thread.joinable()) m_thread.join();').Replace('    m_running = false;', "    m_running = false;`n    s_instance = nullptr;")
$core = Section $core 'void AlwaysOnTop::RegisterHotkey() const' 'void AlwaysOnTop::RegisterLLKH()' 'void AlwaysOnTop::RegisterHotkey() const {}'
$core = Section $core 'void AlwaysOnTop::RegisterLLKH()' 'void AlwaysOnTop::SubscribeToEvents()' 'void AlwaysOnTop::RegisterLLKH() {}'
$core = $core.Replace('LRESULT AlwaysOnTop::WndProc(HWND window, UINT message, WPARAM wparam, LPARAM lparam) noexcept' + "`n{", 'LRESULT AlwaysOnTop::WndProc(HWND window, UINT message, WPARAM wparam, LPARAM lparam) noexcept' + "`n{`n    if (message == WM_HOTKEY) return 0;")
$core = Section $core 'bool AlwaysOnTop::PinTopmostWindow(HWND window) const noexcept' 'bool AlwaysOnTop::UnpinTopmostWindow(HWND window) const noexcept' @'
bool AlwaysOnTop::PinTopmostWindow(HWND window) const noexcept
{
    const bool alreadyOwned = IsPinned(window);
    if (!SetProp(window, NonLocalizable::WINDOW_IS_PINNED_PROP.c_str(), reinterpret_cast<HANDLE>(1))) return false;
    const bool moved = SetWindowPos(window, HWND_TOPMOST, 0, 0, 0, 0, SWP_NOMOVE | SWP_NOSIZE | SWP_NOACTIVATE);
    if (!moved || !IsTopmost(window))
    {
        const auto error = moved ? ERROR_INVALID_STATE : GetLastError();
        if (!alreadyOwned) RemoveProp(window, NonLocalizable::WINDOW_IS_PINNED_PROP.c_str());
        SetLastError(error); return false;
    }
    return true;
}
'@
$core = Section $core 'bool AlwaysOnTop::UnpinTopmostWindow(HWND window) const noexcept' 'bool AlwaysOnTop::IsTracked(HWND window) const noexcept' @'
bool AlwaysOnTop::UnpinTopmostWindow(HWND window) const noexcept
{
    if (!IsPinned(window)) return true;
    if (!SetWindowPos(window, HWND_NOTOPMOST, 0, 0, 0, 0, SWP_NOMOVE | SWP_NOSIZE | SWP_NOACTIVATE)) return false;
    if (IsTopmost(window)) { SetLastError(ERROR_INVALID_STATE); return false; }
    RemoveProp(window, NonLocalizable::WINDOW_IS_PINNED_PROP.c_str());
    return true;
}
'@
$core = $core.Replace('bool AlwaysOnTop::AssignBorder(HWND window)' + "`n{", 'bool AlwaysOnTop::AssignBorder(HWND window)' + "`n{`n    m_topmostWindows[window] = nullptr;")
$core = $core.Replace('if (m_virtualDesktopUtils.IsWindowOnCurrentDesktop(window) && AlwaysOnTopSettings::settings()->enableFrame)', 'if (IsWindowVisible(window) && m_virtualDesktopUtils.IsWindowOnCurrentDesktop(window) && AlwaysOnTopSettings::settings()->enableFrame)')
$core = Section $core 'void AlwaysOnTop::StartTrackingTopmostWindows()' 'bool AlwaysOnTop::AssignBorder(HWND window)' @'
void AlwaysOnTop::StartTrackingTopmostWindows()
{
    std::vector<HWND> candidates;
    EnumWindows([](HWND window, LPARAM data) -> BOOL {
        reinterpret_cast<std::vector<HWND>*>(data)->push_back(window);
        return TRUE;
    }, reinterpret_cast<LPARAM>(&candidates));
    for (const auto window : candidates)
    {
        if (!IsPinned(window)) continue;
        // Hidden windows may belong to WinTray. Retain them for cleanup after
        // a worker crash, without drawing a border over an invisible window.
        if (IsWindowVisible(window) && !isExcluded(window)) AssignBorder(window);
        else m_topmostWindows[window] = nullptr;
    }
}
'@
$core = $core.Replace('            m_topmostWindows[window] = std::move(border);' + "`n        }", '            m_topmostWindows[window] = std::move(border);' + "`n        }`n        else { return false; }")
$core += "`nvoid WinOneSetOwnershipKey(const wchar_t* key) { NonLocalizable::WINDOW_IS_PINNED_PROP = key; }`n"
WriteSource ($module + 'AlwaysOnTop.cpp') $core
$helper = Original 'src/common/SettingsAPI/settings_helpers.cpp'
$helper = Section $helper '    std::wstring get_root_save_folder_location()' '    std::wstring get_local_low_folder_location()' @'
    std::wstring get_root_save_folder_location()
    {
        const auto size = GetEnvironmentVariableW(L"WINONE_AOT_DATA", nullptr, 0);
        if (!size) throw winrt::hresult_error(E_INVALIDARG);
        std::wstring result(size, L'\0');
        const auto length = GetEnvironmentVariableW(L"WINONE_AOT_DATA", result.data(), size);
        if (!length || length >= size) throw winrt::hresult_error(E_INVALIDARG);
        result.resize(length);
        if (!std::filesystem::path(result).is_absolute()) throw winrt::hresult_error(E_INVALIDARG);
        std::filesystem::create_directories(result);
        return result;
    }
'@
$helper = Section $helper '    std::wstring get_local_low_folder_location()' '    std::wstring get_module_save_folder_location' @'
    std::wstring get_local_low_folder_location() { return get_root_save_folder_location(); }
'@
WriteSource 'src/common/SettingsAPI/settings_helpers.cpp' $helper
$project = Original ($module + 'AlwaysOnTop.vcxproj')
$project = $project.Replace('<ConfigurationType>Application</ConfigurationType>', '<ConfigurationType>DynamicLibrary</ConfigurationType>').Replace('<TargetName>PowerToys.$(MSBuildProjectName)</TargetName>', '<TargetName>WinOne.AlwaysOnTop.Native</TargetName>')
$project = $project.Replace('<ClCompile Include="main.cpp" />', '<ClCompile Include="$(WinOneNativeSource)\Bridge.cpp" /><ClCompile Include="$(WinOneNativeSource)\OwnedPin.cpp" />')
$project = $project.Replace('<AdditionalIncludeDirectories>', '<AdditionalIncludeDirectories>$(MSBuildProjectDirectory);')
$project = $project.Replace('powershell -NonInteractive -executionpolicy Unrestricted -NoProfile $(RepoRoot)tools\build\convert-resx-to-rc.ps1 $(MSBuildThisFileDirectory)', 'powershell -NonInteractive -executionpolicy Unrestricted -NoProfile -File &quot;$(RepoRoot)tools\build\convert-resx-to-rc.ps1&quot; &quot;$(MSBuildThisFileDirectory).&quot;')
WriteSource ($module + 'AlwaysOnTop.vcxproj') $project
if (-not $MSBuild) {
    $vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
    $installation = & $vswhere -latest -prerelease -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
    if (-not $installation) { throw 'Install Visual Studio C++ build tools and Windows SDK 26100' }
    $MSBuild = Join-Path $installation 'MSBuild/Current/Bin/MSBuild.exe'
}
& $MSBuild (Join-Path $checkout ($module + 'AlwaysOnTop.vcxproj')) /restore /t:Build /p:Configuration=Release /p:Platform=x64 /p:RestorePackagesConfig=true "/p:RestoreRepositoryPath=$checkout\packages" "/p:WinOneNativeSource=$native" /verbosity:minimal /nologo
if ($LASTEXITCODE -ne 0) { throw 'AlwaysOnTop native build failed' }
$output = if ([IO.Path]::IsPathRooted($OutputDirectory)) { $OutputDirectory } else { Join-Path $root $OutputDirectory }
New-Item -ItemType Directory -Path $output -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $checkout 'x64/Release/WinOne.AlwaysOnTop.Native.dll') -Destination $output
