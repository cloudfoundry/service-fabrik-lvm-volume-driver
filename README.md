# lvm-volume-driver

This volume driver plug-in implements the docker volume driver api. It provides volumes to docker images based on LVM logical volumes.

There is one deviation from the API description at https://docs.docker.com/engine/extend/plugins_volume/: a volume may not be used by several containers and the attempt to do so may fail. The volume API demands that the number of mount operations is kept and the volume is only unmounted after the last matching unmount operation. This would require that the plug-in would have to keep some state persistently.

Note: contains some modified lines from https://github.com/docker/docker (file utils.go for parsing the unix group file)

# TODO

The goal is to make this list disappear asap. The getting started may refer to these features that have not been implemented yet.

- [x] rename main to lvmvd
- [x] check that the given volume group exists on startup
- [x] ensure that process runs as root on startup (otherwiese nothing will work)
- [x] check that the mount root directory exists (otherwiese create it)
- [x] create socket /json files for docker on startup
- [x] implement socket listener (this should actually be used)
- [x] make sure everything is single threaded
- [x] rework usage text
- [x] pass size for volume via volume name
- [x] catch ctrl-c and remove the json/spec file
- [x] better parameter validation for incoming requests (avoid denial of service)
- [ ] fix socket-file vs. socket dir issue

Will not do
- [ ] do some initial size management on the volume (not sure what that would look like)

# Build

Sync the repository, cd into the src path and run the following commands:

```
export GOPATH=`pwd`/..
go build lvmvd.go
```

# Getting Started

The lvm volume driver expects that the lvm volume group has been created prior to running the volume driver. Follow these instructions for setting up the volume group:

1. Make sure lvm2 is installed

`sudo apt-get install lvm2`

2. Create sparse file

`truncate -s 50g docker-lvm-volume.img`

3. Set up loopback device with image

`sudo losetup /dev/loop0 docker-lvm-volume.img`

4. Create physical volume:

`sudo pvcreate /dev/loop0`

5. Create volume group with name `services-vg`:

`sudo vgcreate services-vg /dev/loop0`

6. Start the plug-in

`sudo lvmvd --default-size=1 --volume-group-name=services-vg --mount-root=/var/volume/mnt-services-vg`

This will start the lvm volume driver daemon serving volumnes from the `services-vg` volume group and mounting them to `/var/volume/mnt-services-vg` directory

7. Start Docker

`sudo service docker start`

Note: this is an example for volumes in sparse files. You can of course also provide a volume grouped backed by a physical disk.

# Specify Volume Size

The lvm volume driver will create volumes with a size of 512MB by default. This can be changed using the `--default-size` command line parameter.

Another possiblity is to add a postfix to the volume name which specifies the volume size. The following will create a volume of 4 gigabytes:

    my-volume-oS4G

The syntax is as follows:

`-o :` An section follows
`S  :` A size parameters follows
`4  :` parameter for size (4 GB)
`G  :` Gigabytes

Alternatively one can specify `M` for megabytes. If no letter `G` or `M` is specified `M` is assumed.

This should allow additional options for the future, e.g. something like `Text4` for the file system type ext4.

# Tests

There is a `runtest.sh` script which provides an integration test for the lvm volume driver.

The test implements the following checks:
- Validate that some common command line options do work
- Validate that the socket and http modes work
- Creates a lvm volume group and creates volumes inside
- checks all volume driver options and varios error situations

Prerequisites / caveats:
- curl version > 7.40 is required for the socket tests to work
- The setup can be shaky as there are some side effects regarding lvm and loopback devices; especially after failed runs
- lvm2 is required

# Reference

## Commands for working with sparse files and LVM

Create logical volume:

`lvcreate -L 1G -ntest services`

Volume can now be mounted from 

/dev/mapper/services-test 

List volumes in a volume group

`lvdisplay -v services`

Better: list logical volumes

`lvs --noheadings -o lv_name -v services`

Delete logical volume

`lvremove /dev/mapper/service-myvolume`

## Testing

Create volume:

`curl -v --header "Content-Type: application/json" -d '{"Name": "myvolume"}' http://localhost:8080/VolumeDriver.Create`

List

`curl -v --header "Content-Type: application/json" -d '{}' http://localhost:8080/VolumeDriver.List`

Create

`curl -v --header "Content-Type: application/json" -d '{"Name": "test3"}' http://localhost:8080/VolumeDriver.Create`

Mount

`curl -v --header "Content-Type: application/json" -d '{"Name": "test3"}' http://localhost:8080/VolumeDriver.Mount`

Unmount

`curl -v --header "Content-Type: application/json" -d '{"Name": "test3"}' http://localhost:8080/VolumeDriver.Unmount`

starting the program

./main --listener=http --default-size=1G --volume-group-name=services-vg --mount-root=/var/volume/mnt-services-vg

## LICENSE

This project is licensed under the Apache Software License, v. 2 except as noted otherwise in the [LICENSE](LICENSE) file
