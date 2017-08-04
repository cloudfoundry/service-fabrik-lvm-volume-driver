package daemon

//
// note: contains code snippets from github.com/docker

import (
    "io"
    "strconv"
    "os"
    "os/user"
    "strings"
    "bufio"
    "errors"
    "io/ioutil"
    "path/filepath"
)

const (
    UnixPasswdPath = "/etc/passwd"
    UnixGroupPath  = "/etc/group"
)

func Mkdir(path string, mode os.FileMode, group string) (error) {

    if fi, err := os.Lstat(path); err == nil {
        if fi.IsDir() {
            // ok, make sure rights are correct
            ChangeGroup(path, group)
            return nil
        } else {
            return errors.New(path + " exists and does not appear to be a directory.")
        }
    } else {
        // assume file does not exist
        if err2 := os.MkdirAll(path, mode); err2 == nil {
            ChangeGroup(path, group)
            return nil
        } else {
            return err2
        }
    }
}


func RootCheck() (error) {
    if u, err := user.Current() ; err != nil {
        return err
    } else {
        if u.Uid != "0" {
            return errors.New(os.Args[0] + " must be run as root.")
        } else {
            return nil
        }
    }
}

func WriteSpecFile(filename, jsonContent string) (error) {

    path := filepath.Dir(filename)
    if _, err1 := os.Lstat(path); err1 != nil {        
        if err2 := os.MkdirAll(path, 0750); err2 != nil {
            return err2
        }
        os.Chmod(path, 0750)
        ChangeGroup(path, "docker")
    }
    err := ioutil.WriteFile(filename, []byte(jsonContent), 0640)
    ChangeGroup(path, "docker")
    return err
}


func ChangeGroup(path string, name string) error {
    gid, err := lookupGidByName(name)
    if err != nil {
        return err
    } else {
        return os.Chown(path, 0, gid)
    }
}

func lookupGidByName(name string) (int, error) {
    groupFile := UnixGroupPath
    groups, err := ParseGroupFile(groupFile, name)
    if err != nil {
        return -1, err
    }
    if groups != nil && len(groups) > 0 {
        return groups[0].Gid, nil
    } else {
        return -1, errors.New("Group " + name + " not found")
    }
}

func GetPasswd() (io.ReadCloser, error) {
    return os.Open(unixPasswdPath)
}

func GetGroupPath() (string, error) {
    return unixGroupPath, nil
}

func ParseGroupFile(path, name string) ([]Group, error) {
    if group, err := os.Open(path); err != nil {
        return nil, err
    } else {
        defer group.Close()
        return ParseGroupFilter(group, name)
    }
}

func ParseGroupFilter(r io.Reader, name string) ([]Group, error) {
    if r == nil {
        return nil, errors.New("nil source for group-formatted data")
    }

    var (
        s   = bufio.NewScanner(r)
        out = []Group{}
    )

    for s.Scan() {
        if err := s.Err(); err != nil {
            return nil, err
        }

        text := s.Text()
        if text == "" {
            continue
        }

        // see: man 5 group
        //  group_name:password:GID:user_list
        // Name:Pass:Gid:List
        //  root:x:0:root
        //  adm:x:4:root,adm,daemon
        p := Group{}
        parseLine(
            text,
            &p.Name, &p.Pass, &p.Gid, &p.List,
        )
        if p.Name == name {
            out = append(out, p)
        }
    }
    return out, nil
}

type Group struct {
    Name string
    Pass string
    Gid  int
    List []string
}

func parseLine(line string, v ...interface{}) {
    if line == "" {
        return
    }

    parts := strings.Split(line, ":")
    for i, p := range parts {
        if len(v) <= i {
            // if we have more "parts" than we have places to put them, bail for great "tolerance" of naughty configuration files
            break
        }

        switch e := v[i].(type) {
        case *string:
            // "root", "adm", "/bin/bash"
            *e = p
        case *int:
            // "0", "4", "1000"
            // ignore string to int conversion errors, for great "tolerance" of naughty configuration files
            *e, _ = strconv.Atoi(p)
        case *[]string:
            // "", "root", "root,adm,daemon"
            if p != "" {
                *e = strings.Split(p, ",")
            } else {
                *e = []string{}
            }
        default:
            // panic, because this is a programming/logic error, not a runtime one
            panic("parseLine expects only pointers!  argument " + strconv.Itoa(i) + " is not a pointer!")
        }
    }
}
