package main

import (
	"embed"
	"os"
	"runtime"

	"github.com/ao-data/albiondata-client/client"
	"github.com/ao-data/albiondata-client/dashboard"
	"github.com/ao-data/albiondata-client/log"
	"github.com/ao-data/albiondata-client/systray"
)

//go:embed web/dist/*
var webFiles embed.FS

func init() {
	// Inject the embedded files into the dashboard package
	dashboard.SetStaticFiles(webFiles)
}

var version string

func init() {
	client.ConfigGlobal.SetupFlags()
}

func main() {
	if client.ConfigGlobal.PrintVersion {
		log.Infof("Albion Data Client, version: %s", version)
		return
	}

	// On macOS, the systray requires the Cocoa event loop to run on the main thread.
	// So we run the client in a goroutine and systray on the main thread.
	// On other platforms, we do the opposite for backward compatibility.
	if runtime.GOOS == "darwin" {
		go runClient()
		systray.Run() // This blocks on the main thread (required for macOS)
	} else {
		go systray.Run()
		runClient()
	}
}

func runClient() {
	// Start the market dashboard if enabled
	if client.ConfigGlobal.DashboardPort != "0" {
		craftingHub := dashboard.NewCraftingHub()
		client.CraftingNotify = craftingHub.Notify
		refiningHub := dashboard.NewRefiningHub()
		client.RefiningNotify = refiningHub.Notify
		go dashboard.Start(client.ConfigGlobal.DBPath, client.ConfigGlobal.DashboardPort, craftingHub, refiningHub)
	}

	c := client.NewClient(version)
	err := c.Run()
	if err != nil {
		log.Error(err)
		log.Error("The program encountered an error. Press any key to close this window.")
		var b = make([]byte, 1)
		_, _ = os.Stdin.Read(b)
	}
}

