#!/bin/bash

pkill homebridge && echo "sent kill"
go build  -o homebridge ./hometest.go && ./homebridge