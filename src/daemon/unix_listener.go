package daemon

import (
    "net"
    "os"
    "syscall"
    "log"
)

const (
    unixPasswdPath = "/etc/passwd"
    unixGroupPath  = "/etc/group"
)

// NewUnixSocket creates a unix socket with the specified path and group.
func NewUnixSocket(path, group string) (net.Listener, error) {
    if err := syscall.Unlink(path); err != nil && !os.IsNotExist(err) {
        return nil, err
    }
    mask := syscall.Umask(0777)
    defer syscall.Umask(mask)
    l, err := net.Listen("unix", path)
    if err != nil {
        return nil, err
    }
    if err := setSocketGroup(path, group); err != nil {
        l.Close()
        return nil, err
    }
    if err := os.Chmod(path, 0660); err != nil {
        l.Close()
        return nil, err
    }
    return l, nil
}

func setSocketGroup(path, group string) error {
    if group == "" {
        return nil
    }
    if err := ChangeGroup(path, group); err != nil {
        if group != "docker" {
            return err
        }
        log.Printf("Warning: could not change group %s to docker: %v", path, err)
    }
    return nil
}

