package sd

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

type UserCgroupInfo struct {
	UID       string
	BaseCpath string
}

func DiscoverUserContext() (*UserCgroupInfo, error) {
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current OS user: %w", err)
	}

	basePath := fmt.Sprintf("/sys/fs/cgroup/user.slice/user-%s.slice/user@%s.service", currentUser.Uid, currentUser.Uid)

	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("cgroups v2 user directory does not exist at %s: %w", basePath, err)
	}

	return &UserCgroupInfo{
		UID:       currentUser.Uid,
		BaseCpath: basePath,
	}, nil
}

func (u *UserCgroupInfo) DiscoverActiveUnits() ([]string, error) {
	var units []string

	appSlicePath := filepath.Join(u.BaseCpath, "app.slice")
	
	searchPath := u.BaseCpath
	if _, err := os.Stat(appSlicePath); err == nil {
		searchPath = appSlicePath
	}

	entries, err := os.ReadDir(searchPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cgroup directory %s: %w", searchPath, err)
	}

	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), ".service") {
			units = append(units, entry.Name())
		}
	}

	return units, nil
}
