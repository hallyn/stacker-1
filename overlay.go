package stacker

import (
	"os/exec"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"syscall"

	"github.com/anuvu/stacker/log"
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
	wd := path.Join(o.c.StackerDir, "overlay.workdir", source)
	ud := path.Join(o.c.StackerDir, "overlay.upperdir", source)
	os.MkdirAll(wd, 0750)
	os.MkdirAll(ud, 0750)
	return os.MkdirAll(path.Join(o.c.RootFSDir, source), 0755)
}

func (o *overlay) Snapshot(source string, target string) error {
	src := path.Join(o.c.RootFSDir, source)
	wd := path.Join(o.c.StackerDir, "overlay.workdir", source)
	ud := path.Join(o.c.StackerDir, "overlay.upperdir", source)
	dest := path.Join(o.c.RootFSDir, target)
	fmt.Printf("creating snapshot %s from %s\n", target, source)
	os.MkdirAll(wd, 0755)
	os.MkdirAll(ud, 0755)
	os.MkdirAll(dest, 0755)
	opts := fmt.Sprintf("workdir=%s,upperdir=%s,lowerdir=%s", wd, ud, src)
	output, err := exec.Command("mount", "-t", "overlay", "-o", opts, src, dest).CombinedOutput()
	if err != nil {
		return errors.Errorf("overlay %s -o %s onto %s failed: %s", src, opts, dest, output)
	}
	fmt.Printf("created snapshot %s from %s\n%s\n", target, source, output)
	return nil
}

func (o *overlay) Restore(source string, target string) error {
	err := o.Snapshot(source, target)
	if err != nil {
		return err
	}
	dir := path.Join(o.c.RootFSDir, target)
	fmt.Printf("restoring snapshot %s from %s\n", target, source)
	return syscall.Mount(dir, dir, "none", unix.MS_BIND|unix.MS_REMOUNT, "")
}

func (o *overlay) Delete(target string) error {
	fmt.Printf("Deleting %s\n", target)
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

func (o *overlay) Detach() error {
	return nil
}

func (o *overlay) Exists(thing string) bool {
	mounted, err := isMounted(path.Join(o.c.RootFSDir, thing))
	return err == nil && mounted
}

func (o *overlay) MarkReadOnly(thing string) error {
	fmt.Printf("Marking %s readonly\n", thing)
	// XXX marking readonly fails.  can't even bind mount.  Is this
	// not being done in a private mntns?  May require a restructuring :(
	return nil
	dir := path.Join(o.c.RootFSDir, thing)
	err := syscall.Mount(dir, dir, "none", unix.MS_BIND, "")
	if err != nil {
		fmt.Printf("Error bind mounting %s to make it ro\n", thing)
		return errors.Wrapf(err, "Error bind mounting %s to make it ro", thing)
	}
	err = syscall.Mount(dir, dir, "none", unix.MS_BIND|unix.MS_RDONLY|unix.MS_REMOUNT, "")
	if err != nil {
		fmt.Printf("Error marking %s readonly\n", thing)
		return errors.Wrapf(err, "Error marking %s readonly", thing)
	}
	return err
}

func (o *overlay) TemporaryWritableSnapshot(source string) (string, func(), error) {
	dir, err := ioutil.TempDir(o.c.RootFSDir, fmt.Sprintf("temp-snapshot-%s-", source))
	if err != nil {
		return "", nil, errors.Wrapf(err, "couldn't create temporary snapshot dir for %s", source)
	}

	err = os.RemoveAll(dir)
	if err != nil {
		return "", nil, errors.Wrapf(err, "couldn't remove tempdir for %s", source)
	}

	dir = path.Base(dir)

	err = o.Snapshot(source, dir)
	if err != nil {
		return "", nil, errors.Wrapf(err, "snapshotting %s onto %s", source, dir)
	}

	cleanup := func() {
		err = o.Delete(dir)
		if err != nil {
			log.Infof("problem deleting temp subvolume %s: %v", dir, err)
			return
		}
		err = os.RemoveAll(dir)
		if err != nil {
			log.Infof("problem deleting temp subvolume dir %s: %v", dir, err)
		}
	}

	return dir, cleanup, nil
}
