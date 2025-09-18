package main

import (
	"time"
)

const (
	DeviceTypeMac   = "mac"
	DeviceTypeWatch = "watch"
)

const (
	requestTimeout = 30 * time.Second
	cacheTTL       = 5 * time.Minute
	batteryTTL     = 30 * time.Second
	downloadsTTL   = 10 * time.Second
	addr           = ":8080"
	statusInterval = 5 * time.Second
)
