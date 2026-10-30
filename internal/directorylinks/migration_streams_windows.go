//go:build windows

package directorylinks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type migrationStream struct {
	Name string
	Size int64
}

var migrationFindFirstStream = windows.NewLazySystemDLL("kernel32.dll").NewProc("FindFirstStreamW")
var migrationFindNextStream = windows.NewLazySystemDLL("kernel32.dll").NewProc("FindNextStreamW")

// CopyFileEx copies NTFS alternate streams. Count and hash those streams too:
// comparing only the unnamed data stream could certify an incomplete copy.
func migrationStreams(path string) ([]migrationStream, int64, error) {
	name, err := windows.UTF16PtrFromString(nativePath(path))
	if err != nil {
		return nil, 0, err
	}
	var data struct {
		Size int64
		Name [296]uint16
	}
	h, _, err := migrationFindFirstStream.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&data)), 0)
	if h == uintptr(windows.InvalidHandle) {
		return nil, 0, err
	}
	defer windows.FindClose(windows.Handle(h))
	result := []migrationStream{}
	var total int64
	for {
		stream := windows.UTF16ToString(data.Name[:])
		if !strings.HasPrefix(stream, ":") || !strings.HasSuffix(stream, ":$DATA") || strings.ContainsAny(stream, `/\`) {
			return nil, 0, errors.New("文件包含无法校验的数据流")
		}
		if data.Size < 0 || total > math.MaxInt64-data.Size {
			return nil, 0, errors.New("文件数据流大小无效")
		}
		total += data.Size
		result = append(result, migrationStream{stream, data.Size})
		ok, _, e := migrationFindNextStream.Call(h, uintptr(unsafe.Pointer(&data)))
		if ok == 0 {
			if !errors.Is(e, windows.ERROR_HANDLE_EOF) {
				return nil, 0, e
			}
			break
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, total, nil
}
func hashMigrationStreams(ctx context.Context, path string, primary *os.File, streams []migrationStream) (string, error) {
	digest := sha256.New()
	for _, s := range streams {
		f := primary
		var err error
		if s.Name != "::$DATA" {
			f, err = openMigrationFile(path + strings.TrimSuffix(s.Name, ":$DATA"))
			if err != nil {
				return "", err
			}
		}
		hash, err := hashMigrationFile(ctx, f)
		if f != primary {
			err = errors.Join(err, f.Close())
		}
		if err != nil {
			return "", err
		}
		fmt.Fprintf(digest, "%s\x00%d\x00%s\n", s.Name, s.Size, hash)
	}
	after, _, err := migrationStreams(path)
	if err != nil {
		return "", err
	}
	if len(after) != len(streams) {
		return "", errors.New("校验期间文件数据流发生变化")
	}
	for i, s := range after {
		if s != streams[i] {
			return "", errors.New("校验期间文件数据流发生变化")
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
