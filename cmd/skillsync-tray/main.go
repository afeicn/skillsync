package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"fyne.io/systray"

	"github.com/afeicn/skillsync/internal/config"
	"github.com/afeicn/skillsync/internal/recon"
)

//go:embed icon.png
var iconPNG []byte

var (
	statusItem *systray.MenuItem
	lastState  = "idle"
)

func setState(s string) {
	lastState = s
	if statusItem != nil {
		statusItem.SetTitle("skillsync: " + s)
	}
	switch s {
	case "ok":
		systray.SetTooltip("skillsync — in sync")
	case "pending":
		systray.SetTooltip("skillsync — conflict needs reconciliation")
	default:
		systray.SetTooltip("skillsync — " + s)
	}
}

func runSync() {
	cfg, err := config.Load("")
	if err != nil {
		setState("no config")
		return
	}
	code := recon.Auto(cfg)
	switch code {
	case 0:
		setState("ok")
	case 2:
		setState("pending")
	default:
		setState("error")
	}
}

func latestPending() string {
	dir := config.StateDir()
	files, _ := filepath.Glob(filepath.Join(dir, "pending-*.md"))
	if len(files) == 0 {
		return ""
	}
	return files[len(files)-1]
}

func openReport() {
	p := latestPending()
	if p == "" {
		return
	}
	switch runtime.GOOS {
	case "darwin":
		_, _ = exec.Command("open", "-t", p).Output()
	case "windows":
		_, _ = exec.Command("notepad", p).Output()
	}
}

func main() {
	onExit := func() {}
	systray.Run(func() {
		systray.SetTemplateIcon(iconPNG, iconPNG)
		systray.SetTooltip("skillsync")
		statusItem = systray.AddMenuItem("skillsync: idle", "")
		statusItem.Disable()
		syncNow := systray.AddMenuItem("Sync now", "")
		openRep := systray.AddMenuItem("Open pending report", "")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "")

		go func() {
			cfg, err := config.Load("")
			interval := 60 * time.Minute
			if err == nil && cfg.Interval > 0 {
				interval = time.Duration(cfg.Interval) * time.Minute
			}
			runSync()
			for t := range time.Tick(interval) {
				_ = t
				runSync()
			}
		}()

		go func() {
			for {
				select {
				case <-syncNow.ClickedCh:
					runSync()
				case <-openRep.ClickedCh:
					openReport()
				case <-quit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, onExit)
	fmt.Fprintln(os.Stderr, "tray exited")
}
