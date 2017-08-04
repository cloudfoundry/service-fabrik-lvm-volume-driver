package daemon

import (
      "bytes"
      "fmt"
      "log"
      "os"
      "os/exec"
      "path/filepath"
      "bufio"
      "syscall"
      "strings"
      "strconv"
      "errors"
      "regexp"
)

const (
    LVM_MAPPER_DIR = "/dev/mapper"
    LVM_LIST_VOLUMES_NAME = "lvs"
    DEFAULT_FILESYSTEM = "ext4"
)


type Volume struct {
    Name string
    Mountpoint string
}

type Volumes struct {
    Volumes []Volume
    Err string
}

// --------------------------------------------------------------------------
// Helper
// --------------------------------------------------------------------------

func arrayToMap(names []string) (map[string]bool) {
    m := make(map[string]bool)
    for _, v := range names {
        m[v] = true
    }
    return m
}

func stringInSlice(a string, list []Volume) bool {
    for _, b := range list {
        if b.Name == a {
            return true
        }
    }
    return false
}

func getSizeFromName(name string) (int) {

    pattern := "-oS([0-9]+)([MG])?$"
    var nameRegex = regexp.MustCompile(pattern)
    factor := 1
    if m := nameRegex.FindStringSubmatch(name); m != nil {
        if m[2] == "G" {
            factor = 1024
        }
        v, _ := strconv.Atoi(m[1])
        fmt.Printf("Creating %d mb file system.", (v*factor))
        return v * factor
    } else {
        return 0
    }
}

// --------------------------------------------------------------------------
// Wrapper for execution of external programs
// --------------------------------------------------------------------------

type ExecStatus struct {
    cmd string
    stdout string
    stderr string
    status int
}

func (e ExecStatus) String() (string) {
    return "Command \"" + e.cmd + "\" failed with status: " + strconv.Itoa(e.status) + ": " + e.stdout + e.stderr
}

func runCommand(cmdName string, args []string) (execStatus ExecStatus) {

    vcmd := cmdName
    for _,v := range args {
        vcmd += " " + v
    }
    cmd := exec.Command(cmdName, args...)
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    if execError := cmd.Run(); execError != nil {
        if exitError, ok := execError.(*exec.ExitError); ok {
            waitStatus := exitError.Sys().(syscall.WaitStatus)
            return ExecStatus{
                       cmd: vcmd, 
                       stdout: stdout.String(), 
                       stderr: stderr.String(), 
                       status: waitStatus.ExitStatus(),
                   }
        } else {
            return ExecStatus{cmd: vcmd, stdout: stdout.String(), stderr: stderr.String(), status: 1 }
        }
    } else {
        return ExecStatus{cmd: vcmd, stdout: stdout.String(), status: 0}
    }
}

// --------------------------------------------------------------------------
// Volume Driver Implementation
// --------------------------------------------------------------------------

type VolumeDriver struct {
    MountRoot string
    LvmDevice string
    VolumeGroupName string
    DefaultLogicalVolumeSize int
    Debug bool
}

func makefs(device string) (error) {
    cmd := "mkfs." + DEFAULT_FILESYSTEM
    if status := runCommand(cmd,[]string{device}); status.status != 0 {
        return errors.New("Cannot create filesystem on volume " + strconv.Itoa(status.status) + ": " + status.stderr)
    } else {
        return nil
    }
}

func mount(device string, dir string, options []string) (error) {

    allOptions := append(options, device, dir)

    if status := runCommand("mount",allOptions); status.status != 0 {
        msg := "Cannot mount device " + device + ": " + status.stderr
        return errors.New(msg)
    } else {
        return nil
    }
}

func (d* VolumeDriver) getMountpoint(name string) (string) {
    return filepath.Join(d.MountRoot, name)
}

func (d* VolumeDriver) getDeviceName(name string) (string) {
    return filepath.Join("/dev", d.VolumeGroupName, name)
}

func (d *VolumeDriver) createVolume(name string, size int) (error) {

    sizeStr := strconv.Itoa(size) + "M"
    if status := runCommand("lvcreate",[]string{"-L", sizeStr, "-n", name, d.VolumeGroupName}); status.status != 0 {
        msg := "Cannot create volume, return code is " + strconv.Itoa(status.status) + ": " + status.stderr
        return errors.New(msg)
    } else {
        return nil
    }
}

