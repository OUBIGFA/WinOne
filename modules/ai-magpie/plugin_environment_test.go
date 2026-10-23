package plugin

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestWinOnePluginCommandKeepsWindowsIdentityAndPrivateData(t *testing.T) {
 root := t.TempDir()
 private := filepath.Join(root, "private")
 system := filepath.Join(root, "windows-user")
 t.Setenv("USERPROFILE", private)
 t.Setenv("WINONE_AI_OS_USERPROFILE", system)
 t.Setenv("XDG_CONFIG_HOME", filepath.Join(private, "config"))
 t.Setenv("LOCALAPPDATA", filepath.Join(private, "local"))
 t.Setenv("APPDATA", filepath.Join(private, "roaming"))
 cmd := bunCommand(t.Context(), "unused-bun.exe", root)
 values := map[string]string{}
 for _, value := range cmd.Environ() {
  name, data, _ := strings.Cut(value, "=")
  values[strings.ToUpper(name)] = data
 }
 if values["USERPROFILE"] != system { t.Fatal("plugin command lost the Windows user profile required by AppContainer") }
 for _, name := range []string{"XDG_CONFIG_HOME", "LOCALAPPDATA", "APPDATA"} {
  if values[name] != os.Getenv(name) { t.Fatalf("plugin command changed private %s", name) }
 }
 if os.Getenv("USERPROFILE") != private { t.Fatal("plugin command changed its parent's isolated account home") }
}
