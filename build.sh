#!/bin/sh

export VERSION_NUMBER=`cat version`
echo Getting dependencies...
go mod download
echo Building version $VERSION_NUMBER...
go build -ldflags "-X main.version=$VERSION_NUMBER" -o bin/lmdownload -i .
echo Done
