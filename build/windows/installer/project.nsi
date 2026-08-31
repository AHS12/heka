Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them with the values from ProjectInfo.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
##
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the ProjectInfo file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "MyProject" # Default "{{.Name}}"
## !define INFO_COMPANYNAME    "MyCompany" # Default "{{.Info.CompanyName}}"
## !define INFO_PRODUCTNAME    "MyProduct" # Default "{{.Info.ProductName}}"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "{{.Info.ProductVersion}}"
## !define INFO_COPYRIGHT      "Copyright" # Default "{{.Info.Copyright}}"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
####
## Include the wails tools + nsDialogs for PATH checkbox
####
!define PRODUCT_EXECUTABLE "heka-gui.exe"
!define HEKA_CONSOLE_EXECUTABLE "heka.exe"
!include "wails_tools.nsh"
!include "nsDialogs.nsh"

!ifndef ARG_HEKA_AMD64_BINARY
    !error "Heka: ARG_HEKA_AMD64_BINARY is required"
!endif

!macro heka.files
    !ifdef SUPPORTS_AMD64
        ${If} ${IsNativeAMD64}
            File "/oname=${HEKA_CONSOLE_EXECUTABLE}" "${ARG_HEKA_AMD64_BINARY}"
        ${EndIf}
    !endif
!macroend

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}" # Offer "Run ${INFO_PRODUCTNAME}" on the finish page (checked by default)

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.

# --- PATH checkbox page ---
Var AddToPath
Var Checkbox

Page custom fnc_AddToPath_Show fnc_AddToPath_Leave

Function fnc_AddToPath_Show
    !insertmacro MUI_HEADER_TEXT "Add to PATH" "Optionally add ${INFO_PRODUCTNAME} to your PATH environment variable."
    nsDialogs::Create 1018
    Pop $0

    ${NSD_CreateCheckbox} 10u 30u 280u 12u "Add installation directory to PATH (recommended)"
    Pop $Checkbox

    # Checked by default
    ${NSD_Check} $Checkbox

    nsDialogs::Show
FunctionEnd

Function fnc_AddToPath_Leave
    ${NSD_GetState} $Checkbox $AddToPath
FunctionEnd

!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uinstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
!ifdef WAILS_INSTALL_SCOPE
  !if "${WAILS_INSTALL_SCOPE}" == "user"
    InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
  !else
    InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
  !endif
!else
  InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif # Default installing folder ($PROGRAMFILES is Program Files folder).
ShowInstDetails show # This will always show the installation details.

Function .onInit
   !insertmacro wails.checkArchitecture

   # An upgrade must not race a running app: close the GUI and daemon, and drop
   # the watchdog scheduled task so it cannot re-launch a stale binary mid-install.
   # The task/startup entries are re-created after install (see Section below).
   ${IfNot} ${Silent}
       MessageBox MB_YESNO|MB_ICONQUESTION "Heka is running. Setup will close it to continue. Continue?" IDYES +2
       Abort
   ${EndIf}

   nsExec::ExecToLog 'taskkill /IM heka-gui.exe /F /T'
   nsExec::ExecToLog 'taskkill /IM heka.exe /F /T'
   nsExec::ExecToLog 'taskkill /IM Heka.exe /F /T'
   nsExec::ExecToLog 'schtasks /Delete /TN "Heka Watchdog" /F'
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files
    !insertmacro heka.files

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    # --- Add to PATH if checkbox was checked ---
    ${If} $AddToPath == ${BST_CHECKED}
        ReadRegStr $0 HKCU "Environment" "Path"
        StrCpy $1 $0
        # Check if INSTDIR is already in PATH
        StrLen $2 $0
        StrLen $3 $INSTDIR
        StrCpy $4 0
        StrCpy $5 0
        ${DoWhile} $4 < $2
            StrCpy $6 $0 $3 $4
            ${If} $6 == $INSTDIR
                StrCpy $5 1
                ${ExitDo}
            ${EndIf}
            IntOp $4 $4 + 1
        ${Loop}
        ${If} $5 == 0
            # Not found — append with semicolon separator
            StrLen $2 $0
            ${If} $2 > 0
                StrCpy $0 "$0;$INSTDIR"
            ${Else}
                StrCpy $0 "$INSTDIR"
            ${EndIf}
            WriteRegStr HKCU "Environment" "Path" "$0"
        ${EndIf}
    ${EndIf}

    !insertmacro wails.writeUninstaller

    # --- Restore watchdog + startup entries with the new install path ---
    # The watchdog scheduled task was deleted above; re-create it (same name,
    # default 5-minute interval) so the daemon is guarded with the new binary.
    # The startup Run key is rewritten so it points at the new path.
    nsExec::ExecToLog '"$INSTDIR\${HEKA_CONSOLE_EXECUTABLE}" daemon watchdog install'
    nsExec::ExecToLog '"$INSTDIR\${HEKA_CONSOLE_EXECUTABLE}" daemon startup on'
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    nsExec::ExecToLog 'taskkill /IM heka-gui.exe /F /T'
    nsExec::ExecToLog 'taskkill /IM heka.exe /F /T'
    nsExec::ExecToLog 'taskkill /IM Heka.exe /F /T'

    # --- Remove OS entries BEFORE deleting files ---
    nsExec::ExecToLog 'schtasks /Delete /TN "Heka Watchdog" /F'
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Heka"

    # --- Remove from PATH FIRST (before deleting files) ---
    ReadRegStr $0 HKCU "Environment" "Path"
    ${If} $0 != ""
        StrCpy $8 ""       # rebuilt PATH
        StrCpy $9 ";$0;"   # wrap with semicolons for clean matching
        StrCpy $3 ";$INSTDIR;"

        # Walk through the wrapped PATH looking for the install entry
        StrLen $2 $9
        StrLen $4 $3
        StrCpy $5 0        # scan position

        ${DoWhile} $5 < $2
            StrCpy $6 $9 $4 $5
            ${If} $6 == $3
                # Found our entry — skip it entirely
                IntOp $5 $5 + $4
            ${Else}
                # Copy one character at a time until semicolon
                StrCpy $7 $5
                ${DoWhile} $5 < $2
                    StrCpy $6 $9 1 $5
                    ${If} $6 == ";"
                        ${ExitDo}
                    ${EndIf}
                    IntOp $5 $5 + 1
                ${Loop}
                # Extract entry (between $7 and $5)
                IntOp $6 $5 - $7
                StrCpy $6 $9 $6 $7
                ${If} $6 != ""
                    ${If} $8 != ""
                        StrCpy $8 "$8;$6"
                    ${Else}
                        StrCpy $8 "$6"
                    ${EndIf}
                ${EndIf}
                # Skip semicolon
                ${If} $5 < $2
                    IntOp $5 $5 + 1
                ${EndIf}
            ${EndIf}
        ${Loop}
        WriteRegStr HKCU "Environment" "Path" "$8"
    ${EndIf}

    # --- Remove files ---
    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller

    # Delete install dir (uninstaller.exe is now gone, so RMDir can succeed)
    RMDir /r $INSTDIR
    # Fallback: if anything remains, try non-recursive
    RMDir $INSTDIR
SectionEnd
