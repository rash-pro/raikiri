; Raikiri Windows installer (NSIS 3). Built by packaging/windows/build-installer.sh:
;   makensis -DVERSION=4.0.0 -DSTAGE=<dir with raikiri.exe, LICENSE.txt, THIRD_PARTY_NOTICES.txt> -DOUTFILE=<setup.exe> raikiri.nsi
;
; Installs per user (no admin prompt) into %LOCALAPPDATA%\Programs\Raikiri, keeps settings in
; %APPDATA%\Raikiri so upgrades and uninstalls never touch them unless asked, and starts
; Raikiri as a tray icon (raikiri.exe --tray).

Unicode true
ManifestDPIAware true
SetCompressor /SOLID lzma

!ifndef VERSION
  !error "pass -DVERSION=x.y.z"
!endif
!ifndef STAGE
  !error "pass -DSTAGE=<staging dir>"
!endif
!ifndef OUTFILE
  !define OUTFILE "raikiri-setup.exe"
!endif

!define APP "Raikiri"
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\Raikiri"
!define RUN_ARGS '--tray --data-dir "$APPDATA\Raikiri"'

Name "${APP}"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\Programs\Raikiri"
InstallDirRegKey HKCU "Software\Raikiri" "InstallDir"
RequestExecutionLevel user
BrandingText "${APP} ${VERSION}"

VIProductVersion "${VERSION}.0"
VIAddVersionKey /LANG=0 "ProductName" "${APP}"
VIAddVersionKey /LANG=0 "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=0 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=0 "FileDescription" "${APP} installer"
VIAddVersionKey /LANG=0 "LegalCopyright" "rash-pro"

!include "MUI2.nsh"
!include "FileFunc.nsh"

!define MUI_ICON "${STAGE}\raikiri.ico"
!define MUI_UNICON "${STAGE}\raikiri.ico"
!define MUI_ABORTWARNING
!define MUI_COMPONENTSPAGE_NODESC
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_FUNCTION LaunchRaikiri
!define MUI_FINISHPAGE_RUN_TEXT "$(RunLabel)"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

; NSIS picks the language matching Windows' display language, English otherwise.
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Spanish"

LangString SecCoreName ${LANG_ENGLISH} "Raikiri"
LangString SecCoreName ${LANG_SPANISH} "Raikiri"
LangString SecDesktopName ${LANG_ENGLISH} "Desktop shortcut"
LangString SecDesktopName ${LANG_SPANISH} "Acceso directo en el escritorio"
LangString SecStartupName ${LANG_ENGLISH} "Start with Windows"
LangString SecStartupName ${LANG_SPANISH} "Iniciar con Windows"
LangString RunLabel ${LANG_ENGLISH} "Start Raikiri"
LangString RunLabel ${LANG_SPANISH} "Iniciar Raikiri"
LangString DeleteData ${LANG_ENGLISH} "Also delete your Raikiri settings and data?$\r$\n$\r$\n$APPDATA\Raikiri$\r$\n$\r$\nChoose No to keep them for a future install."
LangString DeleteData ${LANG_SPANISH} "¿Borrar también la configuración y los datos de Raikiri?$\r$\n$\r$\n$APPDATA\Raikiri$\r$\n$\r$\nElige No para conservarlos para una instalación futura."

; A running Raikiri locks raikiri.exe; stop it before replacing or removing files.
!macro StopRaikiri
  nsExec::Exec 'taskkill /F /IM raikiri.exe'
  Pop $0
  Sleep 800
!macroend

Function LaunchRaikiri
  Exec '"$INSTDIR\raikiri.exe" ${RUN_ARGS}'
FunctionEnd

Section "$(SecCoreName)" SecCore
  SectionIn RO
  !insertmacro StopRaikiri

  SetOutPath "$INSTDIR"
  File "${STAGE}\raikiri.exe"
  File "${STAGE}\raikiri.ico"
  File "${STAGE}\LICENSE.txt"
  File "${STAGE}\THIRD_PARTY_NOTICES.txt"
  CreateDirectory "$APPDATA\Raikiri"

  CreateShortCut "$SMPROGRAMS\Raikiri.lnk" "$INSTDIR\raikiri.exe" '${RUN_ARGS}' "$INSTDIR\raikiri.ico"
  ; Optional shortcuts are recreated below only if still selected (matters on upgrades).
  Delete "$DESKTOP\Raikiri.lnk"
  Delete "$SMSTARTUP\Raikiri.lnk"

  WriteUninstaller "$INSTDIR\uninstall.exe"
  WriteRegStr HKCU "Software\Raikiri" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "${APP}"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "rash-pro"
  WriteRegStr HKCU "${UNINST_KEY}" "URLInfoAbout" "https://github.com/rash-pro/raikiri"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\raikiri.ico"
  WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "${UNINST_KEY}" "EstimatedSize" "$0"
SectionEnd

Section "$(SecDesktopName)" SecDesktop
  CreateShortCut "$DESKTOP\Raikiri.lnk" "$INSTDIR\raikiri.exe" '${RUN_ARGS}' "$INSTDIR\raikiri.ico"
SectionEnd

Section "$(SecStartupName)" SecStartup
  CreateShortCut "$SMSTARTUP\Raikiri.lnk" "$INSTDIR\raikiri.exe" '${RUN_ARGS}' "$INSTDIR\raikiri.ico"
SectionEnd

Section "Uninstall"
  !insertmacro StopRaikiri

  Delete "$SMPROGRAMS\Raikiri.lnk"
  Delete "$DESKTOP\Raikiri.lnk"
  Delete "$SMSTARTUP\Raikiri.lnk"
  Delete "$INSTDIR\raikiri.exe"
  Delete "$INSTDIR\raikiri.ico"
  Delete "$INSTDIR\LICENSE.txt"
  Delete "$INSTDIR\THIRD_PARTY_NOTICES.txt"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"

  DeleteRegKey HKCU "${UNINST_KEY}"
  DeleteRegKey HKCU "Software\Raikiri"

  IfFileExists "$APPDATA\Raikiri\*.*" 0 done
    MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "$(DeleteData)" /SD IDNO IDNO done
    RMDir /r "$APPDATA\Raikiri"
  done:
SectionEnd
