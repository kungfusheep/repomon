package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const plistLabel = "com.kungfusheep.repomon"

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", plistLabel+".plist")
}

func logPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "repomon", "repomon.log")
}

func installCron() {
	bin, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: can't resolve binary path: %s\n", err)
		os.Exit(1)
	}

	cfg, err := loadConfig()
	if err != nil || len(cfg.Repos) == 0 {
		fmt.Fprintf(os.Stderr, "error: no repos in config -- run --discover first\n")
		os.Exit(1)
	}

	plist := buildPlist(bin)
	path := plistPath()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating directory: %s\n", err)
		os.Exit(1)
	}

	_ = exec.Command("launchctl", "unload", path).Run()

	if err := os.WriteFile(path, []byte(plist), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing plist: %s\n", err)
		os.Exit(1)
	}

	if err := exec.Command("launchctl", "load", path).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error loading plist: %s\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "installed: %s\n", path)
	fmt.Fprintf(os.Stderr, "refreshes cache every 5 minutes, logs to %s\n", logPath())
}

func uninstallCron() {
	path := plistPath()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "not installed\n")
		return
	}

	if err := exec.Command("launchctl", "unload", path).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: unload failed: %s\n", err)
	}

	if err := os.Remove(path); err != nil {
		fmt.Fprintf(os.Stderr, "error removing plist: %s\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "uninstalled: %s\n", path)
}

func buildPlist(bin string) string {
	log := logPath()

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>--cache</string>
  </array>
  <key>StartInterval</key>
  <integer>300</integer>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, plistLabel, bin, log, log)
}
