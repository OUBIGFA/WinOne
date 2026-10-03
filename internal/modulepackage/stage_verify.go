package modulepackage

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"winone/internal/configjson"
	"winone/internal/distribution"
)

var stageIDPattern = regexp.MustCompile(`^stage-[a-f0-9]{32}$`)

// VerifyStage is read-only. A receipt is checked against the approved signature,
// but never substitutes for inspecting every staged file again.
func VerifyStage(ctx context.Context, id string, options StageOptions) (*StagedPackage, error) {
	if err := validateStageOptions(options); err != nil {
		return nil, err
	}
	if !stageIDPattern.MatchString(id) {
		return nil, errors.New("invalid stage identity")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	base, err := os.OpenRoot(options.Directory)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	info, err := base.Lstat(id)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("stage is not a regular directory")
	}
	root, err := base.OpenRoot(id)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, err := readStageFile(ctx, root, "module.json", MaxManifestBytes)
	if err != nil {
		return nil, err
	}
	signature, err := readStageFile(ctx, root, "module.json.minisig", MaxSignatureBytes)
	if err != nil {
		return nil, err
	}
	manifest, err := VerifyManifest(data, signature, options.Publisher)
	if err != nil {
		return nil, err
	}
	if err = verifyStageApproval(manifest, options); err != nil {
		return nil, err
	}
	receiptBytes, err := readStageFile(ctx, root, StageReceiptName, 16<<10)
	if err != nil {
		return nil, err
	}
	var receipt StageReceipt
	if err = configjson.DecodeObject(receiptBytes, &receipt); err != nil {
		return nil, err
	}
	if receipt.SchemaVersion != 1 || receipt.StageID != id || receipt.ModuleID != manifest.metadata.Module.ID || receipt.Version != manifest.metadata.Version || receipt.ManifestSHA256 != manifest.Digest() {
		return nil, errors.New("stage receipt does not match approved package")
	}
	if err = verifyStageTree(ctx, root, manifest.files); err != nil {
		return nil, err
	}
	buffer := make([]byte, 64<<10)
	for _, record := range manifest.files {
		if err = verifyStagePayload(ctx, root, record, buffer); err != nil {
			return nil, err
		}
	}
	return &StagedPackage{directory: filepath.Join(options.Directory, id), manifest: manifest, ready: true}, nil
}

func readStageFile(ctx context.Context, root *os.Root, name string, limit int64) (data []byte, err error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("stage control file is not regular")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	info, err = file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > limit {
		return nil, errors.New("stage control file is invalid or too large")
	}
	data, err = io.ReadAll(io.LimitReader(cancellableReader{ctx, file}, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != info.Size() || int64(len(data)) > limit {
		return nil, errors.New("stage control file changed while reading")
	}
	return data, nil
}

func verifyStagePayload(ctx context.Context, root *os.Root, record distribution.File, buffer []byte) (err error) {
	info, err := root.Lstat(record.Path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("staged payload is not regular")
	}
	file, err := root.Open(record.Path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	info, err = file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != record.Size {
		return errors.New("staged payload size mismatch")
	}
	return transferPayload(ctx, file, record, nil, buffer)
}

func verifyStageTree(ctx context.Context, root *os.Root, records []distribution.File) error {
	allowed := map[string]bool{"module.json": true, "module.json.minisig": true, StageReceiptName: true}
	directories := map[string]bool{".": true}
	for _, record := range records {
		allowed[strings.ToLower(record.Path)] = true
		for parent := path.Dir(strings.ToLower(record.Path)); parent != "."; parent = path.Dir(parent) {
			directories[parent] = true
		}
	}
	stack := []string{"."}
	seen := map[string]bool{}
	files := 0
	for len(stack) > 0 {
		directory := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		// Read bounded batches rather than allowing WalkDir to materialize an
		// arbitrarily large tampered directory before the first membership check.
		err := func() (err error) {
			folder, err := root.Open(directory)
			if err != nil {
				return err
			}
			defer func() { err = errors.Join(err, folder.Close()) }()
			for {
				if err = ctx.Err(); err != nil {
					return err
				}
				entries, readErr := folder.ReadDir(64)
				for _, entry := range entries {
					name := path.Join(directory, entry.Name())
					key := strings.ToLower(name)
					if !distribution.ValidPayloadPath(name) || seen[key] {
						return errors.New("invalid or duplicate staged path")
					}
					seen[key] = true
					info, err := root.Lstat(name)
					if err != nil {
						return err
					}
					if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
						if !directories[key] {
							return errors.New("unlisted staged directory")
						}
						stack = append(stack, name)
					} else {
						if !info.Mode().IsRegular() || !allowed[key] {
							return errors.New("unlisted or linked staged file")
						}
						files++
					}
				}
				if readErr == io.EOF {
					return nil
				}
				if readErr != nil {
					return readErr
				}
			}
		}()
		if err != nil {
			return err
		}
	}
	if files != len(allowed) {
		return errors.New("staged file inventory is incomplete")
	}
	return nil
}
