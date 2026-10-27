// Package installer makes Windows setup programs with NSIS.
package installer

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	_ "image/jpeg"

	"golang.org/x/image/draw"
)

// Options describe the app to install.
type Options struct {
	// Name is the display name; ID the app ID, which keys the
	// uninstall entry.
	Name, ID string
	// Version is major.minor.patch, Publisher and Website what Windows'
	// list of installed apps shows.
	Version, Publisher, Website string
	// Exe is the built program; Icon a PNG for the setup program's icon
	// (optional); License a text file to accept (optional).
	Exe, Icon, License string
	// PerMachine installs for every user, asking for administrator
	// rights; otherwise for the current user without asking.
	PerMachine bool
	// DesktopShortcut adds a desktop shortcut.
	DesktopShortcut bool
	// Output is the setup program to write.
	Output string
	// Verbose prints the makensis command and its output.
	Verbose bool
}

// Build writes a setup program with makensis: a wizard (welcome,
// license, folder, progress, finish with "Run"), Start menu and optional
// desktop shortcuts, an entry in Settings > Apps with an uninstaller,
// and per-user or per-machine install.
func Build(o Options) error {
	makensis, err := findMakensis()
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "giode-nsis-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	d := scriptData{
		Options: o,
		ExeName: filepath.Base(o.Exe),
		Exe:     abs(o.Exe),
		Output:  abs(o.Output),
	}
	if o.Icon != "" {
		d.IconFile = filepath.Join(work, "app.ico")
		if err := writeICO(o.Icon, d.IconFile); err != nil {
			return fmt.Errorf("installer icon: %w", err)
		}
	}
	if o.License != "" {
		d.License = abs(o.License)
	}
	if st, err := os.Stat(o.Exe); err == nil {
		d.SizeKB = (st.Size() + 1023) / 1024
	}
	var script bytes.Buffer
	if err := nsisTemplate.Execute(&script, d); err != nil {
		return err
	}
	nsi := filepath.Join(work, "setup.nsi")
	if err := os.WriteFile(nsi, script.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(d.Output), 0o755); err != nil {
		return err
	}
	cmd := exec.Command(makensis, "-V2", "-INPUTCHARSET", "UTF8", nsi)
	if o.Verbose {
		fmt.Fprintln(os.Stderr, "+", strings.Join(cmd.Args, " "))
	}
	out, err := cmd.CombinedOutput()
	if o.Verbose || err != nil {
		os.Stderr.Write(out)
	}
	if err != nil {
		return fmt.Errorf("makensis: %w", err)
	}
	return nil
}

// Script returns the NSIS script Build would compile, for tests and
// for customizing.
func Script(o Options) (string, error) {
	var b bytes.Buffer
	err := nsisTemplate.Execute(&b, scriptData{Options: o, ExeName: filepath.Base(o.Exe), Exe: o.Exe, Output: o.Output, IconFile: o.Icon, License: o.License})
	return b.String(), err
}

func abs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

// findMakensis finds NSIS's compiler on the PATH or in its Windows
// install folder.
func findMakensis() (string, error) {
	if p, err := exec.LookPath("makensis"); err == nil {
		return p, nil
	}
	if runtime.GOOS == "windows" {
		for _, root := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")} {
			p := filepath.Join(root, "NSIS", "makensis.exe")
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	return "", errors.New("building a Windows installer needs NSIS: winget install NSIS.NSIS, brew install makensis, or apt install nsis")
}

// writeICO converts a PNG to a Windows icon with the usual sizes, each
// stored as PNG (supported since Windows Vista).
func writeICO(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return err
	}
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	var images [][]byte
	for _, s := range sizes {
		dstImg := image.NewNRGBA(image.Rect(0, 0, s, s))
		draw.CatmullRom.Scale(dstImg, dstImg.Bounds(), img, img.Bounds(), draw.Src, nil)
		var b bytes.Buffer
		if err := png.Encode(&b, dstImg); err != nil {
			return err
		}
		images = append(images, b.Bytes())
	}
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		dim := uint8(s)
		if s >= 256 {
			dim = 0 // 0 means 256
		}
		binary.Write(&out, binary.LittleEndian, struct {
			W, H, Colors, Reserved uint8
			Planes, BitCount       uint16
			Size, Offset           uint32
		}{dim, dim, 0, 0, 1, 32, uint32(len(images[i])), uint32(offset)})
		offset += len(images[i])
	}
	for _, b := range images {
		out.Write(b)
	}
	return os.WriteFile(dst, out.Bytes(), 0o644)
}

