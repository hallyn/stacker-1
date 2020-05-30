package stacker

import (
	"os/exec"
	"fmt"
	"os"
	"path"
	"syscall"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

type overlay struct {
	c	StackerConfig
}

func (o *overlay) Name() string {
	return "overlay"
}

func (o *overlay) Create(source string) error {
	wd := path.Join(o.c.StackerDir, "workdir", source)
	ud := path.Join(o.c.StackerDir, "upperdir", source)
	os.MkdirAll(wd, 0750)
	os.MkdirAll(ud, 0750)
	return os.MkdirAll(path.Join(o.c.RootFSDir, source), 0755)
}

func (o *overlay) Snapshot(source string, target string) error {
	src := path.Join(o.c.RootFSDir, source)
	wd := path.Join(o.c.StackerDir, "workdir", source)
	ud := path.Join(o.c.StackerDir, "upperdir", source)
	dest := path.Join(o.c.RootFSDir, target)
	os.MkdirAll(wd, 0750)
	os.MkdirAll(ud, 0750)
	os.MkdirAll(dest, 0750)
	opts := fmt.Sprintf("workdir=%s,upperdir=%s,lowerdir=%s", wd, ud, src)
	output, err := exec.Command("mount", "-t", "overlay", "-o", opts, src, dest).CombinedOutput()
	if err != nil {
		return errors.Errorf("overlay %s -o %s onto %s failed: %s", src, opts, dest, output)
	}
	return nil
}

func (o *overlay) Restore(source string, target string) error {
	err := o.Snapshot(source, target)
	if err != nil {
		return err
	}
	dir := path.Join(o.c.RootFSDir, target)
	return syscall.Mount(dir, dir, "none", unix.MS_BIND|unix.MS_REMOUNT, "")
}

func (o *overlay) Delete(target string) error {
	dir := path.Join(o.c.RootFSDir, target)
	wd := path.Join(o.c.StackerDir, "workdir", target)
	ud := path.Join(o.c.StackerDir, "upperdir", target)
	err := syscall.Unmount(dir, syscall.MNT_DETACH)
	if err != nil {
		return err
	}
	if err = os.RemoveAll(dir); err != nil {
		return errors.Errorf("Failed to remove target dir %s: %s", dir, err)
	}
	if err = os.RemoveAll(wd); err != nil {
		return errors.Errorf("Failed to remove workdir %s: %s", wd, err)
	}
	if err = os.RemoveAll(ud); err != nil {
		return errors.Errorf("Failed to remove upper dir %s: %s", ud, err)
	}

	return nil
}

func (b *btrfs) Detach() error {
	return nil
}

func (o *overlay) Exists(thing string) bool {
	mounted, err := isMounted(path.Join(o.c.RootFSDir, thing))
	return err == nil && mounted
}

func (o *overlay) MarkReadOnly(thing string) error {
	dir := path.Join(o.c.RootFSDir, thing)
	return syscall.Mount(dir, dir, "none", unix.MS_BIND|unix.MS_RDONLY|unix.MS_REMOUNT, "")
}

func (o *overlay) TemporaryWritableSnapshot(source string) (string, func(), error) {
	return "", func() {}, nil
}
