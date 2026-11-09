package distribution

import "testing"

func TestAnkiPayloadIncludesEngineRendererAndOriginalNotices(t *testing.T){
	for _,file:=range []string{"shared/anki/worker.py","shared/anki/lib/anki/collection.py","shared/anki/renderer/js/reviewer.js","shared/anki/licenses/anki/AGPL-3.0.txt"}{
		if !allowed(file){t.Fatalf("bundled Anki file rejected: %s",file)}
		required:=requiredForPayload(map[string]bool{file:true})
		for _,want:=range ankiRuntimeFiles{found:=false;for _,got:=range required{found=found||got==want};if !found{t.Fatalf("partial Anki payload accepted: missing %s",want)}}
	}
}

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
