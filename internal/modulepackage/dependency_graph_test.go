package modulepackage

import (
	"aead.dev/minisign"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func graphSigner(t *testing.T) func(string, []Dependency, ...func(*Metadata)) *VerifiedManifest {
	t.Helper()
	base, publisher, key := signedTestManifest(t)
	return func(id string, dependencies []Dependency, changes ...func(*Metadata)) *VerifiedManifest {
		t.Helper()
		m := base
		m.Format = "winone.runtime.v1"
		m.Metadata = runtimeMetadataFixture()
		m.Metadata.Runtime.ID = id
		m.Metadata.Dependencies = dependencies
		for _, change := range changes {
			change(&m.Metadata)
		}
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		verified, err := VerifyManifest(raw, minisign.Sign(key, raw), publisher)
		if err != nil {
			t.Fatal(err)
		}
		return verified
	}
}
func graphReference(value *VerifiedManifest) Dependency {
	m := value.Metadata()
	return Dependency{Kind: m.PackageKind(), ID: m.PackageID(), Version: m.Version, ManifestSHA256: value.Digest()}
}

func TestDependencyGraphLocksAndDeduplicatesSignedDiamond(t *testing.T) {
	make := graphSigner(t)
	base := make("example.base", nil)
	left := make("example.left", []Dependency{graphReference(base)})
	right := make("example.right", []Dependency{graphReference(base)})
	root := make("example.root", []Dependency{graphReference(right), graphReference(left)})
	items := map[string]*VerifiedManifest{"example.base": base, "example.left": left, "example.right": right}
	calls := map[string]int{}
	lock, err := ResolveDependencies(context.Background(), root, testEnvironment(), func(_ context.Context, want Dependency) (*VerifiedManifest, error) {
		calls[want.ID]++
		return items[want.ID], nil
	})
	if err != nil || len(lock.Packages) != 3 || lock.Root != graphReference(root) {
		t.Fatal(lock, err)
	}
	for id, count := range calls {
		if count != 1 {
			t.Fatal("shared node fetched repeatedly", id, count)
		}
	}
	first, err := lock.Digest()
	if err != nil {
		t.Fatal(err)
	}
	lock.Packages[0], lock.Packages[2] = lock.Packages[2], lock.Packages[0]
	second, err := lock.Digest()
	if err != nil || second != first {
		t.Fatal("lock hash depends on enumeration order", err)
	}
	lock.Packages[0].ManifestSHA256 = strings.Repeat("f", 64)
	if changed, err := lock.Digest(); err != nil || changed == first {
		t.Fatal("lock does not bind dependency content", err)
	}
}

func TestDependencyGraphRejectsMissingChangedAndIncompatibleContent(t *testing.T) {
	for _, mode := range []string{"missing", "digest", "kind", "platform", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			make := graphSigner(t)
			leaf := make("example.leaf", nil)
			if mode == "platform" {
				leaf = make("example.leaf", nil, func(m *Metadata) { m.Compatibility.Platform = "windows-arm64" })
			}
			wanted := graphReference(leaf)
			if mode == "digest" {
				wanted.ManifestSHA256 = strings.Repeat("a", 64)
			}
			if mode == "kind" {
				wanted.Kind = "module"
			}
			root := make("example.root", []Dependency{wanted})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			_, err := ResolveDependencies(ctx, root, testEnvironment(), func(context.Context, Dependency) (*VerifiedManifest, error) {
				if mode == "missing" {
					return nil, errors.New("not installed")
				}
				return leaf, nil
			})
			if err == nil {
				t.Fatal("invalid graph was locked")
			}
			if mode == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestDependencyGraphRejectsTransitiveVersionConflictsAndCycles(t *testing.T) {
	make := graphSigner(t)
	first := make("example.base", nil)
	second := make("example.base", nil, func(m *Metadata) { m.Version = "1.3.0" })
	left := make("example.left", []Dependency{graphReference(first)})
	right := make("example.right", []Dependency{graphReference(second)})
	root := make("example.root", []Dependency{graphReference(left), graphReference(right)})
	items := map[string]*VerifiedManifest{left.Digest(): left, right.Digest(): right, first.Digest(): first, second.Digest(): second}
	if _, err := ResolveDependencies(context.Background(), root, testEnvironment(), func(_ context.Context, d Dependency) (*VerifiedManifest, error) { return items[d.ManifestSHA256], nil }); err == nil {
		t.Fatal("conflicting shared versions entered one graph")
	}
	// Synthetic nodes exercise the cycle guard independently of signature tests.
	a := &VerifiedManifest{metadata: runtimeMetadataFixture(), digest: strings.Repeat("a", 64)}
	a.metadata.Runtime.ID = "example.a"
	b := &VerifiedManifest{metadata: runtimeMetadataFixture(), digest: strings.Repeat("b", 64)}
	b.metadata.Runtime.ID = "example.b"
	a.metadata.Dependencies = []Dependency{graphReference(b)}
	b.metadata.Dependencies = []Dependency{graphReference(a)}
	if _, err := ResolveDependencies(context.Background(), a, testEnvironment(), func(context.Context, Dependency) (*VerifiedManifest, error) { return b, nil }); err == nil {
		t.Fatal("cycle accepted")
	}
}

func TestDependencyGraphBoundsTransitiveWork(t *testing.T) {
	make := graphSigner(t)
	items := map[string]*VerifiedManifest{}
	var next *VerifiedManifest
	for i := MaxDependencyDepth + 1; i >= 0; i-- {
		var refs []Dependency
		if next != nil {
			refs = []Dependency{graphReference(next)}
		}
		next = make(fmt.Sprintf("example.depth-%d", i), refs)
		items[next.Digest()] = next
	}
	if _, err := ResolveDependencies(context.Background(), next, testEnvironment(), func(_ context.Context, d Dependency) (*VerifiedManifest, error) { return items[d.ManifestSHA256], nil }); err == nil {
		t.Fatal("unbounded dependency depth")
	}
}