func (d *VolumeDriver) removeMountpoint(volume string) (error) {
    // must remove mount point
    mp := d.getMountpoint(volume)

    if status := runCommand("rmdir", []string{mp}); status.status != 0 {
        return errors.New("Volume " + volume + " removed but deletion of mountpoint failed: " + status.String())
    }
    log.Println("Volume " + volume + " removed and mountpoint " + mp + " deleted.")
    return nil

}

func (d *VolumeDriver) removeLogicalVolume(volume string) (error) {
    device := d.getDeviceName(volume)
    if mounted, err := d.isMounted(volume); mounted {
        return errors.New("Volume " + volume + " is still mounted")
    } else if err != nil {
        return err
    }
    if status := runCommand("lvremove",[]string{"-f", device}); status.status != 0 {
        return errors.New(status.String())
    }
    // mountpoint should not exist anymore - so errors will be 
    // ignored, just in case
    d.removeMountpoint(volume)
    return nil
}


func (d *VolumeDriver) listVolumes() (*[]Volume, error) {

    volumes, err1 := d.getMountedVolumes()
    if err1 != nil {
        return nil, err1
    }
    vmap := arrayToMap(*volumes)

    status := runCommand("lvs", []string{"--noheadings", "-o", "lv_name", "-v", d.VolumeGroupName})
    if status.status != 0 {
        return nil, errors.New(status.String())
    } else {
        in := bufio.NewScanner(strings.NewReader(status.stdout))
        var volumes []Volume = make([]Volume, 0)
        for in.Scan() {
            s := strings.Trim(in.Text(), "\n ")
            volume := Volume{Name:s}
            if _, ok := vmap[s]; ok {
                volume.Mountpoint = d.getMountpoint(s)
            }
            volumes = append(volumes, volume)
        }
        return &volumes, nil
    }
}

func (d *VolumeDriver) existsVolume(name string) (bool, error) {
    
    if vols, err := d.listVolumes(); err == nil {
        return stringInSlice(name, *vols), nil
    } else {
        return false, err
    }

}

func (d *VolumeDriver) getMountedVolumes() (*[]string, error) {

    if result := runCommand("mount", []string{}); result.status == 0 {
        in := bufio.NewScanner(strings.NewReader(result.stdout))
        var volumes []string
        for in.Scan() {
            line := in.Text()
            if strings.Contains(line, d.MountRoot) {
                mountpath := strings.Split(line, " ")[2]
                vol := mountpath[len(d.MountRoot)+1:len(mountpath)]
                volumes = append(volumes, vol)
            }
        }
        return &volumes, nil
    } else {
        return nil, errors.New("Unable to retrieve mountpoints. Return code: " + strconv.Itoa(result.status) + ": " + result.stderr)
    }
}

func (d *VolumeDriver) isMounted(volume string) (bool, error) {
    vols, err := d.getMountedVolumes()
    if err != nil {
        return false, err
    }
    for _, v := range *vols {
        if v == volume {
            return true, nil
        }
    }
    return false, nil
}

func (d *VolumeDriver) unmount(device string) (error) {

    if status := runCommand("umount",[]string{device}); status.status != 0 {
        return errors.New("Cannot unmount device " + device + ": " + status.stderr)
    } else {
        return nil
    }
}


func (d *VolumeDriver) DockerActivate() (implements []string) {
    m := []string{"VolumeDriver"}
    return m
}


// Create new logical volume and create an ext4 filesystem
// 
func (d *VolumeDriver) DockerCreateVolume(name string, options map[string]string) (error) {

    log.Print("/VolumeDriver.Create called for volume " + name)
    var size int
    sizeFromName := getSizeFromName(name)
    if val, ok := options["size"]; ok {
        size,_ = strconv.Atoi(val)
    } else if sizeFromName != 0 {
        size = sizeFromName
    } else {
        size = d.DefaultLogicalVolumeSize
    }
    if exists, err := d.existsVolume(name); !exists && err == nil {

        log.Print("Creating volume " + name + " with size " + strconv.Itoa(size) + "MB")
        if err := d.createVolume(name, size) ; err != nil {
            return err
        } else {
            return makefs(d.getDeviceName(name))
        }
    } else {
        if err != nil {
            return err
        } else {
            return errors.New("Volume " + name + " already exists")
        }
    }
}

