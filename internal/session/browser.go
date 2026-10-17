package session

import (
	"os/exec"

	"keycat/internal/firefox"
)

type Browser interface {
	Wait() error
	Kill() error
	Pid() int
}

type StartFunc func(opt firefox.LaunchOptions) (Browser, error)

func FirefoxStarter(core func() (*firefox.Core, error)) StartFunc {
	return func(opt firefox.LaunchOptions) (Browser, error) {
		c, err := core()
		if err != nil {
			return nil, err
		}
		cmd, err := c.Launch(opt)
		if err != nil {
			return nil, err
		}
		return process{cmd}, nil
	}
}

type process struct{ cmd *exec.Cmd }

func (p process) Wait() error { return p.cmd.Wait() }
func (p process) Kill() error { return p.cmd.Process.Kill() }
func (p process) Pid() int    { return p.cmd.Process.Pid }
