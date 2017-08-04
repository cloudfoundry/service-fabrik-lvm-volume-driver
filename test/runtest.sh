#!/bin/bash

# ---------------------------------------------------------------------------
# Test for lvm volume driver
# ---------------------------------------------------------------------------

# ---------------------------------------------------------------------------
# Set this according to your environment
# ---------------------------------------------------------------------------

# this must be curl version > 7.40 in order to support sockets

# set to "0" if you don't have curl > = 7.40 (required to speak to sockets)
TEST_SOCKET_LISTENER=1
CURL=/home/dirk/workspace/curl-7.48.0/src/curl
LVMVD_LOG=/tmp/lvmvd.log
TEST_VOLUME=/tmp/test_vol.disk

# ---------------------------------------------------------------------------

export PORT=8080
export HEADERS="Content-Type: application/json"
export HOST=localhost
export URL_PREFIX=http://${HOST}:${PORT}/VolumeDriver.
export URL_PREFIX1=http://${HOST}:${PORT}/Plugin.Activate
# socket
export SURL_PREFIX=http:/VolumeDriver.
export SURL_PREFIX1=http:/Plugin.Activate

DEFAULT_LOOP=/dev/loop5

compare_json() {
    doc1=$1
    doc2=$2
python - "$doc1" "$doc2" <<END
import json
import sys

def ordered(obj):
    if isinstance(obj, dict):
        return sorted((k, ordered(v)) for k, v in obj.items())
    if isinstance(obj, list):
        return sorted(ordered(x) for x in obj)
    else:
        return obj

if len(sys.argv) < 3:
    print "not enough arguments"
    sys.exit(1)
if sys.argv[1] == "" or sys.argv[2] == "":
    print "Two json objects expected"
    sys.exit(1)

d1=""
d2=""
try:
    d1=json.loads(sys.argv[1])
except ValueError:
    print "not json: " + sys.argv[1]

try:
    d2=json.loads(sys.argv[2])
except ValueError:
    print "not json: " + sys.argv[2]

if ordered(d1.items()) == sorted(d2.items()):
    sys.exit(0)
else:
    print "Expected " + str(d1) + " but received " + str(d2)
    sys.exit(1)
END
    return $?
}

compare() {
    cmd="$1"
    expected="$2"
    actual="$3"
    compare_json "$expected" "$actual"
    local ret=$?
    if [ $ret -ne 0 ]; then
        echo "Test $cmd failed"
    else
        echo "Test $cmd ok"
    fi
    return $ret
}

create_volume() {

    truncate -s 100G ${TEST_VOLUME}
    losetup ${DEFAULT_LOOP} ${TEST_VOLUME}
    pvcreate ${DEFAULT_LOOP}
    vgcreate test-vg ${DEFAULT_LOOP}
}

delete_volume() {
    losetup -d ${DEFAULT_LOOP}
    rm ${TEST_VOLUME}
}


