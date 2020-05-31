package stacker

import (
	"bufio"
	"os"
	"os/user"
	"path"
	"strconv"
	"strings"
	"syscall"

	"github.com/pkg/errors"
)

type Storage interface {
	Name() string
	Create(path string) error
	Snapshot(source string, target string) error
	Restore(source string, target string) error
	Delete(path string) error
	Detach() error
	Exists(thing string) bool
	MarkReadOnly(thing string) error
	TemporaryWritableSnapshot(source string) (string, func(), error)
}

func NewStorage(c StackerConfig) (Storage, error) {
	return &overlay{c: c}, nil

	fs := syscall.Statfs_t{}

	if err := os.MkdirAll(c.RootFSDir, 0755); err != nil {
		return nil, err
	}

	err := syscall.Statfs(c.RootFSDir, &fs)
	if err != nil {
		return nil, err
	}

	/* btrfs superblock magic number */
	isBtrfs := fs.Type == 0x9123683E

	currentUser, err := user.Current()
	if err != nil {
		return nil, err
	}

	if !isBtrfs {
		if err := os.MkdirAll(c.StackerDir, 0755); err != nil {
			return nil, err
		}

		loopback := path.Join(c.StackerDir, "btrfs.loop")
		size := 100 * 1024 * 1024 * 1024
		uid, err := strconv.Atoi(currentUser.Uid)
		if err != nil {
			return nil, err
		}

		gid, err := strconv.Atoi(currentUser.Gid)
		if err != nil {
			return nil, err
		}

		err = MakeLoopbackBtrfs(loopback, int64(size), uid, gid, c.RootFSDir)
		if err != nil {
			return nil, err
		}

	}

	return &btrfs{c: c, needsUmount: !isBtrfs}, nil
}

func IsMountpoint(path string) (bool, error) {
	return IsMountpointOfDevice(path, "")
}

func IsMountpointOfDevice(path, devicepath string) (bool, error) {
	path = strings.TrimSuffix(path, "/")
	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) <= 1 {
			continue
		}
		if (fields[1] == path || path == "") && (fields[0] == devicepath || devicepath == "") {
			return true, nil
		}
	}

	return false, nil
}

func isMounted(path string) (bool, error) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, path) {
			return true, nil
		}
	}

	return false, nil
}

func CleanRoots(config StackerConfig) error {
	// TODO - remove the workdir and upperdir
	subvolErr := btrfsSubVolumesDelete(config.RootFSDir)
	loopback := path.Join(config.StackerDir, "btrfs.loop")

	var umountErr error
	_, err := os.Stat(loopback)
	if err == nil {
		umountErr = syscall.Unmount(config.RootFSDir, syscall.MNT_DETACH)
	}
	if subvolErr != nil && umountErr != nil {
		return errors.Errorf("both subvol delete and umount failed: %v, %v", subvolErr, umountErr)
	}

	if subvolErr != nil {
		return subvolErr
	}

	return umountErr
}
