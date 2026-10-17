package vault

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var archiveMagic = []byte("KCARCH\x00\x01")

const (
	entryDir  = 'D'
	entryFile = 'F'
	entryEnd  = 'E'
)

type ArchiveStats struct {
	Files int64 `json:"files"`
	Dirs  int64 `json:"dirs"`
	Bytes int64 `json:"bytes"`
}

type ExcludeFunc func(rel string, isDir bool) bool

func ScanSize(root string, exclude ExcludeFunc) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel := filepath.ToSlash(mustRel(root, p))
		if exclude != nil && exclude(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func Pack(root string, w io.Writer, exclude ExcludeFunc, progress func(int64)) (ArchiveStats, error) {
	var st ArchiveStats
	bw := bufio.NewWriterSize(w, 256<<10)
	if _, err := bw.Write(archiveMagic); err != nil {
		return st, err
	}
	copyBuf := make([]byte, 256<<10)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel := filepath.ToSlash(mustRel(root, p))
		if exclude != nil && exclude(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			return nil
		}
		if err := validRelPath(rel); err != nil {
			return fmt.Errorf("cannot archive %q: %w", rel, err)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			st.Dirs++
			return writeEntryHeader(bw, entryDir, rel, info.ModTime())
		}
		if err := writeEntryHeader(bw, entryFile, rel, info.ModTime()); err != nil {
			return err
		}
		size := info.Size()
		if err := binary.Write(bw, binary.LittleEndian, uint64(size)); err != nil {
			return err
		}
		f, err := openForArchive(p)
		if err != nil {
			return err
		}
		defer f.Close()
		n, err := copyWithProgress(bw, io.LimitReader(f, size), copyBuf, func(n int64) {
			if progress != nil {
				progress(st.Bytes + n)
			}
		})
		if err != nil {
			return fmt.Errorf("archiving %q: %w", rel, err)
		}
		if n != size {
			return fmt.Errorf("archiving %q: file shrank while reading", rel)
		}
		var probe [1]byte
		if k, _ := f.Read(probe[:]); k > 0 {
			return fmt.Errorf("archiving %q: file grew while reading", rel)
		}
		st.Files++
		st.Bytes += size
		return nil
	})
	if err != nil {
		return st, err
	}
	if err := bw.WriteByte(entryEnd); err != nil {
		return st, err
	}
	if err := binary.Write(bw, binary.LittleEndian, uint64(st.Files)); err != nil {
		return st, err
	}
	if err := binary.Write(bw, binary.LittleEndian, uint64(st.Bytes)); err != nil {
		return st, err
	}
	return st, bw.Flush()
}

func openForArchive(p string) (*os.File, error) {
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		f, err := os.Open(p)
		if err == nil {
			return f, nil
		}
		lastErr = err
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
	}
	return nil, lastErr
}

func writeEntryHeader(w io.Writer, kind byte, rel string, mtime time.Time) error {
	if len(rel) > 0xFFFF {
		return fmt.Errorf("path too long: %q", rel)
	}
	hdr := make([]byte, 0, 1+2+len(rel)+8)
	hdr = append(hdr, kind)
	hdr = binary.LittleEndian.AppendUint16(hdr, uint16(len(rel)))
	hdr = append(hdr, rel...)
	hdr = binary.LittleEndian.AppendUint64(hdr, uint64(mtime.UnixNano()))
	_, err := w.Write(hdr)
	return err
}

type unpackSink interface {
	Dir(native string, mtime time.Time) error
	File(native string) (io.WriteCloser, error)
	FileDone(native string, mtime time.Time)
	Finish()
}

type rootSink struct {
	root     *os.Root
	dirTimes []dirTime
}

type dirTime struct {
	rel   string
	mtime time.Time
}

func (s *rootSink) Dir(native string, mtime time.Time) error {
	if err := s.root.MkdirAll(native, 0o700); err != nil {
		return err
	}
	s.dirTimes = append(s.dirTimes, dirTime{native, mtime})
	return nil
}

