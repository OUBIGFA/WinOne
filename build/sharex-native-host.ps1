function Initialize-ShareXNativeHost([string]$SourceDirectory) {
    $project = [IO.Path]::GetFullPath((Join-Path $SourceDirectory 'ShareX.ScreenCaptureLib/ShareX.ScreenCaptureLib.csproj'))
    $targets = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../modules/capture/NativeUI/ShareXHost.targets'))
    if (!(Test-Path -LiteralPath $project -PathType Leaf) -or !(Test-Path -LiteralPath $targets -PathType Leaf)) { throw 'Native capture build inputs are missing' }
    foreach ($type in @('FreehandDrawingShape', 'LineDrawingShape')) {
        $file = Join-Path $SourceDirectory ('ShareX.ScreenCaptureLib/Shapes/Drawing/' + $type + '.cs')
        $text = [IO.File]::ReadAllText($file)
        $original = 'public class ' + $type + ' :'
        $partial = 'public partial class ' + $type + ' :'
        if ($text.Contains($original)) { [IO.File]::WriteAllText($file, $text.Replace($original, $partial), [Text.UTF8Encoding]::new($false)) }
        elseif (!$text.Contains($partial)) { throw ('ShareX native drawing declaration changed: ' + $type) }
    }
    [xml]$document = [IO.File]::ReadAllText($project)
    if ($document.DocumentElement.Name -ne 'Project') { throw 'Unexpected ShareX project format' }
    $imports = @($document.SelectNodes('/Project/Import[@Label="WinOneNativeCapture"]'))
    if ($imports.Count -gt 1) { throw 'Duplicate WinOne native capture imports' }
    $baseUri = [Uri]((Split-Path -Parent $project) + [IO.Path]::DirectorySeparatorChar)
    $relation = $baseUri.MakeRelativeUri([Uri]$targets)
    $relative = if ($relation.IsAbsoluteUri) { $targets } else { [Uri]::UnescapeDataString($relation.ToString()) }
    if ($imports.Count -eq 1 -and $imports[0].GetAttribute('Project') -eq $relative) { return }
    $import = if ($imports.Count -eq 1) { $imports[0] } else { $document.CreateElement('Import') }
    $import.SetAttribute('Project', $relative)
    $import.SetAttribute('Label', 'WinOneNativeCapture')
    if ($imports.Count -eq 0) { [void]$document.DocumentElement.AppendChild($import) }
    # All project-graph visits must compile the same engine. Per-reference
    # global properties can create competing builds that share one output DLL.
    $settings = [Xml.XmlWriterSettings]::new()
    $settings.Indent = $true; $settings.NewLineChars = "`n"; $settings.Encoding = [Text.UTF8Encoding]::new($false)
    $writer = [Xml.XmlWriter]::Create($project, $settings)
    try { $document.Save($writer) } finally { $writer.Dispose() }
}
