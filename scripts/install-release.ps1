#requires -Version 5.1
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$Archive,
    [Parameter(Mandatory=$true)][string]$Checksums,
    [string]$BinDir = (Join-Path $env:LOCALAPPDATA 'Axiom\bin'),
    [string]$ReceiptDir = (Join-Path $env:LOCALAPPDATA 'Axiom\install')
)
$ErrorActionPreference = 'Stop'
& (Join-Path $PSScriptRoot 'axiom.exe') install-release --archive $Archive --checksums $Checksums --bin-dir $BinDir --receipt-dir $ReceiptDir
if ($LASTEXITCODE -ne 0) { throw "Verified release installation failed (exit $LASTEXITCODE)." }
