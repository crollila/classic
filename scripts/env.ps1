$repoRoot = Split-Path $PSScriptRoot -Parent
$env:GOPATH = Join-Path $repoRoot '.tools/gopath'
$env:GOCACHE = Join-Path $repoRoot '.tools/go-cache'
$env:GOTOOLCHAIN = 'local'
$env:npm_config_cache = Join-Path $repoRoot '.tools/npm-cache'
$env:PATH = (Join-Path $repoRoot '.tools/go/bin') + [IO.Path]::PathSeparator + (Join-Path $env:GOPATH 'bin') + [IO.Path]::PathSeparator + $env:PATH