run_tests() {

    socket=$1
    activate_url=$2
    base_url=$3

    empty_err='{"Err":""}'
    ret=0

    # Activate
    val=$(${CURL} $socket -s --header "$HEADERS" ${activate_url})
    expected='{"Implements":["VolumeDriver"]}'
    compare "Activate" "$expected" "$val"
    ((ret+=$?))

    # List
    val=$(${CURL} $socket -s --header "$HEADERS" ${base_url}List)
    compare "List" "{\"Err\":\"\",\"Volumes\":[]}" "$val"
    ((ret+= $?))

    # Create
    AUTOTEST_MSG='{"Name": "autotest1"}'
    val=$(${CURL} $socket -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${base_url}Create)
    compare "Create" '{"Err":""}' "$val"
    ((ret+= $?))

    # Get
    AUTOTEST_MSG='{"Name": "autotest1"}'
    val=$(${CURL} $socket -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${base_url}Get)
    expected='{"Err":"","Volume":{"Name":"autotest1","Mountpoint":""}}'
    compare "Get 1" "$expected" "$val"
    ((ret+=$?))

    # List
    val=$(${CURL} $socket -s --header "$HEADERS" ${base_url}List)
    expected='{"Err":"","Volumes":[{"Name":"autotest1","Mountpoint":""}]}'
    compare "List" "$expected" "$val"
    ((ret+=$?))

    # Remove
    val=$(${CURL} $socket -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${base_url}Remove)
    compare "Remove" $empty_err "$val"
    ((ret+=$?))

    # Create
    AUTOTEST_MSG='{"Name": "autotest1"}'
    val=$(${CURL} $socket -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${base_url}Create)
    compare "Create" '{"Err":""}' $val
    ((ret+=$?))

    # Create again -> error
    AUTOTEST_MSG='{"Name": "autotest1"}'
    val=$(${CURL} $socket -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${base_url}Create)
    expected='{"Err":"Volume autotest1 already exists"}'
    compare "Create again" "$expected" "$val"
    ((ret+=$?))

    # Remove
    AUTOTEST_MSG='{"Name": "autotest1"}'
    val=$(${CURL} $socket -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${base_url}Remove)
    compare "Remove" '{"Err":""}' $val
    ((ret+=$?))

    # List
    val=$(${CURL} $socket -s --header "$HEADERS" ${base_url}List)
    compare "List" "{\"Err\":\"\",\"Volumes\":[]}" "$val"
    ((ret+=$?))

    # Create
    AUTOTEST_MSG='{"Name": "autotest1"}'
    val=$(${CURL} $socket -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${base_url}Create)
    compare "Create" "$empty_err" "$val"
    ((ret+=$?))

    # Mount
    AUTOTEST_MSG='{"Name": "autotest1"}'
    val=$(${CURL} $socket -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${base_url}Mount)
    expected='{"Err":"","Mountpoint":"/var/volume/test-vg/autotest1"}'
    compare "Mount" "$expected" $val
    ((ret+=$?))

    # Mount again (error)
    AUTOTEST_MSG='{"Name": "autotest1"}'
    val=$(${CURL} $socket -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${base_url}Mount)
    expected='{"Err":"Cannot mount device /dev/test-vg/autotest1: mount: /dev/mapper/test--vg-autotest1 already mounted or /var/volume/test-vg/autotest1 busy\nmount: according to mtab, /dev/mapper/test--vg-autotest1 is already mounted on /var/volume/test-vg/autotest1\n"}'
    compare "Mount" "$expected" "$val"
    ((ret+=$?))

    # Remove (fail)
    AUTOTEST_MSG='{"Name": "autotest1"}'
    val=$(${CURL} $socket -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${base_url}Remove)
    expected='{"Err":"Volume autotest1 is still mounted"}'
    compare "Remove" "$expected" "$val"
    ((ret+=$?))

    # Unmount
    AUTOTEST_MSG='{"Name": "autotest1"}'
    val=$(${CURL} $socket -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${base_url}Unmount)
    compare "Unmount" $empty_err "$val"
    ((ret+=$?))

    # Remove
    AUTOTEST_MSG='{"Name": "autotest1"}'
    val=$(${CURL} $socket -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${base_url}Remove)
    compare "Remove" $empty_err "$val"
    ((ret+=$?))

    # List
    val=$(${CURL} $socket -s --header "$HEADERS" ${URL_PREFIX}List)
    compare "List" "{\"Err\":\"\",\"Volumes\":[]}" "$val"
    ((ret+=$?))

    # Create will null opts
    AUTOTEST_MSG='{"Name": "autotest1", "Opts": null}'
    val=$(${CURL} $socket -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${base_url}Create)
    compare "Create with null opts" $empty_err "$val"
    ((ret+=$?))

    # Remove
    AUTOTEST_MSG='{"Name": "autotest1"}'
    val=$(${CURL} $socket -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${base_url}Remove)
    compare "Remove" $empty_err "$val"
    ((ret+=$?))

    return $ret
}


