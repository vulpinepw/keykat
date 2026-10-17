//go:build windows

package firefox

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

func FindSystemFirefox() string {
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, base := range []string{`SOFTWARE\Mozilla\Mozilla Firefox`, `SOFTWARE\Mozilla\Mozilla Firefox ESR`} {
			k, err := registry.OpenKey(root, base, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			cur, _, err := k.GetStringValue("CurrentVersion")
			k.Close()
			if err != nil {
				continue
			}
			mk, err := registry.OpenKey(root, base+`\`+cur+`\Main`, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			dir, _, err := mk.GetStringValue("Install Directory")
			mk.Close()
			if err == nil {
				if _, err := DetectCore(dir); err == nil {
					return dir
				}
			}
		}
	}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		dir := filepath.Join(os.Getenv(env), "Mozilla Firefox")
		if _, err := DetectCore(dir); err == nil {
			return dir
		}
	}
	return ""
}
