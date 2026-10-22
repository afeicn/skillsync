package notify

import (
	"os/exec"
	"runtime"
)

func Send(title, body string) {
	switch runtime.GOOS {
	case "darwin":
		_, _ = exec.Command("osascript", "-e",
			`display notification "`+body+`" with title "`+title+`"`).Output()
	case "windows":
		ps := `New-BurntToastNotification -Text '` + title + "," + body + `'`
		_, _ = exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
	}
}
