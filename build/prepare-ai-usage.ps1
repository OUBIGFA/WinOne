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
# Alias only the keys used by the existing ledger aggregators and filters.
# Rows and CSV retain the model that was actually requested and priced.
$ledger=(& git -C $SourceDirectory show ($Revision+':internal/usage/ledger.go')) -join [Environment]::NewLine
if($LASTEXITCODE -ne 0){throw 'Cannot read pinned Magpie ledger source'}
$aliasHooks=@{
 'f.Model != "" && r.Model != f.Model'='f.Model != "" && WinOneModelGroup(r.Model) != WinOneModelGroup(f.Model)'
 'return ModelAtKey(r.Provider, r.Model)'='return ModelAtKey(r.Provider, WinOneModelGroup(r.Model))'
 'return r.Model'='return WinOneModelGroup(r.Model)'
}
foreach($hook in $aliasHooks.Keys){
 if(($ledger.Split([string[]]@($hook),[StringSplitOptions]::None).Length-1) -ne 1){throw ('Magpie ledger alias boundary changed: '+$hook)}
 $ledger=$ledger.Replace($hook,$aliasHooks[$hook])
}
[IO.File]::WriteAllText((Join-Path $SourceDirectory 'internal/usage/ledger.go'),$ledger+[Environment]::NewLine,$encoding)
$archive=(& git -C $SourceDirectory show ($Revision+':internal/gateway/archive_local.go')) -join [Environment]::NewLine
if($LASTEXITCODE -ne 0){throw 'Cannot read pinned local archive source'}
$archiveHook='dir := filepath.Join(settings.Dir(), "request-archive")'
if(-not $archive.Contains($archiveHook)){throw 'Magpie archive path boundary changed'}
$archive=$archive.Replace($archiveHook,$archiveHook+'; if WinOneArchiveDirectory != "" { dir = WinOneArchiveDirectory }')
[IO.File]::WriteAllText((Join-Path $SourceDirectory 'internal/gateway/archive_local.go'),$archive+[Environment]::NewLine,$encoding)
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/prompt_embed.go.txt') -Destination (Join-Path $SourceDirectory 'internal/gateway/winone_prompt.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/usage.go'),(Join-Path $root 'modules/ai-magpie/usage_test.go') -Destination (Join-Path $SourceDirectory 'cmd/winone-ai')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/context.go') -Destination (Join-Path $SourceDirectory 'cmd/winone-ai')
