#!/bin/sh
echo "Starting redis..."
/usr/bin/redis-server /etc/redis.conf --dir /var/lib/redis &
sleep 1
echo "Starting service daemon..."
/usr/sbin/geoip serve -c docker