func (d *VolumeDriver) DockerRemoveVolume(name string) (error) {
    log.Print("/VolumeDriver.Remove called for volume " + name)
    if yes, err := d.existsVolume(name); yes {
        log.Print("Volume " + name + " exists, deleting")
        if mounted, err2 := d.isMounted(name); mounted && err2 == nil {
            return errors.New("Volume " + name + " is still mounted")
        } else if err2 != nil {
            return  err2
        } else {
            return d.removeLogicalVolume(name)
        }
    } else {
        if err != nil {
            return err 
        } else {
            msg := "Cannot delete volume " + name + ": it does not exist"
            log.Print(msg)
            return errors.New(msg)
        }
    }
}

func (d *VolumeDriver) DockerMountVolume(name string) (*string, error) {

    log.Print("/VolumeDriver.Mount called for volume " + name)
    mountpoint := d.getMountpoint(name)
    device := d.getDeviceName(name)

    if mounted, error := d.isMounted(name); mounted && error == nil {
        log.Print("Remounting already mounted volume " + name)
        return &mountpoint, nil
    } else if error != nil {
        return nil, error
    }

    if error := os.MkdirAll(mountpoint, 0750); error != nil {
        return nil, error
    } else {
        if error := mount(device, mountpoint, []string{}); error != nil {
            return nil, error
        } else {
            return &mountpoint, nil
        }
    }

}

func (d *VolumeDriver) DockerUnmountVolume(name string) (error) {

    log.Print("/VolumeDriver.Unmount called for volume " + name)
    var err error
    device := d.getDeviceName(name)
    if err = d.unmount(device); err != nil {
        if err != nil {
            log.Print("Ignoring unmount error " + err.Error())
        }
        // mountpoint has most likely been deleted before, therefore 
        // ignoring possible errors here
        d.removeMountpoint(name)
        return nil
    } else {
        return d.removeMountpoint(name)
    }
}

func (d *VolumeDriver) DockerVolumePath(name string) (*string, error) {

    if d.Debug {
        log.Print("/VolumeDriver.Path called for volume " + name)
    }
    if mounted, err := d.isMounted(name); mounted {
        mountpoint := d.getMountpoint(name)
        return &mountpoint, nil
    } else {
        if err != nil {
            return nil, err
        } else {
            return nil, errors.New("Volume " + name + " not mounted")
        }
    }
}

func (d *VolumeDriver) dockerGetVolume(name string) (*Volume, error) {

    if d.Debug {
        log.Print("/VolumeDriver.Get called for volume " + name)
    }
    if exists, err := d.existsVolume(name); !exists {
        return nil, errors.New("Volume " + name + " does not exist")
    } else if err != nil {
        return nil, err
    } else {
        vol := Volume{
            Name: name,
        }
        if mounted, err := d.isMounted(name); mounted && err == nil {
            mountpoint := d.getMountpoint(name)
            vol.Mountpoint = mountpoint
            return &vol, nil
        } else {
            return &vol, err
        }
    }
}

func (d *VolumeDriver) dockerListVolume() (map[string]interface{}, error) {

    if d.Debug {
        log.Print("/VolumeDriver.List called")
    }
    vs := make(map[string]interface{})
    if list, err := d.listVolumes(); err != nil {

        vs["Err"] = err.Error()
        return vs, err
    } else {
        vs["Volumes"] = *list
        vs["Err"] = ""
        return vs, nil
    }
}

func (d *VolumeDriver) EnsureVGExists() (error) {

    pattern := "\"" + d.VolumeGroupName + "\"" 
    var nameRegex = regexp.MustCompile(pattern)
    if result := runCommand("vgdisplay", []string{"-s"}); result.status == 0 {
        in := bufio.NewScanner(strings.NewReader(result.stdout))
        for in.Scan() {
            line := in.Text()
            if nameRegex.MatchString(line) {
                return nil
            }
        }
        return errors.New("No such volume group: " + d.VolumeGroupName)
    } else {
        return errors.New("Cannot run program pvdisplay: " + result.String())
    }
}

func (d *VolumeDriver) EnsureMountpointExists() (error) {

    if fi, err := os.Lstat(d.MountRoot); err == nil {
        if fi.IsDir() {
            // ok, make sure rights are correct (only if group docker exists,
            // otherwise fail silently)
            ChangeGroup(d.MountRoot, "docker")
            return nil
        } else {
            return errors.New(d.MountRoot + " exists and does not appear to be a directory.")
        }
    } else {
        // assume file does not exist
        if err2 := os.MkdirAll(d.MountRoot, 0750); err2 == nil {
            ChangeGroup(d.MountRoot, "docker")
            return nil
        } else {
            return err2
        }
    }
}