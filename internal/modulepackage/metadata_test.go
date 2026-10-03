package modulepackage

import (
	"encoding/json"
	"strings"
	"testing"
	"winone/internal/catalog"
)

func testMetadata() Metadata {
	return Metadata{SchemaVersion: 1, Version: "1.2.0", Module: catalog.Manifest{ManifestVersion: 1, ID: "example.tool", Name: catalog.LocalizedName{Chinese: "测试工具", English: "Test tool"}, Distribution: "optional", Worker: "bin/worker.exe", Activation: "on-demand", SettingsSchemaVersion: 1}, Compatibility: Compatibility{MinimumHostVersion: "0.1.0", MaximumHostVersionExclusive: "2.0.0", ProtocolVersion: 1, DesignSystemVersion: 1, Platform: "windows-x64", MinimumWindowsBuild: 22000}}
}
func testEnvironment() Environment {
	return Environment{HostVersion: "1.0.0", ProtocolVersion: 1, DesignSystemVersion: 1, Platform: "windows-x64", WindowsBuild: 22631}
}

func TestMetadataRequiresAnExplicitOptionalModuleContract(t *testing.T) {
	good := testMetadata()
	data, err := json.Marshal(good)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := DecodeMetadata(data)
	if err != nil {
		t.Fatal(err)
	}
	report, err := parsed.CheckCompatibility(testEnvironment())
	if err != nil || !report.Compatible || len(report.Issues) != 0 {
		t.Fatal(report, err)
	}
	for _, change := range []func(*Metadata){
		func(m *Metadata) { m.SchemaVersion = 2 }, func(m *Metadata) { m.Version = "latest" }, func(m *Metadata) { m.Module.Distribution = "bundled" },
		func(m *Metadata) { m.Module.ID = "profiles.import" }, func(m *Metadata) { m.Module.Worker = "../worker.exe" },
		func(m *Metadata) { m.Compatibility.MinimumHostVersion = "" }, func(m *Metadata) { m.Compatibility.MaximumHostVersionExclusive = "0.1.0" },
		func(m *Metadata) { m.Compatibility.ProtocolVersion = 0 }, func(m *Metadata) { m.Compatibility.DesignSystemVersion = 0 },
		func(m *Metadata) { m.Compatibility.Platform = "linux-x64" }, func(m *Metadata) { m.Compatibility.MinimumWindowsBuild = 0 },
	} {
		value := testMetadata()
		change(&value)
		data, _ := json.Marshal(value)
		if _, err := DecodeMetadata(data); err == nil {
			t.Fatal("invalid package metadata accepted", string(data))
		}
	}
	boundary := append(append([]byte{}, data...), []byte(strings.Repeat(" ", MaxMetadataBytes-len(data)))...)
	if _, err := DecodeMetadata(boundary); err != nil {
		t.Fatal("valid boundary-size metadata rejected", err)
	}
	duplicate := []byte(strings.Replace(string(data), `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1))
	unknown := []byte(strings.Replace(string(data), `"schemaVersion":1`, `"schemaVersion":1,"unknown":true`, 1))
	for _, bad := range [][]byte{duplicate, unknown, append(append([]byte{}, data...), []byte(`{}`)...), append(boundary, ' '), []byte(`null`)} {
		if _, err := DecodeMetadata(bad); err == nil {
			t.Fatal("ambiguous or oversized metadata accepted")
		}
	}
}

func TestCompatibilityReportsAllIndependentRequirements(t *testing.T) {
	m := testMetadata()
	environment := testEnvironment()
	environment.HostVersion = "0.1.0-alpha.1"
	environment.ProtocolVersion = 2
	environment.DesignSystemVersion = 2
	environment.Platform = "windows-arm64"
	environment.WindowsBuild = 19041
	report, err := m.CheckCompatibility(environment)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, issue := range report.Issues {
		found[issue.Code] = true
	}
	for _, code := range []string{"host_too_old", "protocol", "design_system", "platform", "windows_build"} {
		if !found[code] {
			t.Fatal("missing compatibility reason", code, report)
		}
	}
	if report.Compatible {
		t.Fatal("incompatible module accepted")
	}
}

func TestHostVersionBoundsAreInclusiveMinimumAndExclusiveMaximum(t *testing.T) {
	for _, test := range []struct {
		version    string
		compatible bool
	}{{"0.1.0", true}, {"1.9.9", true}, {"2.0.0", false}, {"3.0.0", false}} {
		env := testEnvironment()
		env.HostVersion = test.version
		report, err := testMetadata().CheckCompatibility(env)
		if err != nil || report.Compatible != test.compatible {
			t.Fatal(test, report, err)
		}
	}
	env := testEnvironment()
	env.HostVersion = "unknown"
	if _, err := testMetadata().CheckCompatibility(env); err == nil {
		t.Fatal("unknown host version treated as compatible")
	}
}
