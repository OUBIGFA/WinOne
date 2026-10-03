function Update-ShareXNativePresentation([string]$Path) {
    $text=[IO.File]::ReadAllText($Path)
    if($text.Contains('WinOnePresentation')) { return }
    $changes=[ordered]@{
        'public sealed class RegionCaptureForm : Form'='public sealed partial class RegionCaptureForm : Form'
        '            Update();'='            if (!WinOnePresentation) Update();'
        '            DrawBackground(g);'="            DrawBackground(g);`n            if (WinOnePresentation) DrawWinOneCandidate(g);"
        '            if (!pause)'='            if (!pause && !WinOnePresentation)'
        '                g.DrawPath(borderDotStaticPen, regionDrawPath);'='                if (!WinOnePresentation) g.DrawPath(borderDotStaticPen, regionDrawPath);'
        '                    g.DrawPath(borderDotPen, hoverDrawPath);'='                    if (!WinOnePresentation) g.DrawPath(borderDotPen, hoverDrawPath);'
        '                        g.Clip = region;'="                        var clipState = g.Save();`n                        g.SetClip(region, CombineMode.Intersect);"
        '                        g.ResetClip();'='                        g.Restore(clipState);'
    }
    foreach($old in $changes.Keys){
        if(-not $text.Contains($old)){throw ('ShareX presentation anchor changed: '+$old)}
        $text=$text.Replace($old,$changes[$old])
    }
    [IO.File]::WriteAllText($Path,$text,[Text.UTF8Encoding]::new($false))
}

function Update-ShareXNativeEdgeSnap([string]$Path) {
    $text=[IO.File]::ReadAllText($Path)
    if($text.Contains('SnapWinOneSelectionEdge(shape);')) { return }
    $anchor='                    shape.OnNodeUpdate();'
    if(-not $text.Contains($anchor)){throw 'ShareX node resize anchor changed'}
    $text=$text.Replace($anchor, $anchor+"`n                    SnapWinOneSelectionEdge(shape);")
    [IO.File]::WriteAllText($Path,$text,[Text.UTF8Encoding]::new($false))
}

function Update-ShareXNativePointer([string]$Path) {
    $text=[IO.File]::ReadAllText($Path)
    if($text.Contains('if (WinOnePresentation) { SetWinOneDefaultCursor(); return; }')) { return }
    $anchor='public void SetDefaultCursor()'
    $pattern='public void SetDefaultCursor\(\)\s*\{'
    if(-not $text.Contains($anchor)){throw 'ShareX default cursor anchor changed'}
    $text=[regex]::Replace($text,$pattern,"public void SetDefaultCursor()`n        {`n            if (WinOnePresentation) { SetWinOneDefaultCursor(); return; }")
    [IO.File]::WriteAllText($Path,$text,[Text.UTF8Encoding]::new($false))
}

function Update-ShareXNativeResizePointer([string]$Path) {
    $text=[IO.File]::ReadAllText($Path)
    if($text.Contains('if (WinOnePresentation && SetWinOneResizeCursor()) return;')) { return }
    $pattern='public void SetHandCursor\(bool grabbing\)\s*\{'
    if(-not [regex]::IsMatch($text,$pattern)){throw 'ShareX handle cursor anchor changed'}
    $text=[regex]::Replace($text,$pattern,"public void SetHandCursor(bool grabbing)`n        {`n            if (WinOnePresentation && SetWinOneResizeCursor()) return;")
    [IO.File]::WriteAllText($Path,$text,[Text.UTF8Encoding]::new($false))
}

function Update-ShareXNativeEventInput([string]$Directory) {
    $inputPath=Join-Path $Directory 'ShareX.ScreenCaptureLib/Helpers/InputManager.cs'
    $text=[IO.File]::ReadAllText($inputPath)
    if(-not $text.Contains('if (WinOneEventActive) return;')) {
        if(-not $text.Contains('public class InputManager') -or -not $text.Contains('public void Update(Control control)')){throw 'ShareX input anchors changed'}
        $text=$text.Replace('public class InputManager','public partial class InputManager')
        $text=[regex]::Replace($text,'public void Update\(Control control\)\s*\{',"public void Update(Control control)`n        {`n            if (WinOneEventActive) return;")
        [IO.File]::WriteAllText($inputPath,$text,[Text.UTF8Encoding]::new($false))
    }
    $statePath=Join-Path $Directory 'ShareX.ScreenCaptureLib/Helpers/MouseState.cs'
    $text=[IO.File]::ReadAllText($statePath)
    if(-not $text.Contains('public partial struct MouseState')) {
        if(-not $text.Contains('public struct MouseState')){throw 'ShareX mouse state anchor changed'}
        [IO.File]::WriteAllText($statePath,$text.Replace('public struct MouseState','public partial struct MouseState'),[Text.UTF8Encoding]::new($false))
    }
}

function Update-ShareXNativeSelectionOutline([string]$Path) {
    $text=[IO.File]::ReadAllText($Path)
    foreach($draw in @('g.DrawRectangleProper(borderPen, rect);','g.DrawEllipse(borderPen, rect);')) {
        $updated=$draw+"`n            if (WinOnePresentation) return;"
        if($text.Contains($updated)){continue}
        if(-not $text.Contains($draw)){throw 'ShareX selection outline anchor changed'}
        $text=$text.Replace($draw,$updated)
    }
    [IO.File]::WriteAllText($Path,$text,[Text.UTF8Encoding]::new($false))
}
