package clipboard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// GetImage attempts to read an image from the system clipboard.
// Returns the path to a temp file containing the image, or an error.
// The caller is responsible for cleaning up the temp file.
func GetImage() (path string, mimeType string, err error) {
	switch runtime.GOOS {
	case "darwin":
		return getImageMacOS()
	case "windows":
		return getImageWindows()
	case "linux":
		return getImageLinux()
	default:
		return "", "", fmt.Errorf("clipboard image not supported on %s", runtime.GOOS)
	}
}

func getImageMacOS() (string, string, error) {
	tmpFile := filepath.Join(os.TempDir(), "openclaw-tui-clipboard.png")

	// Try pngpaste first (brew install pngpaste)
	if path, err := exec.LookPath("pngpaste"); err == nil {
		cmd := exec.Command(path, tmpFile)
		if err := cmd.Run(); err == nil {
			return tmpFile, "image/png", nil
		}
	}

	// Fallback: osascript
	script := `
use framework "AppKit"
set pb to current application's NSPasteboard's generalPasteboard()
set imgData to pb's dataForType:(current application's NSPasteboardTypePNG)
if imgData is missing value then
    error "no image on clipboard"
end if
set filePath to POSIX file "` + tmpFile + `"
imgData's writeToFile:"` + tmpFile + `" atomically:true
`
	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("no image on clipboard")
	}

	// Verify file was created
	if _, err := os.Stat(tmpFile); err != nil {
		return "", "", fmt.Errorf("no image on clipboard")
	}

	return tmpFile, "image/png", nil
}

func getImageWindows() (string, string, error) {
	tmpFile := filepath.Join(os.TempDir(), "openclaw-tui-clipboard.png")

	// PowerShell: save clipboard image
	ps := fmt.Sprintf(`
$img = Get-Clipboard -Format Image
if ($img -eq $null) { exit 1 }
$img.Save('%s', [System.Drawing.Imaging.ImageFormat]::Png)
`, tmpFile)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("no image on clipboard")
	}

	if _, err := os.Stat(tmpFile); err != nil {
		return "", "", fmt.Errorf("no image on clipboard")
	}

	return tmpFile, "image/png", nil
}

func getImageLinux() (string, string, error) {
	tmpFile := filepath.Join(os.TempDir(), "openclaw-tui-clipboard.png")

	// Try wl-paste (Wayland) first
	if path, err := exec.LookPath("wl-paste"); err == nil {
		cmd := exec.Command(path, "--type", "image/png")
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			if err := os.WriteFile(tmpFile, out, 0600); err == nil {
				return tmpFile, "image/png", nil
			}
		}
	}

	// Try xclip (X11)
	if path, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command(path, "-selection", "clipboard", "-t", "image/png", "-o")
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			if err := os.WriteFile(tmpFile, out, 0600); err == nil {
				return tmpFile, "image/png", nil
			}
		}
	}

	return "", "", fmt.Errorf("no image on clipboard (install xclip or wl-paste)")
}
