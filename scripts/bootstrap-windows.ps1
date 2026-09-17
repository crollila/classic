# Portable, repository-local toolchain. Does not modify the system PATH.
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. "$PSScriptRoot/env.ps1"
Push-Location $repoRoot
try {
    New-Item -ItemType Directory -Force .tools | Out-Null
    if (-not (Test-Path .tools/go/bin/go.exe)) {
        Invoke-WebRequest 'https://go.dev/dl/go1.23.4.windows-amd64.zip' -OutFile .tools/go.zip
        $expected = '16c59ac9196b63afb872ce9b47f945b9821a3e1542ec125f16f6085a1c0f3c39'
        if ((Get-FileHash .tools/go.zip -Algorithm SHA256).Hash.ToLower() -ne $expected) {
            throw 'Go archive checksum mismatch'
        }
        Expand-Archive .tools/go.zip .tools -Force
    }
    & go version
    if ($LASTEXITCODE) { throw 'Go toolchain failed' }
    & go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.6
    if ($LASTEXITCODE) { throw 'protoc-gen-go installation failed' }
    & npm ci --no-audit --no-fund
    if ($LASTEXITCODE) { throw 'npm ci failed' }
    & npm run generate
    if ($LASTEXITCODE) { throw 'Code generation failed' }
} finally {
    Pop-Location
}
