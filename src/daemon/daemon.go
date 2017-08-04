package daemon

import (
      "fmt"
      "net/http"
      "log"
      "encoding/json"
      "strconv"
      "os"
      "path/filepath"
      "sync"
)

const (
    DefaultContentTypeV1_1 = "application/vnd.docker.plugins.v1.1+json"
    DefaultJsonLocation = "/etc/docker/plugins"
    DefaultSocketSpecLocation = "/run/docker/plugins"
    VolumeDriverName = "lvm-volume-driver"
    DefaultVolumeSize = 512
)

var volumeDriver VolumeDriver

type CapabilitiesResp struct {
    Capabilities VolumeDriverCapabilities
}

type VolumeDriverCapabilities struct {
    Scope string
}

func writeJson(jsonObject interface{}, code int, w http.ResponseWriter, debug bool) {

    msg, _ := json.Marshal(jsonObject)
    w.Header().Set("Content-Type", DefaultContentTypeV1_1)
    w.WriteHeader(code)
    w.Write(msg)
    if debug {
        log.Printf("%d: %s", code, msg)
    }
}


func ensureContentType(r *http.Request) (error) {

    if r.Header.Get("Content-Type") != "application/json" {
        // Docker does not send Content-Type header
        return nil //errors.New("Expected content type application/json but got " + r.Header.Get("Content-Type"))
    } else {
        return nil
    }
}

func logRequest(r *http.Request, body string) {

    log.Printf("Request: %s %s, body: %s", r.Method, r.URL, body)
}

func decodeRequest(r *http.Request, debug bool) (map[string]interface{}, error) {

    if err := ensureContentType(r); err == nil {
        dec := json.NewDecoder(r.Body)
        var m map[string]interface{}
        if err2 := dec.Decode(&m); err2 == nil {
            msg, _ := json.Marshal(m)
            if debug {
                logRequest(r, string(msg))
            }
            return m, nil
        } else {
            return nil, err2
        }
    } else {
        return nil, err
    }
}

func getValue(doc map[string]interface{}, key string) (*string) {

    if val, ok := doc[key]; ok {
        if value, ok := val.(string); ok {
            return &value
        }
    }
    return nil
}

func getMap(doc map[string]interface{}, key string) (map[string]interface{}) {

    if val, ok := doc[key]; ok {
        if m, ok := val.(map[string]interface{}); ok {
            return m
        }
    }
    return nil
}

func getOptions(r *http.Request, debug bool) (map[string]string) {
    if doc, err := decodeRequest(r, debug); err == nil {
        if opts := getMap(doc, "Opts"); opts != nil {
            options := make(map[string]string)
            if size := getValue(opts, "Size"); size != nil {
                options["size"] = *size
            }
            return options
        }
    }
    return nil
}

func getName(w http.ResponseWriter, r *http.Request, debug bool) (*string) {

    var name *string = nil
    msg := make(map[string]interface{})
    if m, err := decodeRequest(r, debug); err != nil {
        msg["Err"] = err.Error()
        writeJson(msg, http.StatusOK, w, debug)
    } else {
        if name = getValue(m, "Name"); name == nil {
            msg["Err"] = "Illegal request"
            writeJson(msg, http.StatusOK, w, debug)
        }
    }
    return name
}

// --------------------------------------------------------------------------
// Daemon
// --------------------------------------------------------------------------

type Daemon struct {
    Listener string
    LvmDevice string
    MountRoot string
    VolumeGroupName string
    DefaultLogicalVolumeSize int
    SocketSpecLocation string
    JsonLocation string
    Host string
    Port int
    Debug bool
    // need to ensure that we don't handle concurrent calls
    m *sync.Mutex
}

// Handler methods invoked by Docker

func (d *Daemon) pluginActivate(w http.ResponseWriter, r *http.Request) {

    d.m.Lock()
    defer d.m.Unlock()

    if d.Debug {
        logRequest(r, "")
    }
    type response struct {
        Implements []string
    }

    resp := response{
        Implements: volumeDriver.DockerActivate(),
    }
    writeJson(resp, http.StatusOK, w, d.Debug)
}

