#requires -Version 5.1
# Offline native installer contract. No release, network or user installation.
$ErrorActionPreference = 'Stop'
$repository = Split-Path $PSScriptRoot -Parent
$work = Join-Path ([IO.Path]::GetTempPath()) ('axiom-windows-test-' + [guid]::NewGuid().ToString('N'))
$utf8 = New-Object Text.UTF8Encoding($false)
function Write-TestFile([string]$Path,[string]$Content) { [IO.File]::WriteAllText($Path,$Content,$utf8) }
function Assert-NativeExit([string]$Label) { if ($LASTEXITCODE -ne 0) { throw "$Label failed: $LASTEXITCODE" } }
function New-TestBundle([string]$Version) {
    $name = "axiom-$Version-windows-amd64"
    $root = Join-Path $work $name
    New-Item -ItemType Directory -Path $root | Out-Null
    & go build -trimpath -ldflags "-X main.buildVersion=$Version -X main.buildRevision=123456789abc -X main.buildSourceState=clean -X main.buildRelease=true" -o (Join-Path $root 'axiom.exe') ./cmd/lingo
    Assert-NativeExit 'Build Windows executable'
    Copy-Item -LiteralPath (Join-Path $repository 'LICENSE') -Destination $root
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'install-release.ps1') -Destination (Join-Path $root 'install.ps1')
    Copy-Item -LiteralPath (Join-Path $repository 'internal\codexruntime\skills') -Destination $root -Recurse
    $skillManifest = "formatVersion=1`nskillSetVersion=1`nbinaryCompatibility=1`n"
    foreach ($skill in (Get-ChildItem -LiteralPath (Join-Path $root 'skills') -Directory | Sort-Object Name)) {
        $hash = (Get-FileHash -LiteralPath (Join-Path $skill.FullName 'SKILL.md')).Hash.ToLowerInvariant()
        $skillManifest += "skill.$($skill.Name)=$hash`n"
    }
    Write-TestFile (Join-Path $root 'skills-manifest.txt') $skillManifest
    Write-TestFile (Join-Path $root 'release-metadata.txt') "formatVersion=1`nproduct=Axiom`nversion=$Version`nrevision=123456789abc`nsourceState=clean`nrelease=true`nplatform=windows`ngoos=windows`narchitecture=amd64`nskillSetVersion=1`n"
    $manifest = ''
    foreach ($file in (Get-ChildItem -LiteralPath $root -Recurse -File | Sort-Object FullName)) {
        $relative = $file.FullName.Substring($root.Length+1).Replace('\','/')
        $hash = (Get-FileHash -LiteralPath $file.FullName).Hash.ToLowerInvariant()
        $manifest += "$hash  $relative`n"
    }
    Write-TestFile (Join-Path $root 'MANIFEST.sha256') $manifest
    $archive = Join-Path $work "$name.tar.gz"
    & tar.exe -czf $archive -C $work $name
    Assert-NativeExit 'Create Windows archive'
    $sums = Join-Path $work "$Version-SHA256SUMS"
    $hash = (Get-FileHash -LiteralPath $archive).Hash.ToLowerInvariant()
    Write-TestFile $sums "$hash  $name.tar.gz`n"
    return @{ Root=$root; Archive=$archive; Checksums=$sums }
}
function Install-TestBundle($Bundle) {
    & (Join-Path $Bundle.Root 'install.ps1') -Archive $Bundle.Archive -Checksums $Bundle.Checksums -BinDir $bin -ReceiptDir $receipt
}
New-Item -ItemType Directory -Path $work | Out-Null
$savedEnvironment = @{}
foreach ($name in @('LINGO_PROJECTS_ROOT','LINGO_STATE_ROOT','AXIOM_CODEX_SKILLS_ROOT')) {
    $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name,'Process')
    [Environment]::SetEnvironmentVariable($name,(Join-Path $work $name),'Process')
}
Push-Location $repository
try {
    $bin = Join-Path $work 'installation with spaces\bin'
    $receipt = Join-Path $work 'installation with spaces\receipts'
    $first = New-TestBundle '1.0.0'
    $bad = Join-Path $work 'bad-checksums'
    Write-TestFile $bad (('0'*64) + '  ' + [IO.Path]::GetFileName($first.Archive) + "`n")
    $rejected = $false
    try { & (Join-Path $first.Root 'install.ps1') -Archive $first.Archive -Checksums $bad -BinDir $bin -ReceiptDir $receipt } catch { $rejected = $true }
    if (-not $rejected -or (Test-Path -LiteralPath $bin) -or (Test-Path -LiteralPath $receipt)) { throw 'Checksum refusal must have zero installation effects.' }
    Install-TestBundle $first
    $receiptFile = Join-Path $receipt 'installation.receipt'
    $before = [IO.File]::ReadAllText($receiptFile)
    Install-TestBundle $first
    if ([IO.File]::ReadAllText($receiptFile) -cne $before) { throw 'Reinstall changed the receipt.' }
    & (Join-Path $bin 'axiom.exe') version
    Assert-NativeExit 'Installed executable version'
    $next = New-TestBundle '1.1.0'
    $held = [IO.File]::Open((Join-Path $bin 'axiom.exe'),[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::None)
    try {
        $rejected = $false
        try { Install-TestBundle $next } catch { $rejected = $true }
        if (-not $rejected -or [IO.File]::ReadAllText($receiptFile) -cne $before) { throw 'An in-use binary was not preserved.' }
    } finally { $held.Dispose() }
    Install-TestBundle $next
    $version = & (Join-Path $bin 'axiom.exe') version
    Assert-NativeExit 'Upgraded executable version'
    if (($version -join "`n") -notmatch '1\.1\.0') { throw 'Upgrade did not publish the new executable.' }
    foreach ($arguments in @(@('-Version','invalid'),@('-Version','v1.0.0','-Channel','stable'))) {
        $rejected = $false
        try { & (Join-Path $PSScriptRoot 'install.ps1') @arguments } catch { $rejected = $true }
        if (-not $rejected) { throw 'Invalid bootstrap selector accepted.' }
    }
    Write-Output 'windows_install_contract=pass'
} finally {
    Pop-Location
    foreach ($name in $savedEnvironment.Keys) { [Environment]::SetEnvironmentVariable($name,$savedEnvironment[$name],'Process') }
    # Exact UUID directory created above; never delete a caller-supplied path.
    if (Test-Path -LiteralPath $work) { Remove-Item -LiteralPath $work -Recurse -Force }
}