run_dangling_mount() {

    local failed=0

    echo "Dangling mount test"
    echo

    ../src/lvmvd --listener=http --default-size=1024 --volume-group-name=test-vg --mount-root=/var/volume/test-vg  >>${LVMVD_LOG} 2>&1 &
    lvmvdpid=$!
    sleep 2

    # Activate
    val=$(${CURL} -s --header "$HEADERS" ${URL_PREFIX1})
    expected='{"Implements":["VolumeDriver"]}'
    compare "Activate" "$expected" "$val"
    ((failed+=$?))

    # Create
    AUTOTEST_MSG='{"Name": "autotest-dangling1"}'
    val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Create)
    compare "Create" '{"Err":""}' "$val"
    ((failed+=$?))

    # mount
    val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Mount)
    expected='{"Err":"","Mountpoint":"/var/volume/test-vg/autotest-dangling1"}'
    compare "Mount" "$expected" "$val"
    ((failed+=$?))

    # Create
    AUTOTEST_MSG='{"Name": "autotest-dangling2"}'
    val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Create)
    compare "Create" '{"Err":""}' "$val"
    ((failed+=$?))

    # mount
    val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Mount)
    expected='{"Err":"","Mountpoint":"/var/volume/test-vg/autotest-dangling2"}'
    compare "Mount" "$expected" "$val"
    ((failed+=$?))

    # exit daemon with hanging mount points

    kill -15 $lvmvdpid
    sleep 1

    # 2 volumes hanging
    volumes=$(mount | grep "/var/volume/test-vg" | awk '{print $1}')
    for i in $volumes; do
        echo $i
        echo "Unmounting $i"
        umount $i
    done

    # properly delete them (after they have been unmounted above)

    ../src/lvmvd --listener=http --default-size=1024 --volume-group-name=test-vg --mount-root=/var/volume/test-vg  >>${LVMVD_LOG} 2>&1 &
    lvmvdpid=$!
    sleep 2

    # Activate
    val=$(${CURL} -s --header "$HEADERS" ${URL_PREFIX1})
    expected='{"Implements":["VolumeDriver"]}'
    compare "Activate" "$expected" "$val"
    ((failed+=$?))

    # Remove
    AUTOTEST_MSG='{"Name": "autotest-dangling1"}'
    val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Remove)
    expected='{"Err":""}'
    compare "Remove" "$expected" "$val"
    ((failed+=$?))

    # Remove
    AUTOTEST_MSG='{"Name": "autotest-dangling2"}'
    val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Remove)
    expected='{"Err":""}'
    compare "Remove" "$expected" "$val"
    ((failed+=$?))

    kill -15 $lvmvdpid
    sleep 1

    return $failed
}


# ---------------------------------------------------------------------------
# test preparations
# ---------------------------------------------------------------------------

create_volume

echo "Note: lvmvd logs are appended to ${LVMVD_LOG}"

# ---------------------------------------------------------------------------
# test some options
# ---------------------------------------------------------------------------

failed=0

../src/lvmvd --default-size=1024 --volume-group-name=test-vg --mount-root=/var/volume/test-vg  >>${LVMVD_LOG} 2>&1 &
lvmvdpid=$!
sleep 2

# test default location of socket file

if [ ! -S "/run/docker/plugins/lvm-volume-driver.sock" ]; then
    ((failed+=1))
    echo "FAIL: socket file not in expected location"
fi

kill -15 $lvmvdpid
sleep 1

# make sure it is gone

if [ -S "/run/docker/plugins/lvm-volume-driver.sock" ]; then
    ((failed+=1))
    echo "FAIL: socket file still exits after daemon stop"
fi


../src/lvmvd --default-size=1024 --volume-group-name=test-vg --mount-root=/var/volume/test-vg --listener=http --json-file=/tmp/lvm-volume-driver.json >>${LVMVD_LOG} 2>&1 &
lvmvdpid=$!
sleep 2

# test default location of json file

if [ ! -f "/tmp/lvm-volume-driver.json" ]; then
    ((failed+=1))
    echo "FAIL: json file not in expected location"
fi

kill -15 $lvmvdpid
sleep 1

# make sure its gone
if [ -f "/tmp/lvm-volume-driver.json" ]; then
    ((failed+=1))
    echo "FAIL: json file not in expected location"
fi

../src/lvmvd --debug --default-size=1024 --volume-group-name=test-vg --mount-root=/var/volume/test-vg --listener=http >>${LVMVD_LOG} 2>&1 &
lvmvdpid=$!
sleep 2

if [ ! -f "/etc/docker/plugins/lvm-volume-driver.json" ]; then
    ((failed+=1))
    echo "FAIL: json file not in expected location"
fi


# ---------------------------------------------------------------------------
# test with http listener
# ---------------------------------------------------------------------------


run_tests "" ${URL_PREFIX1} ${URL_PREFIX}
((failed+=$?))

kill -15 $lvmvdpid
sleep 1

# ---------------------------------------------------------------------------
# test with socket listener
# ---------------------------------------------------------------------------

