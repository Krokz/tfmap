package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/crgimenes/glaze"
)

// runNativeWindow opens url in a native webview window (WKWebView, WebView2,
// or WebKitGTK) owned by the tfmap process itself, so the OS taskbar/dock
// shows tfmap rather than a browser. Blocks until the window is closed.
// Must be called from the main goroutine (see runtime.LockOSThread in main).
func runNativeWindow(url, title string) error {
	w, err := glaze.New(false)
	if err != nil {
		return err
	}
	defer w.Destroy()
	w.SetTitle(title)
	w.SetSize(1280, 800, glaze.HintNone)
	w.Navigate(url)
	w.Run()
	return nil
}

// openWindow opens url in a dedicated app-mode window (no tabs, no URL bar)
// using an installed Chromium-based browser. Falls back to a regular tab in
// the default browser when none is found.
func openWindow(url string) {
	if browser := findChromiumBrowser(); browser != "" {
		if err := exec.Command(browser, "--app="+url).Start(); err == nil {
			return
		}
	}
	openBrowser(url)
}

// findChromiumBrowser returns the path to an installed Chromium-based
// browser that supports the --app flag, or "" if none is found.
func findChromiumBrowser() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		apps := []string{
			"Google Chrome.app/Contents/MacOS/Google Chrome",
			"Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"Brave Browser.app/Contents/MacOS/Brave Browser",
			"Chromium.app/Contents/MacOS/Chromium",
		}
		roots := []string{"/Applications"}
		if home != "" {
			roots = append(roots, filepath.Join(home, "Applications"))
		}
		for _, app := range apps {
			for _, root := range roots {
				if p := filepath.Join(root, app); fileExists(p) {
					return p
				}
			}
		}
	case "linux":
		names := []string{
			"google-chrome", "google-chrome-stable", "chromium",
			"chromium-browser", "microsoft-edge", "brave-browser",
		}
		for _, name := range names {
			if p, err := exec.LookPath(name); err == nil {
				return p
			}
		}
	case "windows":
		rels := []string{
			`Google\Chrome\Application\chrome.exe`,
			`Microsoft\Edge\Application\msedge.exe`,
			`BraveSoftware\Brave-Browser\Application\brave.exe`,
			`Chromium\Application\chrome.exe`,
		}
		roots := []string{
			os.Getenv("ProgramFiles"),
			os.Getenv("ProgramFiles(x86)"),
			os.Getenv("LocalAppData"),
		}
		for _, rel := range rels {
			for _, root := range roots {
				if root == "" {
					continue
				}
				if p := filepath.Join(root, rel); fileExists(p) {
					return p
				}
			}
		}
	}
	return ""
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
