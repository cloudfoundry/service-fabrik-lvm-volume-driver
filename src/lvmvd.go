/*
 * Simple LVM volume driver for docker to be used by the containers broker
 * 
 * see https://github.wdf.sap.corp/d040949/lvm-volume-driver 
 *
 * for further information
 */

package main

import (
    "fmt"
    "daemon"
    "flag"
    "os"
    "os/signal"
    "strconv"
    "syscall"
    "path/filepath"
)

var usage =
`usage: %s [options]

This program implements a volume driver for docker. The name is name of the
volume driver is lvm-volume-driver.

The following options can be specified:

  --listener=http|unix       listen on a unix socket or http port 
                             (optional, default: unix)
  --host=<host>              host name in case http is specified (optional, 
                             default: localhost)
  --port=<port>              port number in case http is specified (optional,
                             default: 8080)
  --default-size=<size>      default size in megabytes for volumes in case no
                             size is specified (default: 512MB)
  --mount-root=<directory>   root directory for mount points (required)
  --volume-group-name=<name> name of volume group (required)
  --sock-file                name of file for socket spec file (default:
                             /run/docker/plugins/lvm-volume-driver.sock)
  --json-file                Name of directory for json file (default:
                             /etc/docker/plugins/lvm-volume-driver.json)
`

// --------------------------------------------------------------------------
// Try not to leave stale files behind
// --------------------------------------------------------------------------

func cleanup(d *daemon.Daemon) {
    os.Remove(d.JsonLocation)
    os.Remove(d.SocketSpecLocation)
}

func registerCleanupHandler(d *daemon.Daemon) {
    c:= make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt)
    signal.Notify(c, syscall.SIGTERM)
    signal.Notify(c, syscall.SIGKILL)
    go func() {
        <-c
        cleanup(d)
        os.Exit(0)
    }()
}

// --------------------------------------------------------------------------
// Program entry point
// --------------------------------------------------------------------------

func main() {

    listener := flag.String("listener", "unix", "listen on a unix socket or http port")
    mount_root := flag.String("mount-root", "", "root directory for mount points (required)")
    volumeGroupName := flag.String("volume-group-name", "", "name of volume group (required)")
    defaultLogicalVolumeSize := flag.Int("default-size", daemon.DefaultVolumeSize, "default size of volume in megabytes")
    host := flag.String("host", "localhost", "host name in case http is specified")
    port := flag.String("port", "8080", "port number in case http is specified")
    sock := flag.String("sock-file", "", "name of file for socket spec file")
    jsonf := flag.String("json-file", "", "Name of directory for json file")
    debug := flag.Bool("debug", false, "Print verbose debug output")
    flag.Parse()

    if *mount_root == "" {
        fmt.Fprintf(os.Stderr, "must specify a root directory for mounted filesystems\n" + usage, os.Args[0])
        os.Exit(1)
    }

    portnum, _ := strconv.Atoi(*port)
    d := &daemon.Daemon{
        Host: *host, 
        Port: portnum,
        Listener: *listener,
        MountRoot: *mount_root,
        VolumeGroupName: *volumeGroupName,
        DefaultLogicalVolumeSize: *defaultLogicalVolumeSize,
        SocketSpecLocation: *sock,
        Debug: *debug,
    }
    if *jsonf != "" {
        d.JsonLocation = *jsonf
    } else {
        d.JsonLocation = filepath.Join(daemon.DefaultJsonLocation, daemon.VolumeDriverName + ".json")
    }
    if *sock != "" {
        d.SocketSpecLocation = *sock
    } else {
        d.SocketSpecLocation = filepath.Join(daemon.DefaultSocketSpecLocation, daemon.VolumeDriverName + ".sock")
    }

    registerCleanupHandler(d)

    d.StartServer()
}