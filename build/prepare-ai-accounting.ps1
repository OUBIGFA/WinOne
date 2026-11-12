param([Parameter(Mandatory)][string]$SourceDirectory,[Parameter(Mandatory)][string]$Revision)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$utf8=[Text.UTF8Encoding]::new($false)
# Key budgets and provider spending caps share an installation-owned ledger.
# The adapter does not expose the removed usage or request-archive services.
$text=(& git -C $SourceDirectory show ($Revision+':internal/usage/usage.go')) -join "`n"
if($LASTEXITCODE -ne 0){throw 'Cannot read pinned accounting source'}
foreach($hook in @('func Path() string {','func Append(r Record) {')){if(-not $text.Contains($hook)){throw 'Upstream accounting boundary changed'}}
$text=$text.Replace('func Path() string {','func Path() string { if WinOnePath!=""{return WinOnePath};')
$text=$text.Replace('func Append(r Record) {','func Append(r Record) { if WinOneSkipAppend{return};if WinOneRecord!=nil{r=WinOneRecord(r)};')
[IO.File]::WriteAllText((Join-Path $SourceDirectory 'internal/usage/usage.go'),$text+"`n",$utf8)
[IO.File]::WriteAllText((Join-Path $SourceDirectory 'internal/usage/winone_accounting.go'),"package usage`n// Supplied only by the WinOne request owner.`nvar WinOnePath string`nvar WinOneSkipAppend bool`nvar WinOneRecord func(Record)Record`n",$utf8)
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/accounting.go') -Destination (Join-Path $SourceDirectory 'cmd/winone-ai')
