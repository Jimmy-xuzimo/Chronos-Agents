#ifndef AppVersion
  #define AppVersion "0.1.0"
#endif
[Setup]
AppId={{9BDCF408-68ED-4A31-9146-4EB7E9FDFA42}
AppName=Chronos Agent
AppVersion={#AppVersion}
AppPublisher=Chronos Contributors
DefaultDirName={localappdata}\Programs\ChronosAgent
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=..\..\artifacts
OutputBaseFilename=Chronos-Agent-{#AppVersion}-windows-x64-setup
SetupIconFile=..\..\internal\desktop\icon.ico
UninstallDisplayIcon={app}\chronos-agent.exe
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
CloseApplications=yes
CloseApplicationsFilter=chronos-agent.exe
RestartApplications=no
LicenseFile=..\..\LICENSE

[Tasks]
Name: "autostart"; Description: "登录 Windows 后自动运行"; Flags: checkedonce

[Files]
Source: "..\..\artifacts\chronos-agent-windows-amd64.exe"; DestDir: "{app}"; DestName: "chronos-agent.exe"; Flags: ignoreversion restartreplace

[Icons]
Name: "{autoprograms}\Chronos Agent"; Filename: "{app}\chronos-agent.exe"; Parameters: "--open"

[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "ChronosAgent"; ValueData: """{app}\chronos-agent.exe"""; Tasks: autostart; Flags: uninsdeletevalue

[Run]
Filename: "{app}\chronos-agent.exe"; Description: "启动 Chronos Agent"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; Runtime configuration is intentionally retained; uninstall UI documents its
; location. Revoke the device in Hub before removing the credential directory.
Type: files; Name: "{app}\chronos-agent.exe.old"