type scriptData struct {
	Options
	ExeName, Exe, Output, IconFile, License string
	SizeKB                                  int64
}

// nsisTemplate is the setup script. NSIS strings are quoted with
// double quotes; the q function escapes them.
var nsisTemplate = template.Must(template.New("nsis").Funcs(template.FuncMap{
	"q": func(s string) string {
		s = strings.ReplaceAll(s, "$", "$$")
		s = strings.ReplaceAll(s, `"`, `$\"`)
		return `"` + s + `"`
	},
}).Parse(`Unicode true
ManifestDPIAware true
SetCompressor /SOLID lzma
!include "MUI2.nsh"
!include "x64.nsh"

!define APPNAME {{q .Name}}
!define APPID {{q .ID}}
!define EXE {{q .ExeName}}
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPID}"

Name "${APPNAME}"
OutFile {{q .Output}}
BrandingText "${APPNAME} {{.Version}}"
{{- if .PerMachine}}
RequestExecutionLevel admin
InstallDir "$PROGRAMFILES64\${APPNAME}"
!define REGROOT HKLM
{{- else}}
RequestExecutionLevel user
InstallDir "$LOCALAPPDATA\Programs\${APPNAME}"
!define REGROOT HKCU
{{- end}}
InstallDirRegKey ${REGROOT} "${UNINST_KEY}" "InstallLocation"

VIProductVersion "{{.Version}}.0"
VIAddVersionKey "ProductName" "${APPNAME}"
VIAddVersionKey "ProductVersion" "{{.Version}}"
VIAddVersionKey "FileVersion" "{{.Version}}"
VIAddVersionKey "CompanyName" {{q .Publisher}}
VIAddVersionKey "FileDescription" "${APPNAME} Setup"
VIAddVersionKey "LegalCopyright" {{q .Publisher}}

{{- if .IconFile}}
!define MUI_ICON {{q .IconFile}}
!define MUI_UNICON {{q .IconFile}}
{{- end}}
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXE}"

!insertmacro MUI_PAGE_WELCOME
{{- if .License}}
!insertmacro MUI_PAGE_LICENSE {{q .License}}
{{- end}}
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Function .onInit
{{- if .PerMachine}}
	${If} ${RunningX64}
		SetRegView 64
	${EndIf}
	SetShellVarContext all
{{- else}}
	SetShellVarContext current
{{- end}}
FunctionEnd

Function un.onInit
{{- if .PerMachine}}
	${If} ${RunningX64}
		SetRegView 64
	${EndIf}
	SetShellVarContext all
{{- else}}
	SetShellVarContext current
{{- end}}
FunctionEnd

Section "Install"
	SetOutPath "$INSTDIR"
	File {{q .Exe}}
	WriteUninstaller "$INSTDIR\Uninstall.exe"
	CreateShortCut "$SMPROGRAMS\${APPNAME}.lnk" "$INSTDIR\${EXE}"
{{- if .DesktopShortcut}}
	CreateShortCut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\${EXE}"
{{- end}}
	WriteRegStr ${REGROOT} "${UNINST_KEY}" "DisplayName" "${APPNAME}"
	WriteRegStr ${REGROOT} "${UNINST_KEY}" "DisplayVersion" "{{.Version}}"
	WriteRegStr ${REGROOT} "${UNINST_KEY}" "Publisher" {{q .Publisher}}
{{- if .Website}}
	WriteRegStr ${REGROOT} "${UNINST_KEY}" "URLInfoAbout" {{q .Website}}
{{- end}}
	WriteRegStr ${REGROOT} "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${EXE}"
	WriteRegStr ${REGROOT} "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
	WriteRegStr ${REGROOT} "${UNINST_KEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
	WriteRegStr ${REGROOT} "${UNINST_KEY}" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
	WriteRegDWORD ${REGROOT} "${UNINST_KEY}" "NoModify" 1
	WriteRegDWORD ${REGROOT} "${UNINST_KEY}" "NoRepair" 1
	WriteRegDWORD ${REGROOT} "${UNINST_KEY}" "EstimatedSize" {{.SizeKB}}
SectionEnd

Section "Uninstall"
	Delete "$INSTDIR\${EXE}"
	Delete "$INSTDIR\Uninstall.exe"
	RMDir "$INSTDIR"
	Delete "$SMPROGRAMS\${APPNAME}.lnk"
	Delete "$DESKTOP\${APPNAME}.lnk"
	DeleteRegKey ${REGROOT} "${UNINST_KEY}"
SectionEnd
`))
