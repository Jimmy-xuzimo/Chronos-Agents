param([string]$Setup='artifacts/Chronos-Agent-1.0.0-windows-x64-setup.exe')
$ErrorActionPreference='Stop'
Set-Location (Join-Path $PSScriptRoot '..')
$base=[IO.Path]::GetFullPath((Join-Path (Get-Location) '.runtime'))
New-Item -ItemType Directory -Force $base | Out-Null
$target=Join-Path $base 'install-test'
if(-not $target.StartsWith($base+[IO.Path]::DirectorySeparatorChar)){throw 'Unsafe test installation path'}
$registry='HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\{9BDCF408-68ED-4A31-9146-4EB7E9FDFA42}_is1'
if(Test-Path $registry){throw 'Existing Chronos Agent installation found; isolated test refused'}
$run='HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
if(Get-ItemProperty -Path $run -Name ChronosAgent -ErrorAction SilentlyContinue){throw 'Existing startup setting found; test refused'}
$setupPath=(Resolve-Path $Setup).Path
$arguments=@('/VERYSILENT','/SUPPRESSMSGBOXES','/SP-','/NORESTART','/MERGETASKS=!autostart',('/DIR="'+$target+'"'),('/LOG="'+(Join-Path $base 'install-test.log')+'"'))
$qa=$null;$result=[ordered]@{}
try {
    $install=Start-Process $setupPath -ArgumentList $arguments -WindowStyle Hidden -PassThru -Wait
    $result.InstallExit=$install.ExitCode
    if($install.ExitCode -ne 0){throw 'Installation failed'}
    $exe=Join-Path $target 'chronos-agent.exe'
    $result.PayloadMatches=(Get-FileHash $exe).Hash -eq (Get-FileHash artifacts/chronos-agent-windows-amd64.exe).Hash
    if(-not $result.PayloadMatches){throw 'Installed binary differs'}
    $data=Join-Path $base 'install-test-data'
    $qa=Start-Process $exe -ArgumentList '--headless','--data',('"'+$data+'"'),'--listen','127.0.0.1:17961','--origin','http://127.0.0.1:17961' -WindowStyle Hidden -PassThru
    Start-Sleep -Seconds 2
    $result.Health=(Invoke-RestMethod http://127.0.0.1:17961/healthz).ok
    $stateHash=(Get-FileHash (Join-Path $data 'state.json')).Hash
    $upgrade=Start-Process $setupPath -ArgumentList $arguments -WindowStyle Hidden -PassThru -Wait
    $result.UpgradeExit=$upgrade.ExitCode
    $qa.Refresh();$result.UpgradeClosedOwnProcess=$qa.HasExited
    $result.ConfigurationPreserved=$stateHash -eq (Get-FileHash (Join-Path $data 'state.json')).Hash
    if($upgrade.ExitCode -ne 0 -or -not $qa.HasExited){throw 'Upgrade could not replace running agent'}
} finally {
    if($qa -and -not $qa.HasExited){
        $owned=Get-Process -Id $qa.Id -ErrorAction SilentlyContinue
        if($owned -and $owned.Path -eq (Join-Path $target 'chronos-agent.exe')){Stop-Process -Id $qa.Id}
    }
    $uninstaller=Join-Path $target 'unins000.exe'
    if(Test-Path $uninstaller){
        $remove=Start-Process $uninstaller -ArgumentList '/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART' -WindowStyle Hidden -PassThru -Wait
        $result.UninstallExit=$remove.ExitCode
    }
    $result.RegistryRemoved=-not (Test-Path $registry)
    $result | ConvertTo-Json | Set-Content (Join-Path $base 'windows-install-result.json') -Encoding utf8
}
$result | ConvertTo-Json
if($result.UninstallExit -ne 0 -or -not $result.RegistryRemoved){throw 'Uninstall verification failed'}