func (d *Daemon) volumeDriverCreate(w http.ResponseWriter, r *http.Request) {

    d.m.Lock()
    defer d.m.Unlock()

    if name := getName(w, r, d.Debug); name != nil {
        msg := make(map[string]interface{})
        options := getOptions(r, d.Debug)
        if err := volumeDriver.DockerCreateVolume(*name, options); err != nil {
            msg["Err"] = err.Error()
            writeJson(msg, http.StatusOK, w, d.Debug)
            log.Print(err.Error())
        } else {
            msg["Err"] = ""
            writeJson(msg, http.StatusOK, w, d.Debug)
        }
    }
}

// Remove logical volume
// pre: - volume must exist
//      - volume must not be mounted
func (d *Daemon) volumeDriverRemove(w http.ResponseWriter, r *http.Request) {

    d.m.Lock()
    defer d.m.Unlock()

    if name := getName(w, r, d.Debug); name != nil {
        msg := make(map[string]interface{})
        if err := volumeDriver.DockerRemoveVolume(*name); err != nil {
            msg["Err"] = err.Error()
            writeJson(msg, http.StatusOK, w, d.Debug)
            log.Print(err.Error())
        } else {
            msg["Err"] = ""
            writeJson(msg, http.StatusOK, w, d.Debug)
        }
    }
}

func (d *Daemon) volumeDriverMount(w http.ResponseWriter, r *http.Request) {

    d.m.Lock()
    defer d.m.Unlock()

    if name := getName(w, r, d.Debug); name != nil {
        msg := make(map[string]interface{})
        if mountpoint, err := volumeDriver.DockerMountVolume(*name); err != nil {
            msg["Err"] = err.Error()
            writeJson(msg, http.StatusOK, w, d.Debug)
            log.Print(err.Error())
        } else {
            msg["Mountpoint"] = mountpoint
            msg["Err"] = ""
            writeJson(msg, http.StatusOK, w, d.Debug)
        }
    }
}

func (d *Daemon) volumeDriverPath(w http.ResponseWriter, r *http.Request) {

    d.m.Lock()
    defer d.m.Unlock()

    if name := getName(w, r, d.Debug); name != nil {
        msg := make(map[string]interface{})
        if mountpoint, err := volumeDriver.DockerVolumePath(*name); err != nil {
            msg["Err"] = err.Error()
            writeJson(msg, http.StatusOK, w, d.Debug)
            if d.Debug { log.Print(err.Error())}
        } else {
            msg["Mountpoint"] = mountpoint
            msg["Err"] = ""
            writeJson(msg, http.StatusOK, w, d.Debug)
        }
    }
}

func (d *Daemon) volumeDriverUnmount(w http.ResponseWriter, r *http.Request) {

    d.m.Lock()
    defer d.m.Unlock()

    if name := getName(w, r, d.Debug); name != nil {
        msg := make(map[string]interface{})
        if err := volumeDriver.DockerUnmountVolume(*name); err != nil {
            msg["Err"] = err.Error()
            writeJson(msg, http.StatusOK, w, d.Debug)
            log.Print(err.Error())
        } else {
            msg["Err"] = ""
            writeJson(msg, http.StatusOK, w, d.Debug)
        }
    }
}

func (d *Daemon) volumeDriverGet(w http.ResponseWriter, r *http.Request) {

    d.m.Lock()
    defer d.m.Unlock()

    if name := getName(w, r, d.Debug); name != nil {
        msg := make(map[string]interface{})
        if volume, err := volumeDriver.dockerGetVolume(*name); err != nil {
            msg["Err"] = err.Error()
            writeJson(msg, http.StatusOK, w, d.Debug)
            if d.Debug { log.Print(err.Error())}
        } else {
            msg["Volume"] = volume
            msg["Err"] = ""
            writeJson(msg, http.StatusOK, w, d.Debug)
        }
    }
}

