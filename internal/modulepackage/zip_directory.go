package modulepackage

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
)

const maxZIPDirectoryBytes = 8 << 20
const maxArchiveBytes = MaxModulePayloadBytes + (32 << 20)

type zipDirectory struct {
	offset, end int64
	entries     int
}

// Inspect the standard EOCD before archive/zip can preallocate an entry slice.
// Module format v1 fits ZIP32 bounds; split/ZIP64 archives and trailing data are
// rejected. Offsets follow PKWARE APPNOTE sections 4.3.12 and 4.3.16.
func inspectZIPEnd(tail []byte, base, size int64) (zipDirectory, error) {
	bad := errors.New("unsupported or invalid module ZIP directory")
	for index := len(tail) - 22; index >= 0; index-- {
		if binary.LittleEndian.Uint32(tail[index:]) != 0x06054b50 {
			continue
		}
		header := tail[index:]
		comment := int(binary.LittleEndian.Uint16(header[20:]))
		// Match archive/zip's last plausible record, then require an exact EOF.
		if 22+comment > len(header) {
			continue
		}
		if 22+comment != len(header) {
			return zipDirectory{}, bad
		}
		count := int(binary.LittleEndian.Uint16(header[10:]))
		length := int64(binary.LittleEndian.Uint32(header[12:]))
		offset := int64(binary.LittleEndian.Uint32(header[16:]))
		end := base + int64(index)
		if binary.LittleEndian.Uint16(header[4:]) != 0 || binary.LittleEndian.Uint16(header[6:]) != 0 || int(binary.LittleEndian.Uint16(header[8:])) != count || count < 2 || count > MaxModuleFiles+2 || length < int64(count*46) || length > maxZIPDirectoryBytes || offset < 0 || offset+length != end || end >= size {
			return zipDirectory{}, bad
		}
		if index >= 20 && binary.LittleEndian.Uint32(tail[index-20:]) == 0x07064b50 {
			return zipDirectory{}, bad
		}
		return zipDirectory{offset: offset, end: end, entries: count}, nil
	}
	return zipDirectory{}, bad
}

type immutableZIPDirectory struct {
	source io.ReaderAt
	tail   *bytes.Reader
	offset int64
	ctx    context.Context
}

func (r *immutableZIPDirectory) ReadAt(p []byte, offset int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if offset < 0 {
		return 0, errors.New("negative archive offset")
	}
	if offset >= r.offset {
		return r.tail.ReadAt(p, offset-r.offset)
	}
	prefix := len(p)
	if int64(prefix) > r.offset-offset {
		prefix = int(r.offset - offset)
	}
	n, err := r.source.ReadAt(p[:prefix], offset)
	if err != nil || prefix == len(p) {
		return n, err
	}
	copied, err := r.tail.ReadAt(p[prefix:], 0)
	return n + copied, err
}

// Snapshot only the bounded directory and footer. Payload bytes stay on disk;
// later installation must copy and reverify them before committing a version.
func snapshotZIPDirectory(ctx context.Context, source io.ReaderAt, size int64) (*immutableZIPDirectory, int, error) {
	if size < 22 || size > maxArchiveBytes {
		return nil, 0, errors.New("module archive size is invalid")
	}
	tailSize := int64(65 * 1024)
	if size < tailSize {
		tailSize = size
	}
	tail := make([]byte, int(tailSize))
	if _, err := source.ReadAt(tail, size-tailSize); err != nil {
		return nil, 0, err
	}
	first, err := inspectZIPEnd(tail, size-tailSize, size)
	if err != nil {
		return nil, 0, err
	}
	data := make([]byte, int(size-first.offset))
	if _, err := source.ReadAt(data, first.offset); err != nil {
		return nil, 0, err
	}
	final, err := inspectZIPEnd(data, first.offset, size)
	if err != nil {
		return nil, 0, err
	}
	if final.offset != first.offset {
		return nil, 0, errors.New("module ZIP directory changed while reading")
	}
	cursor := 0
	limit := int(final.end - final.offset)
	for index := 0; index < final.entries; index++ {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		if cursor+46 > limit {
			return nil, 0, errors.New("truncated module ZIP directory")
		}
		header := data[cursor : cursor+46]
		if binary.LittleEndian.Uint32(header) != 0x02014b50 {
			return nil, 0, errors.New("invalid module ZIP entry")
		}
		name := int(binary.LittleEndian.Uint16(header[28:]))
		extra := int(binary.LittleEndian.Uint16(header[30:]))
		comment := int(binary.LittleEndian.Uint16(header[32:]))
		method := binary.LittleEndian.Uint16(header[10:])
		if name == 0 || name > 1024 || cursor+46+name+extra+comment > limit || binary.LittleEndian.Uint16(header[34:]) != 0 || binary.LittleEndian.Uint16(header[8:])&1 != 0 || (method != 0 && method != 8) || int64(binary.LittleEndian.Uint32(header[20:])) > size || int64(binary.LittleEndian.Uint32(header[24:])) > MaxModuleFileBytes || int64(binary.LittleEndian.Uint32(header[42:]))+30 > final.offset {
			return nil, 0, errors.New("unsupported module ZIP entry")
		}
		cursor += 46 + name + extra + comment
	}
	if cursor != limit {
		return nil, 0, errors.New("module ZIP entry count mismatch")
	}
	return &immutableZIPDirectory{source: source, tail: bytes.NewReader(data), offset: final.offset, ctx: ctx}, final.entries, nil
}
