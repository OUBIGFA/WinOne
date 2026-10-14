param([Parameter(Mandatory)][string]$SourceDirectory,[Parameter(Mandatory)][string]$Revision)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$encoding=[Text.UTF8Encoding]::new($false)
# Always patch the pinned original, not a previous build's generated source.
$lines=& git -C $SourceDirectory show ($Revision+':internal/usage/usage.go')
if($LASTEXITCODE -ne 0){throw 'Cannot read pinned Magpie usage source'}
$text=$lines -join [Environment]::NewLine
$pathHook='func Path() string {'
$appendHook='func Append(r Record) {'
if(-not $text.Contains($pathHook) -or -not $text.Contains($appendHook)){throw 'Magpie usage embedding points changed'}
$text=$text.Replace($pathHook,$pathHook+' if WinOnePath != "" { return WinOnePath }; ')
$text=$text.Replace($appendHook,$appendHook+' if WinOneRecord != nil { r = WinOneRecord(r) }; ')
$text=$text.Replace('type Record struct {','type Record struct { WinOnePrompt json.RawMessage `json:"winone_prompt,omitempty"`; ')
[IO.File]::WriteAllText((Join-Path $SourceDirectory 'internal/usage/usage.go'),$text+[Environment]::NewLine,$encoding)
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/usage_embed.go.txt') -Destination (Join-Path $SourceDirectory 'internal/usage/winone_embed.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/prompt_embed.go.txt') -Destination (Join-Path $SourceDirectory 'internal/gateway/winone_prompt.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/usage.go'),(Join-Path $root 'modules/ai-magpie/usage_test.go') -Destination (Join-Path $SourceDirectory 'cmd/winone-ai')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/context.go') -Destination (Join-Path $SourceDirectory 'cmd/winone-ai')