func (d *Daemon) volumeDriverList(w http.ResponseWriter, r *http.Request) {

    d.m.Lock()
    defer d.m.Unlock()
    // no message expected
    if msg, err := volumeDriver.dockerListVolume(); err == nil {
        writeJson(msg, http.StatusOK, w, d.Debug)
    } else {
        // error already part of message
        writeJson(msg, http.StatusOK, w, d.Debug)
    }
}

func (d *Daemon) volumeCapabilities(w http.ResponseWriter, r *http.Request) {

    capabilitiesResp := &CapabilitiesResp{
        Capabilities: VolumeDriverCapabilities{
            Scope: "local",
        },
    }
     writeJson(capabilitiesResp, http.StatusOK, w, d.Debug)
}


func (s *Daemon) StartServer() {

    s.m = new(sync.Mutex)

    if err := RootCheck(); err != nil {
        fmt.Fprintf(os.Stderr, err.Error() + "\n")
        os.Exit(1)
    }

    volumeDriver = VolumeDriver{
        MountRoot: s.MountRoot,
        LvmDevice: s.LvmDevice,
        VolumeGroupName: s.VolumeGroupName,
        DefaultLogicalVolumeSize: s.DefaultLogicalVolumeSize,
        Debug: s.Debug,
    }

    if err := volumeDriver.EnsureVGExists(); err != nil {
        fmt.Fprintf(os.Stderr, err.Error() + "\n")
        os.Exit(1)
    }

    if err := volumeDriver.EnsureMountpointExists(); err != nil {
        fmt.Fprintf(os.Stderr, err.Error() + "\n")
        os.Exit(1)
    }

    http.HandleFunc("/Plugin.Activate", s.pluginActivate)
    http.HandleFunc("/VolumeDriver.Create", s.volumeDriverCreate)
    http.HandleFunc("/VolumeDriver.Remove", s.volumeDriverRemove)
    http.HandleFunc("/VolumeDriver.Mount", s.volumeDriverMount)
    http.HandleFunc("/VolumeDriver.Path", s.volumeDriverPath)
    http.HandleFunc("/VolumeDriver.Unmount", s.volumeDriverUnmount)
    http.HandleFunc("/VolumeDriver.Get", s.volumeDriverGet)
    http.HandleFunc("/VolumeDriver.List", s.volumeDriverList)
    http.HandleFunc("/VolumeDriver.Capabilities", s.volumeCapabilities)

    switch s.Listener {
    case "unix":
        socketDir := filepath.Dir(s.SocketSpecLocation)
        if err := Mkdir(socketDir, 0750, "docker"); err != nil {
            fmt.Fprint(os.Stderr, err.Error() + "\n")
            os.Exit(1)
        }
        if listener, err := NewUnixSocket(s.SocketSpecLocation, "docker"); err != nil {
            fmt.Fprintf(os.Stderr, "Cannot create socket file: %s\n", err.Error())
            os.Exit(1)
        } else {
            s := &http.Server{}
            s.SetKeepAlivesEnabled(false)
            err := s.Serve(listener)
            if err != nil {
                fmt.Fprintf(os.Stderr, "%s\n" + err.Error())
                os.Exit(1)
            }
        }

    case "http":

        if err := WriteSpecFile(s.JsonLocation, `
{
     "Name": "lvm-volume-driver",
     "Addr": "http://` + s.Host + ":" + strconv.Itoa(s.Port) + `"
}`); err != nil {
            fmt.Fprintf(os.Stderr, err.Error() + "\n")
            os.Exit(1)
        }



        log.Printf("Start listening on %s:%d\n", s.Host, s.Port)
        err := http.ListenAndServe(s.Host + ":" + strconv.Itoa(s.Port), nil)
        if err != nil {
            fmt.Fprintf(os.Stderr, err.Error() + "\n")
            os.Exit(1)
        }
    default:
        fmt.Fprintf(os.Stderr, "unrecognized listener " + s.Listener)
        os.Exit(1)
    }
}
