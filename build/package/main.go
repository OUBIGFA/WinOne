package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"winone/internal/distribution"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	bundle := flag.String("bundle", "", "verified build directory")
	license := flag.String("license", "", "project license")
	output := flag.String("output", "", "output directory")
	version := flag.String("version", "", "package version")
	verify := flag.String("verify", "", "verify an existing package")
	installerArtifacts := flag.String("verify-installer-artifacts", "", "verify setup and uninstaller record against package")
	allowTestArtifacts := flag.Bool("allow-test-installer-artifacts", false, "explicitly allow isolated test installer artifacts")
	record := flag.Bool("record-build", false, "record a build after its checks pass")
	nsis := flag.String("nsis-files", "", "write installer file macros after package verification")
	upgradeFrom := flag.String("upgrade-from", "", "compare a verified older ZIP with --verify without installing")
	installed := flag.String("installed", "", "also verify the installed directory against the reference package")
	backup := flag.String("backup", "", "copy a verified installed payload to a new backup directory")
	verifyBackup := flag.String("verify-backup", "", "verify backup payload, completion record and installer files")
	registrationBackup := flag.String("backup-registration", "", "write registration metadata into a verified backup")
	registrationKey := flag.String("registration-key", `Software\Microsoft\Windows\CurrentVersion\Uninstall\WinOne`, "current-user registration key to capture")
	shortcutBackup := flag.String("backup-shortcut", "", "write a shortcut snapshot into a verified backup")
	shortcutFile := flag.String("shortcut-file", "", "exact .lnk file to snapshot")
	flag.Parse()
	if *installerArtifacts != "" || *allowTestArtifacts {
		valid := *installerArtifacts != "" && *verify != "" && flag.NArg() == 0
		flag.Visit(func(f *flag.Flag) {
			if f.Name != "verify" && f.Name != "verify-installer-artifacts" && f.Name != "allow-test-installer-artifacts" {
				valid = false
			}
		})
		if !valid {
			return fmt.Errorf("verify-installer-artifacts requires verify without other operations")
		}
		result, err := distribution.VerifyInstallerArtifacts(*installerArtifacts, *verify, *allowTestArtifacts)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	if *shortcutFile != "" && *shortcutBackup == "" {
		return fmt.Errorf("shortcut-file requires backup-shortcut")
	}
	if *shortcutBackup != "" {
		if *verify == "" || *shortcutFile == "" || *installed != "" || *backup != "" || *verifyBackup != "" || *registrationBackup != "" || *upgradeFrom != "" || *nsis != "" || *record {
			return fmt.Errorf("backup-shortcut requires verify and shortcut-file without other operations")
		}
		if err := distribution.BackupShortcut(*verify, *shortcutBackup, *shortcutFile); err != nil {
			return err
		}
		fmt.Println("Shortcut snapshot saved")
		return nil
	}
	if *registrationBackup != "" {
		if *verify == "" || *installed == "" || *backup != "" || *verifyBackup != "" || *upgradeFrom != "" || *nsis != "" || *record {
			return fmt.Errorf("backup-registration requires installed and verify without other operations")
		}
		if err := backupRegistration(*installed, *verify, *registrationBackup, *registrationKey); err != nil {
			return err
		}
		fmt.Println("Registration snapshot saved")
		return nil
	}
	if *verifyBackup != "" {
		if *verify == "" || *backup != "" || *installed != "" || *upgradeFrom != "" || *nsis != "" || *record {
			return fmt.Errorf("verify-backup requires verify without other operations")
		}
		receipt, err := distribution.VerifyBackup(*verifyBackup, *verify)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(receipt)
	}
	if *backup != "" {
		if *installed == "" || *verify == "" || *upgradeFrom != "" || *nsis != "" || *record {
			return fmt.Errorf("backup requires installed and verify, without other operations")
		}
		receipt, err := distribution.BackupInstalled(*installed, *verify, *backup)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(receipt)
	}
	if *installed != "" && *verify == "" {
		return fmt.Errorf("installed requires a reference package with verify")
	}
	if *upgradeFrom != "" {
		if *verify == "" || *nsis != "" || *record {
			return fmt.Errorf("upgrade-from requires verify and cannot generate installers or build records")
		}
		plan, err := distribution.PlanUpgrade(*upgradeFrom, *verify)
		if err != nil {
			return err
		}
		if *installed != "" {
			if _, err := distribution.VerifyInstalled(*installed, *upgradeFrom); err != nil {
				return err
			}
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(plan)
	}
	if *nsis != "" && *verify == "" {
		return fmt.Errorf("nsis-files requires package verification")
	}
	if *verify != "" {
		var manifest distribution.Manifest
		var err error
		if *installed != "" {
			manifest, err = distribution.VerifyInstalled(*installed, *verify)
		} else {
			manifest, err = distribution.Verify(*verify)
		}
		if err != nil {
			return err
		}
		if *nsis != "" {
			text, err := distribution.NSISFileMacros(manifest)
			if err != nil {
				return err
			}
			if err = exclusiveWrite(*nsis, []byte(text)); err != nil {
				return err
			}
		}
		fmt.Printf("Verified %s: %d files\n", manifest.Version, len(manifest.Files))
		return nil
	}
	if *record {
		return distribution.RecordBuild(*bundle, *license, *version)
	}
	info, err := distribution.ReadBuildInfo(*bundle)
	if err != nil {
		return err
	}
	if *version != "" && *version != info.Version {
		return fmt.Errorf("requested version does not match build")
	}
	*version = info.Version
	name, err := distribution.PackageName(*version)
	if err != nil {
		return err
	}
	if *bundle == "" || *license == "" || *output == "" {
		return fmt.Errorf("bundle, license and output are required")
	}
	destination := filepath.Join(*output, name)
	for _, path := range []string{destination, destination + ".sha256", destination + ".manifest.json"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return fmt.Errorf("output already exists or cannot be checked: %s", path)
		}
	}
	if err = distribution.Build(*bundle, *license, destination, *version); err != nil {
		return err
	}
	manifest, err := distribution.Verify(destination)
	if err != nil {
		return err
	}
	file, err := os.Open(destination)
	if err != nil {
		return err
	}
	digest := sha256.New()
	_, err = io.Copy(digest, file)
	file.Close()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err = exclusiveWrite(destination+".manifest.json", data); err != nil {
		return err
	}
	if err = exclusiveWrite(destination+".sha256", []byte(hex.EncodeToString(digest.Sum(nil))+"  "+name+"\n")); err != nil {
		return err
	}
	fmt.Printf("Verified package: %s (%d files)\n", destination, len(manifest.Files))
	return nil
}
func exclusiveWrite(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
