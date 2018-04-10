#!/bin/bash

export VERSION_NUMBER=`cat version`
echo Building version $VERSION_NUMBER...
go build -ldflags "-X main.version=$VERSION_NUMBER" -o bin/lmdownload -i .
echo Done
