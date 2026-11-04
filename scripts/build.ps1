param([string]$Go='go',[string]$Inno='',[string]$Rsrc='rsrc',[string]$Python='python')
$ErrorActionPreference='Stop'
Set-Location (Join-Path $PSScriptRoot '..')
New-Item -ItemType Directory -Force artifacts | Out-Null
function Invoke-Build([string]$TargetOS,[string]$Arch,[string]$Component,[bool]$Tray=$false) {
    $env:GOOS=$TargetOS; $env:GOARCH=$Arch; $env:CGO_ENABLED='0'
    $suffix=if($TargetOS -eq 'windows'){'.exe'}else{''}
    $flags='-s -w'; if($TargetOS -eq 'windows' -and $Component -eq 'agent'){$flags+=' -H=windowsgui'}
    $arguments=@('build','-trimpath','-ldflags',$flags,'-o',"artifacts/chronos-$Component-$TargetOS-$Arch$suffix")
    if(-not $Tray){$arguments+=@('-tags','headless')}
    & $Go @arguments "./cmd/$Component"
    if($LASTEXITCODE -ne 0){throw "Build failed: $TargetOS/$Arch/$Component"}
}
$oldOS=$env:GOOS;$oldArch=$env:GOARCH;$oldCGO=$env:CGO_ENABLED
try {
    & $Rsrc -arch amd64 -ico internal/desktop/icon.ico -o cmd/agent/rsrc_windows_amd64.syso
    if($LASTEXITCODE -ne 0){throw 'Windows resource compilation failed'}
    Invoke-Build windows amd64 agent $true
    Invoke-Build windows amd64 hub
    foreach($arch in @('amd64','arm64')){Invoke-Build linux $arch agent;Invoke-Build linux $arch hub}
    $oldBuildPath=$env:PATH
    try {
      $goPath=Split-Path -Parent (Get-Command $Go).Source
      $env:PATH=$goPath+[IO.Path]::PathSeparator+$env:PATH
      & $Python scripts/licenses.py
      if($LASTEXITCODE -ne 0){throw 'License inventory failed'}
    } finally {$env:PATH=$oldBuildPath}
    if($Inno){& $Inno deploy/windows/setup.iss;if($LASTEXITCODE -ne 0){throw 'Installer build failed'}}
} finally {$env:GOOS=$oldOS;$env:GOARCH=$oldArch;$env:CGO_ENABLED=$oldCGO}
$lines=Get-ChildItem artifacts -File | Where-Object Name -ne 'SHA256SUMS.txt' | Sort-Object Name | ForEach-Object {('{0}  {1}' -f (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLower(),$_.Name)}
[IO.File]::WriteAllLines((Join-Path (Get-Location) 'artifacts/SHA256SUMS.txt'),$lines,[Text.UTF8Encoding]::new($false))
Write-Output 'Built Windows x64 and Linux amd64/arm64 binaries. See artifacts/SHA256SUMS.txt.'
