package platform

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

const RaisesWindows = false

func FocusProcess(pid uint32) bool { return false }

func AllowAnyForeground() {}

func ShuttingDown() bool { return false }

func SingleInstance(dataDir, localDir string) (release func(), first bool, err error) {
	f, err := os.OpenFile(filepath.Join(localDir, "keycat.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return func() {}, false, nil
		}
		return nil, false, err
	}
	return func() { f.Close() }, true, nil
}

func ShowError(title, text string) { message("error", title, text) }

func ShowInfo(title, text string) { message("info", title, text) }

func AskYesNo(title, text string) bool { return message("question", title, text) }

func message(kind, title, text string) bool {
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		if path, err := exec.LookPath("zenity"); err == nil {
			return exec.Command(path, "--"+kind, "--title="+title, "--text="+text, "--no-markup", "--width=440").Run() == nil
		}
		if path, err := exec.LookPath("kdialog"); err == nil {
			flag := map[string]string{"error": "--error", "info": "--msgbox", "question": "--warningyesno"}[kind]
			return exec.Command(path, "--title", title, flag, text).Run() == nil
		}
	}
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, text)
	if kind != "question" || !isTerminal(os.Stdin) {
		return false
	}
	fmt.Fprint(os.Stderr, "Continue? [y/N] ")
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(answer)), "y")
}

func isTerminal(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS)
	return err == nil
}

func desktopEntry() string {
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		home, _ := os.UserHomeDir()
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "applications", "keycat.desktop")
}

func DefaultBrowser(exe string) (registered, isDefault bool) {
	if entry, err := os.ReadFile(desktopEntry()); err == nil {
		for _, line := range strings.Split(string(entry), "\n") {
			if command, ok := strings.CutPrefix(line, "Exec="); ok {
				registered = samePath(execProgram(command), exe) && strings.Contains(string(entry), "x-scheme-handler/https")
			}
		}
	}
	out, err := exec.Command("xdg-settings", "get", "default-web-browser").Output()
	return registered, err == nil && strings.TrimSpace(string(out)) == "keycat.desktop"
}

func execProgram(command string) string {
	command = strings.TrimSpace(command)
	if rest, ok := strings.CutPrefix(command, `"`); ok {
		program, _, _ := strings.Cut(rest, `"`)
		return program
	}
	program, _, _ := strings.Cut(command, " ")
	return program
}

func samePath(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

func MakeDefaultBrowser(exe string) error {
	if registered, _ := DefaultBrowser(exe); !registered {
		return errors.New("This copy of Keycat is not installed. Install Keycat with install.sh to make it your default browser.")
	}
	if out, err := exec.Command("xdg-settings", "set", "default-web-browser", "keycat.desktop").CombinedOutput(); err != nil {
		return fmt.Errorf("xdg-settings could not set the default browser: %v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func OpenFolder(path string, selectFile bool) error {
	if selectFile {
		path = filepath.Dir(path)
	}
	cmd := exec.Command("xdg-open", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
