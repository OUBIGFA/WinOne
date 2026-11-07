package distribution

import "testing"

func TestLearningPackageKeepsDatabaseManifestAndLicenseTogether(t *testing.T) {
	for _, file := range []string{"shared/ui/learning/ecdict.db", "shared/ui/learning/manifest.json", "shared/ui/learning/ECDICT-LICENSE.txt"} {
		if !allowed(file) {
			t.Fatalf("learning asset not allowed: %s", file)
		}
		required := requiredForPayload(map[string]bool{file: true})
		for _, want := range []string{"shared/ui/learning/ecdict.db", "shared/ui/learning/manifest.json", "shared/ui/learning/ECDICT-LICENSE.txt"} {
			found := false
			for _, got := range required {
				if want == got {
					found = true
				}
			}
			if !found {
				t.Fatalf("partial learning payload allowed: %s missing %s", file, want)
			}
		}
	}
}
