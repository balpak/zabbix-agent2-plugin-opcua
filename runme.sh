#!/usr/bin/env bash

# Get plugin name
BUILDNAME=$(basename $(pwd))
echo "Building $BUILDNAME"

# Creating folder for binary if running script for the first time after 'git clone'
[ ! -d "build" ] && mkdir build && echo "New folder 'build' created"

# Clean-up
[ -f go.mod ] && rm go.mod && echo "Old 'go.mod' deleted"
[ -f go.sum ] && rm go.sum && echo "Old 'go.sum' deleted"
[ -f build/"$BUILDNAME" ] && rm build/"$BUILDNAME" && echo "Old $BUILDNAME binary deleted"
[ -f build/"$BUILDNAME.sha256" ] && rm build/"$BUILDNAME.sha256" && echo "Old $BUILDNAME checksum deleted"

cp template.go.mod go.mod && echo "Original 'go.mod' restored from template"

echo -e "\n"

# Check latest MASTER commit here
# https://git.zabbix.com/projects/AP/repos/plugin-support/commits


#go get golang.zabbix.com/sdk@f7caedd9e13ed	# 7.5 latest from 04 Sep 2026 with Go version 1.25.12
go get golang.zabbix.com/sdk@f70f12fff03	# 7.5 from 18 Aug 2026

echo -e "\n"

go get github.com/gopcua/opcua@latest

go mod tidy

echo -e "\n"

# statically linked
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o build/"$BUILDNAME"

[ -f build/"$BUILDNAME" ] && sha256sum build/"$BUILDNAME" > build/"$BUILDNAME.sha256"

echo -e "\n"

sha256sum -c build/"$BUILDNAME.sha256"

echo -e "\n"

###### tree -a -I '.git'	# Un-comment if you want to see all fils after build

# EOF
