package firefox

import (
	"os"
	"os/exec"
	"time"
)

type LaunchOptions struct {
	ProfileDir string
	Env        []string
	Args       []string
	Headless   bool
	Marionette bool
}

func (c *Core) Launch(opt LaunchOptions) (*exec.Cmd, error) {
	args := append(systemArgs(opt.Headless), "-profile", opt.ProfileDir, "-no-remote")
	if opt.Headless {
		args = append(args, "--headless")
	}
	if opt.Marionette {
		args = append(args, "--marionette", "-remote-allow-system-access")
	}
	args = append(args, opt.Args...)
	cmd := exec.Command(c.Exe, args...)
	cmd.Dir = c.Dir
	cmd.Env = append(cleanEnv(os.Environ()),
		"MOZ_CRASHREPORTER_DISABLE=1",
		"MOZ_CRASHREPORTER_NO_REPORT=1",
	)
	cmd.Env = append(cmd.Env, opt.Env...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func cleanEnv(env []string) []string {
	drop := map[string]bool{
		"MOZ_NO_REMOTE": true, "MOZ_LEGACY_PROFILES": true, "XRE_PROFILE_PATH": true,
		"XRE_PROFILE_LOCAL_PATH": true, "XRE_PROFILE_NAME": true, "XRE_START_OFFLINE": true,
		"KEYCAT_MODE": true, "KEYCAT_PROFILE": true, "KEYCAT_API": true, "KEYCAT_TOKEN": true,
	}
	out := env[:0:0]
	for _, kv := range env {
		k := kv
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' && i > 0 {
				k = kv[:i]
				break
			}
		}
		if !drop[k] {
			out = append(out, kv)
		}
	}
	return out
}

func WaitProfileReleased(dir string, stop <-chan struct{}) {
	for i := 0; i < 3; i++ {
		time.Sleep(700 * time.Millisecond)
		if ProfileInUse(dir) {
			break
		}
	}
	for ProfileInUse(dir) {
		select {
		case <-stop:
			return
		case <-time.After(time.Second):
		}
	}
}