if [ $TEST_SOCKET_LISTENER -eq 1 ]; then 
    ../src/lvmvd --default-size=1024 --volume-group-name=test-vg --mount-root=/var/volume/test-vg  >>${LVMVD_LOG} 2>&1 &
    lvmvdpid=$!
    sleep 2

    run_tests "--unix-socket /run/docker/plugins/lvm-volume-driver.sock" ${SURL_PREFIX1} ${SURL_PREFIX}
    ((failed+=$?))

    kill -15 $lvmvdpid
    sleep 1
else 
    echo "WARNING: Not testing daemon in unix socket mode"
fi

# ---------------------------------------------------------------------------
# test volume size options
# ---------------------------------------------------------------------------

echo "Test volume size options"
echo

../src/lvmvd --default-size=324 --volume-group-name=test-vg --mount-root=/var/volume/test-vg --listener=http >>${LVMVD_LOG} 2>&1 &
lvmvdpid=$!
sleep 2

# Activate
val=$(${CURL} -s --header "$HEADERS" ${URL_PREFIX1})
expected='{"Implements":["VolumeDriver"]}'
compare "Activate" "$expected" "$val"
((failed+=$?))

# Create
AUTOTEST_MSG='{"Name": "autotest-size1"}'
val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Create)
compare "Create" '{"Err":""}' $val
((failed+=$?))

size=$(blockdev --getsize64 /dev/test-vg/autotest-size1)
expected_size=339738624
if [ $size -ne $expected_size ]; then
    echo "Expected size $expected_size but actual is $size"
    ((failed+=1))
fi

# Remove
val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Remove)
compare "Remove" $empty_err "$val"
((failed+=$?))

# --------------

# Create
AUTOTEST_MSG='{"Name": "autotest-size2-oS200M"}'
val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Create)
compare "Create" '{"Err":""}' $val
((failed+=$?))

size=$(blockdev --getsize64 /dev/test-vg/autotest-size2-oS200M)
expected_size=209715200
if [ $size -ne $expected_size ]; then
    echo "Expected size $expected_size but actual is $size"
    ((failed+=1))
fi

# Remove
val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Remove)
compare "Remove" $empty_err "$val"
((failed+=$?))

# --------------
# defaults == megabytes

# Create
AUTOTEST_MSG='{"Name": "autotest-size3-oS178M"}'
val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Create)
compare "Create" '{"Err":""}' $val
((failed+=$?))

size=$(blockdev --getsize64 /dev/test-vg/autotest-size3-oS178M)
expected_size=186646528 # 178*1024*1024
expecte2=188743680 # 180mb
if [ $size -ne $expected_size ]; then
    if [ $size -eq $expecte2 ]; then
        echo "myterious, asked for 178MB but got 180MB"
    else
        echo "Expected size $expected_size but actual is $size"
        ((failed+=$?))
    fi
fi

# Remove
val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Remove)
compare "Remove" $empty_err "$val"
((failed+=$?))

# --------------

# Create
AUTOTEST_MSG='{"Name": "autotest-size2-oS1G"}'
val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Create)
compare "Create" '{"Err":""}' $val
((failed+=$?))

size=$(blockdev --getsize64 /dev/test-vg/autotest-size2-oS1G)
expected_size=1073741824
if [ $size -ne $expected_size ]; then
    echo "Expected size $expected_size but actual is $size"
    ((failed+=1))
fi

# Remove
val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Remove)
compare "Remove" $empty_err "$val"
((failed+=$?))

# --------------
# big

# Create
AUTOTEST_MSG='{"Name": "autotest-size5-oS10G"}'
val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Create)
compare "Create" '{"Err":""}' $val
((failed+=$?))

size=$(blockdev --getsize64 /dev/test-vg/autotest-size5-oS10G)
expected_size=10737418240
if [ $size -ne $expected_size ]; then
    echo "Expected size $expected_size but actual is $size"
    ((failed+=1))
fi

# Remove
val=$(${CURL} -s -d "$AUTOTEST_MSG" --header "$HEADERS" ${URL_PREFIX}Remove)
compare "Remove" $empty_err "$val"
((failed+=$?))


kill -15 $lvmvdpid
sleep 1


# ---------------------------------------------------------------------------
# test dangling mount
# ---------------------------------------------------------------------------

run_dangling_mount
((failed+=$?))

# ---------------------------------------------------------------------------
# Wrap up
# ---------------------------------------------------------------------------

if [ $failed -eq 0 ]; then
    echo "All tests ok, removing test volume"
    delete_volume
else
    echo "Number of failed tests: $failed"
fi