func (s *rootSink) File(native string) (io.WriteCloser, error) {
	if dir := filepath.Dir(native); dir != "." {
		if err := s.root.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	return s.root.OpenFile(native, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
}

func (s *rootSink) FileDone(native string, mtime time.Time) {
	_ = s.root.Chtimes(native, mtime, mtime)
}

func (s *rootSink) Finish() {
	for i := len(s.dirTimes) - 1; i >= 0; i-- {
		_ = s.root.Chtimes(s.dirTimes[i].rel, s.dirTimes[i].mtime, s.dirTimes[i].mtime)
	}
}

type discardSink struct{}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func (discardSink) Dir(string, time.Time) error         { return nil }
func (discardSink) File(string) (io.WriteCloser, error) { return nopWriteCloser{io.Discard}, nil }
func (discardSink) FileDone(string, time.Time)          {}
func (discardSink) Finish()                             {}

func Unpack(r io.Reader, dest string, progress func(int64)) (ArchiveStats, error) {
	root, err := os.OpenRoot(dest)
	if err != nil {
		return ArchiveStats{}, err
	}
	defer root.Close()
	return readArchive(r, &rootSink{root: root}, progress)
}

func VerifyArchive(r io.Reader) (ArchiveStats, error) {
	return readArchive(r, discardSink{}, nil)
}

func readArchive(r io.Reader, sink unpackSink, progress func(int64)) (ArchiveStats, error) {
	var st ArchiveStats
	br := bufio.NewReaderSize(r, 256<<10)
	magic := make([]byte, len(archiveMagic))
	if _, err := io.ReadFull(br, magic); err != nil {
		return st, eofToTruncated(err)
	}
	if string(magic) != string(archiveMagic) {
		return st, errors.New("vault: not a Keycat archive")
	}
	copyBuf := make([]byte, 256<<10)

	for {
		kind, err := br.ReadByte()
		if err != nil {
			return st, eofToTruncated(err)
		}
		if kind == entryEnd {
			var files, bytes uint64
			if err := binary.Read(br, binary.LittleEndian, &files); err != nil {
				return st, eofToTruncated(err)
			}
			if err := binary.Read(br, binary.LittleEndian, &bytes); err != nil {
				return st, eofToTruncated(err)
			}
			if int64(files) != st.Files || int64(bytes) != st.Bytes {
				return st, errors.New("vault: archive totals do not match its contents")
			}
			if _, err := br.ReadByte(); err != io.EOF {
				if err == nil {
					return st, ErrTrailing
				}
				return st, err
			}
			break
		}
		if kind != entryDir && kind != entryFile {
			return st, ErrCorrupt
		}
		var plen uint16
		if err := binary.Read(br, binary.LittleEndian, &plen); err != nil {
			return st, eofToTruncated(err)
		}
		pbuf := make([]byte, plen)
		if _, err := io.ReadFull(br, pbuf); err != nil {
			return st, eofToTruncated(err)
		}
		rel := string(pbuf)
		if err := validRelPath(rel); err != nil {
			return st, fmt.Errorf("vault: refusing archive entry %q: %w", rel, err)
		}
		var mt int64
		if err := binary.Read(br, binary.LittleEndian, &mt); err != nil {
			return st, eofToTruncated(err)
		}
		mtime := time.Unix(0, mt)
		native := filepath.FromSlash(rel)

		if kind == entryDir {
			if err := sink.Dir(native, mtime); err != nil {
				return st, err
			}
			st.Dirs++
			continue
		}

		var size uint64
		if err := binary.Read(br, binary.LittleEndian, &size); err != nil {
			return st, eofToTruncated(err)
		}
		f, err := sink.File(native)
		if err != nil {
			return st, err
		}
		base := st.Bytes
		n, err := copyWithProgress(f, io.LimitReader(br, int64(size)), copyBuf, func(n int64) {
			if progress != nil {
				progress(base + n)
			}
		})
		closeErr := f.Close()
		if err != nil {
			return st, err
		}
		if uint64(n) != size {
			return st, ErrTruncated
		}
		if closeErr != nil {
			return st, closeErr
		}
		sink.FileDone(native, mtime)
		st.Files++
		st.Bytes += int64(size)
	}
	sink.Finish()
	return st, nil
}

func eofToTruncated(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return ErrTruncated
	}
	return err
}

func validRelPath(rel string) error {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.ContainsAny(rel, "\\:\x00") {
		return errors.New("invalid path")
	}
	if path.Clean(rel) != rel {
		return errors.New("path is not clean")
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return errors.New("invalid path segment")
		}
	}
	return nil
}

func copyWithProgress(dst io.Writer, src io.Reader, buf []byte, progress func(int64)) (int64, error) {
	var written int64
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			w, werr := dst.Write(buf[:n])
			written += int64(w)
			if werr != nil {
				return written, werr
			}
			if w != n {
				return written, io.ErrShortWrite
			}
			progress(written)
		}
		if rerr == io.EOF {
			return written, nil
		}
		if rerr != nil {
			return written, rerr
		}
	}
}

func mustRel(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return rel
}
