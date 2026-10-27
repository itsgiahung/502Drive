; 502Drive Windows installer (Inno Setup 6).
;
; Build (from the repository root, after the verified archives are staged in
; dist/windows):
;   ISCC.exe /DMyAppVersion=1.2.3 packaging/windows/502drive.iss
;
; This installer is NOT code-signed: Windows SmartScreen warns about an
; unknown publisher. The release notes say so explicitly; nothing here claims
; a signed build.
;
; Uninstalling removes only the program directory and the shortcuts created
; here. User data (config.json, API token, state-v2.db, secrets) lives in
; %APPDATA%\502drive* and %LOCALAPPDATA%\502drive* and is never touched.

#ifndef MyAppVersion
  #define MyAppVersion "0.0.0-dev"
#endif
#define MyAppName "502Drive"
#define MyAppExeName "502drive.exe"
#define MyAppTrayName "502drive-tray.exe"

[Setup]
AppId={{8B1F1F4E-6C4B-4A0F-9C39-3B2F0A6D8E11}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppVerName={#MyAppName} {#MyAppVersion}
AppPublisher=502Drive contributors
AppPublisherURL=https://github.com/itsgiahung/502Drive
AppSupportURL=https://github.com/itsgiahung/502Drive/issues
AppUpdatesURL=https://github.com/itsgiahung/502Drive/releases
DefaultDirName={autopf}\502Drive
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=..\..\artifacts\windows
OutputBaseFilename=502drive-setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
ChangesEnvironment=yes
UninstallDisplayIcon={app}\502drive.ico
SetupIconFile=..\assets\logo.ico
LicenseFile=..\..\LICENSE

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "addtopath"; Description: "Add 502Drive to PATH (so you can type 502drive or the short 502d in a terminal)"; GroupDescription: "Command line:"
Name: "autostart"; Description: "Start 502Drive in the notification area when I sign in"; GroupDescription: "Startup:"; Flags: unchecked

[Files]
Source: "..\..\dist\windows\{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\dist\windows\{#MyAppTrayName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\assets\logo.ico"; DestDir: "{app}"; DestName: "502drive.ico"; Flags: ignoreversion
Source: "502d.cmd"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\NOTICE.md"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\{#MyAppName} tray"; Filename: "{app}\{#MyAppTrayName}"; IconFilename: "{app}\502drive.ico"
Name: "{group}\{#MyAppName} menu"; Filename: "{app}\{#MyAppExeName}"; Parameters: "menu"; WorkingDir: "{app}"; IconFilename: "{app}\502drive.ico"
Name: "{group}\Uninstall {#MyAppName}"; Filename: "{uninstallexe}"

[Registry]
Root: HKCU; Subkey: "Environment"; ValueType: expandsz; ValueName: "Path"; ValueData: "{olddata};{app}"; Check: NeedsAddPath('{app}'); Tasks: addtopath
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "502Drive"; ValueData: """{app}\{#MyAppExeName}"" up --quiet"; Flags: uninsdeletevalue; Tasks: autostart

[Run]
Filename: "{app}\{#MyAppTrayName}"; Description: "Start 502Drive now"; Flags: nowait postinstall skipifsilent

[Code]
// True when dir is not yet in the user's PATH, so reinstalling never duplicates it.
function NeedsAddPath(Dir: string): Boolean;
var
  Paths: string;
begin
  if not RegQueryStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', Paths) then
    Paths := '';
  Result := Pos(';' + Lowercase(ExpandConstant(Dir)) + ';', ';' + Lowercase(Paths) + ';') = 0;
end;
