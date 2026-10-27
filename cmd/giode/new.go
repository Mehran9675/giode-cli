package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mehran9675/giode-cli/internal/config"
	"github.com/mehran9675/giode-cli/internal/scaffold"
)

const newUsage = `
Usage: giode new [options] <directory>

Creates a giode project in directory: a counter app in main.go, go.mod
with the latest giode, giode.toml with the settings to fill in, a
placeholder icon and assets/README.md, .gitignore and a README.

Example:

	giode new -id com.acme.notes notes
	cd notes && giode run
`

func cmdNew(args []string) error {
	fs := flags("new", newUsage)
	var (
		name     = fs.String("name", "", "display name (default: from the directory name)")
		module   = fs.String("module", "", "Go module path (default: the directory name)")
		appID    = fs.String("id", "", "reverse-DNS app ID (default: com.example.<name>)")
		version  = fs.String("giode", "latest", "giode version to require")
		offline  = fs.Bool("offline", false, "don't fetch giode; run go mod tidy yourself later")
		noGitDir = fs.Bool("no-git", false, "don't run git init")
	)
	fs.BoolVar(&verbose, "x", false, "print the commands run")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		fs.Usage()
		return errors.New("give one directory")
	}
	dir := pos[0]
	defName, defModule, defID := scaffold.Defaults(dir)
	p := scaffold.Project{Dir: dir, Name: defName, Module: defModule, AppID: defID}
	if *name != "" {
		p.Name = *name
	}
	if *module != "" {
		p.Module = *module
	}
	if *appID != "" {
		p.AppID = *appID
	}
	if err := scaffold.Create(p); err != nil {
		return err
	}
	fmt.Printf("Created %s\n", dir)

	if err := run(command(nil, "go", "-C", dir, "mod", "init", p.Module)); err != nil {
		return err
	}
	if !*offline {
		fmt.Println("Fetching github.com/mehran9675/giode@" + *version + "…")
		if err := run(command(nil, "go", "-C", dir, "get", "github.com/mehran9675/giode@"+*version)); err != nil {
			return err
		}
		if err := run(command(nil, "go", "-C", dir, "mod", "tidy")); err != nil {
			return err
		}
	}
	if !*noGitDir {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
			// Not fatal: git may be missing.
			if err := run(command(nil, "git", "-C", dir, "init", "-q")); err != nil {
				fmt.Fprintln(os.Stderr, "giode new: skipped git init:", err)
			}
		}
	}
	fmt.Printf("\nNext:\n\n\tcd %s\n\tgiode run            # on this computer\n\tgiode run android    # on a connected Android device\n\nApp settings are in %s; replace the placeholders in assets/ before publishing.\n", dir, config.FileName)
	return nil
}

const initUsage = `
Usage: giode init [options]

Adds giode.toml to the Go module in the current directory, so giode
build and giode run can package it.
`

func cmdInit(args []string) error {
	fs := flags("init", initUsage)
	var (
		name  = fs.String("name", "", "display name (default: from the directory name)")
		appID = fs.String("id", "", "reverse-DNS app ID (default: com.example.<name>)")
		pkg   = fs.String("package", ".", "main package")
	)
	if _, err := parse(fs, args); err != nil {
		return err
	}
	if _, err := os.Stat(config.FileName); err == nil {
		return fmt.Errorf("%s already exists", config.FileName)
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	defName, _, defID := scaffold.Defaults(wd)
	if *name == "" {
		*name = defName
	}
	if *appID == "" {
		*appID = defID
	}
	icon := ""
	for _, c := range []string{"appicon.png", "assets/icon.png", "icon.png"} {
		if _, err := os.Stat(filepath.Join(*pkg, c)); err == nil {
			icon = filepath.ToSlash(filepath.Join(*pkg, c))
			break
		}
	}
	s := fmt.Sprintf(`# giode.toml configures how the giode CLI builds this app.
# Reference: https://github.com/mehran9675/giode-cli#giodetoml

[app]
name = %q
id = %q
version = "0.1.0"
build = 1
package = %q
`, *name, *appID, *pkg)
	if icon != "" {
		s += fmt.Sprintf("icon = %q\n", icon)
	}
	if err := os.WriteFile(config.FileName, []byte(s), 0o644); err != nil {
		return err
	}
	fmt.Printf("Wrote %s for %s (%s). Edit it to set the icon, version and permissions.\n", config.FileName, *name, *appID)
	return nil
}
