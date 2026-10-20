package discovery

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	// cgroupMount is where the unified cgroup v2 hierarchy is mounted.
	cgroupMount = "/sys/fs/cgroup"
)

// RequiredControllers must be delegated to the user manager to expose cpu and memory metrics.
var RequiredControllers = []string{"cpu", "memory"}

// UserCgroupInfo describes the cgroup subtree of the systemd user manager (user@<uid>.service).
type UserCgroupInfo struct {
	UID       string
	BaseCpath string
}

// Unit is a running systemd user unit found in the cgroup tree.
type Unit struct {
	// Name is the unescaped unit name, e.g. "my-app.service".
	Name string
	// Path is the unit's cgroup directory.
	Path string
	UID  string
}

// DiscoverUserContext resolves the cgroup base directory of the user manager.
// It uses customBasePath if given, otherwise the path systemd derives from the uid of this process.
func DiscoverUserContext(customBasePath string) (*UserCgroupInfo, error) {
	return discoverUserContext(customBasePath, os.Getuid(), cgroupMount)
}

func discoverUserContext(customBasePath string, uid int, mount string) (*UserCgroupInfo, error) {
	uidStr := strconv.Itoa(uid)

	basePath := customBasePath
	if basePath == "" {
		basePath = defaultBase(mount, uidStr)
	}

	if _, err := os.Stat(basePath); err != nil {
		return nil, fmt.Errorf("cgroups v2 base directory does not exist at %s: %w", basePath, err)
	}

	return &UserCgroupInfo{
		UID:       uidStr,
		BaseCpath: basePath,
	}, nil
}

// defaultBase is where systemd places the user manager of the given uid.
func defaultBase(mount, uid string) string {
	return fmt.Sprintf("%s/user.slice/user-%s.slice/user@%s.service", mount, uid, uid)
}

// Controllers returns the controllers enabled for the user manager's cgroup.
func (u *UserCgroupInfo) Controllers() ([]string, error) {
	content, err := os.ReadFile(filepath.Join(u.BaseCpath, "cgroup.controllers"))
	if err != nil {
		return nil, fmt.Errorf("failed to read cgroup.controllers: %w", err)
	}
	return strings.Fields(string(content)), nil
}

// MissingControllers returns the RequiredControllers that are not delegated.
func (u *UserCgroupInfo) MissingControllers() ([]string, error) {
	available, err := u.Controllers()
	if err != nil {
		return nil, err
	}
	have := make(map[string]bool, len(available))
	for _, c := range available {
		have[c] = true
	}
	var missing []string
	for _, c := range RequiredControllers {
		if !have[c] {
			missing = append(missing, c)
		}
	}
	return missing, nil
}

// Discover lists the currently running units. It is stateless and cheap (one readdir plus a stat
// per unit), so it is meant to be called on every scrape.
func (u *UserCgroupInfo) Discover() ([]Unit, error) {
	appSlicePath := filepath.Join(u.BaseCpath, "app.slice")

	searchPath := u.BaseCpath
	if _, err := os.Stat(appSlicePath); err == nil {
		searchPath = appSlicePath
	}

	entries, err := os.ReadDir(searchPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cgroup directory %s: %w", searchPath, err)
	}

	var units []Unit
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".service") {
			continue
		}
		unitPath := filepath.Join(searchPath, entry.Name())
		// The unit may have stopped since the readdir.
		if _, err := os.Stat(unitPath); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("failed to stat unit directory %s: %w", unitPath, err)
		}
		units = append(units, Unit{
			Name: UnescapeUnitName(entry.Name()),
			Path: unitPath,
			UID:  u.UID,
		})
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Name < units[j].Name })

	return units, nil
}

var unitEscape = regexp.MustCompile(`\\x([0-9a-fA-F]{2})`)

// UnescapeUnitName reverses systemd's \xNN escaping, e.g. `my\x2dapp.service` -> "my-app.service".
func UnescapeUnitName(name string) string {
	return unitEscape.ReplaceAllStringFunc(name, func(m string) string {
		b, err := strconv.ParseUint(m[2:], 16, 8)
		if err != nil {
			return m
		}
		return string([]byte{byte(b)})
	})
}
