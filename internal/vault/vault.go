package vault

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"runtime"
	"time"

	"github.com/klauspost/compress/zstd"
)

var vaultMagic = []byte("KCVAULT\x01")

const (
	CompressionZstd = "zstd"
	CompressionNone = "none"
	maxHeaderSize   = 64 << 10
)

type Header struct {
	Format      string    `json:"format"`
	Version     int       `json:"version"`
	Profile     string    `json:"profile"`
	Cipher      CipherID  `json:"cipher"`
	Salt        []byte    `json:"salt"`
	Prefixes    [][]byte  `json:"prefixes"`
	ChunkSize   int       `json:"chunkSize"`
	Compression string    `json:"compression"`
	Created     time.Time `json:"created"`
}

const (
	PhaseEncrypt = "encrypt"
	PhaseVerify  = "verify"
	PhaseDecrypt = "decrypt"
)

type Progress func(phase string, done, total int64)

type SealOptions struct {
	ProfileID   string
	Cipher      CipherID
	Compression string
	Exclude     ExcludeFunc
	Progress    Progress
	KeepBackup  bool
}

type SealResult struct {
	Archive    ArchiveStats  `json:"archive"`
	VaultBytes int64         `json:"vaultBytes"`
	Duration   time.Duration `json:"duration"`
}

func Seal(srcDir, vaultPath string, mk *SecretKey, opt SealOptions) (SealResult, error) {
	start := time.Now()
	var res SealResult
	if !opt.Cipher.Valid() {
		return res, fmt.Errorf("unknown cipher %q", opt.Cipher)
	}
	if opt.Compression == "" {
		opt.Compression = CompressionZstd
	}
	progress := opt.Progress
	if progress == nil {
		progress = func(string, int64, int64) {}
	}
	total, err := ScanSize(srcDir, opt.Exclude)
	if err != nil {
		return res, err
	}

	tmp := vaultPath + ".tmp"
	if err := writeVault(tmp, srcDir, mk, opt, total, progress, &res); err != nil {
		_ = os.Remove(tmp)
		return res, err
	}
	if _, err := verifyVault(tmp, opt.ProfileID, mk, progress); err != nil {
		_ = os.Remove(tmp)
		return res, fmt.Errorf("verification of the new vault failed: %w", err)
	}
	if _, err := os.Stat(vaultPath); err == nil && opt.KeepBackup {
		if err := RenameDurable(vaultPath, vaultPath+".bak"); err != nil {
			_ = os.Remove(tmp)
			return res, err
		}
	}
	if err := RenameDurable(tmp, vaultPath); err != nil {
		return res, err
	}
	res.Duration = time.Since(start)
	return res, nil
}

func writeVault(path, srcDir string, mk *SecretKey, opt SealOptions, total int64, progress Progress, res *SealResult) (err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	bw := bufio.NewWriterSize(f, 1<<20)
	cw := &countingWriter{w: bw}

	hdr, s, err := newHeader(opt.ProfileID, opt.Cipher, opt.Compression, mk)
	if err != nil {
		return err
	}
	hdrBytes, err := encodeHeader(hdr)
	if err != nil {
		return err
	}
	if _, err := cw.Write(hdrBytes); err != nil {
		return err
	}
	aad := sha256.Sum256(hdrBytes)
	sw := newStreamWriter(cw, s, hdr.Prefixes, aad[:], hdr.ChunkSize)

	var payload io.Writer = sw
	var enc *zstd.Encoder
	if hdr.Compression == CompressionZstd {
		enc, err = zstd.NewWriter(sw,
			zstd.WithEncoderLevel(zstd.SpeedDefault),
			zstd.WithEncoderConcurrency(max(1, runtime.NumCPU()/2)))
		if err != nil {
			return err
		}
		payload = enc
	}
	st, err := Pack(srcDir, payload, opt.Exclude, func(done int64) {
		progress(PhaseEncrypt, done, total)
	})
	if err != nil {
		if enc != nil {
			enc.Close()
		}
		return err
	}
	if enc != nil {
		if err := enc.Close(); err != nil {
			return err
		}
	}
	if err := sw.Close(); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	res.Archive = st
	res.VaultBytes = cw.n
	return nil
}

func newHeader(profileID string, cipherID CipherID, compression string, mk *SecretKey) (Header, *suite, error) {
	h := Header{
		Format:      "keycat-vault",
		Version:     1,
		Profile:     profileID,
		Cipher:      cipherID,
		Salt:        RandomBytes(32),
		ChunkSize:   DefaultChunkSize,
		Compression: compression,
		Created:     time.Now().UTC(),
	}
	s, err := newSuite(cipherID, mk.Bytes(), h.Salt, "data")
	if err != nil {
		return h, nil, err
	}
	for _, l := range s.layers {
		h.Prefixes = append(h.Prefixes, RandomBytes(noncePrefixSize(l.NonceSize())))
	}
	return h, s, nil
}

func encodeHeader(h Header) ([]byte, error) {
	js, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	out := append([]byte(nil), vaultMagic...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(js)))
	return append(out, js...), nil
}

func ReadHeader(r io.Reader) (Header, []byte, error) {
	var h Header
	pre := make([]byte, len(vaultMagic)+4)
	if _, err := io.ReadFull(r, pre); err != nil {
		return h, nil, errors.New("vault: file is too short")
	}
	if string(pre[:len(vaultMagic)]) != string(vaultMagic) {
		return h, nil, errors.New("vault: not a Keycat vault")
	}
	n := binary.LittleEndian.Uint32(pre[len(vaultMagic):])
	if n == 0 || n > maxHeaderSize {
		return h, nil, ErrCorrupt
	}
	js := make([]byte, n)
	if _, err := io.ReadFull(r, js); err != nil {
		return h, nil, ErrTruncated
	}
	if err := json.Unmarshal(js, &h); err != nil {
		return h, nil, ErrCorrupt
	}
	if h.Format != "keycat-vault" || h.Version != 1 {
		return h, nil, fmt.Errorf("vault: unsupported format %q v%d", h.Format, h.Version)
	}
	kinds, err := h.Cipher.layerKinds()
	if err != nil {
		return h, nil, err
	}
	if len(h.Salt) != 32 || len(h.Prefixes) != len(kinds) {
		return h, nil, ErrCorrupt
	}
	if h.ChunkSize < 64<<10 || h.ChunkSize > 16<<20 {
		return h, nil, ErrCorrupt
	}
	if h.Compression != CompressionZstd && h.Compression != CompressionNone {
		return h, nil, ErrCorrupt
	}
	return h, append(pre, js...), nil
}

type payload struct {
	io.Reader
	sr  *streamReader
	dec *zstd.Decoder
}

func (p *payload) finish() error {
	p.close()
	_, err := io.Copy(io.Discard, p.sr)
	return err
}

func (p *payload) close() {
	if p.dec != nil {
		p.dec.Close()
		p.dec = nil
	}
}

func openPayload(r io.Reader, profileID string, mk *SecretKey) (*payload, error) {
	h, hdrBytes, err := ReadHeader(r)
	if err != nil {
		return nil, err
	}
	if profileID != "" && h.Profile != profileID {
		return nil, errors.New("vault belongs to a different profile")
	}
	s, err := newSuite(h.Cipher, mk.Bytes(), h.Salt, "data")
	if err != nil {
		return nil, err
	}
	for i, l := range s.layers {
		if len(h.Prefixes[i]) != noncePrefixSize(l.NonceSize()) {
			return nil, ErrCorrupt
		}
	}
	aad := sha256.Sum256(hdrBytes)
	sr := newStreamReader(r, s, h.Prefixes, aad[:], h.ChunkSize)
	if h.Compression == CompressionNone {
		return &payload{Reader: sr, sr: sr}, nil
	}
	dec, err := zstd.NewReader(sr, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return nil, err
	}
	return &payload{Reader: dec, sr: sr, dec: dec}, nil
}

func Unseal(vaultPath, destDir, profileID string, mk *SecretKey, progress Progress) (ArchiveStats, error) {
	if progress == nil {
		progress = func(string, int64, int64) {}
	}
	f, err := os.Open(vaultPath)
	if err != nil {
		return ArchiveStats{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ArchiveStats{}, err
	}
	cr := &countingReader{r: bufio.NewReaderSize(f, 1<<20)}
	p, err := openPayload(cr, profileID, mk)
	if err != nil {
		return ArchiveStats{}, err
	}
	defer p.close()
	st, err := Unpack(p, destDir, func(int64) {
		progress(PhaseDecrypt, cr.n, info.Size())
	})
	if err != nil {
		return st, err
	}
	return st, p.finish()
}

func Verify(vaultPath, profileID string, mk *SecretKey) (ArchiveStats, error) {
	return verifyVault(vaultPath, profileID, mk, nil)
}

func VerifyProgress(vaultPath, profileID string, mk *SecretKey, progress Progress) (ArchiveStats, error) {
	return verifyVault(vaultPath, profileID, mk, progress)
}

func verifyVault(vaultPath, profileID string, mk *SecretKey, progress Progress) (ArchiveStats, error) {
	f, err := os.Open(vaultPath)
	if err != nil {
		return ArchiveStats{}, err
	}
	defer f.Close()
	info, _ := f.Stat()
	cr := &countingReader{r: bufio.NewReaderSize(f, 1<<20)}
	p, err := openPayload(cr, profileID, mk)
	if err != nil {
		return ArchiveStats{}, err
	}
	defer p.close()
	var src io.Reader = p
	if progress != nil {
		src = &progressReader{r: p, fn: func() {
			progress(PhaseVerify, cr.n, info.Size())
		}}
	}
	st, err := VerifyArchive(src)
	if err != nil {
		return st, err
	}
	return st, p.finish()
}

func RecoverInterruptedSeal(vaultPath, profileID string, mk *SecretKey) (string, error) {
	tmp, bak := vaultPath+".tmp", vaultPath+".bak"
	if _, err := os.Stat(vaultPath); err == nil {
		_ = os.Remove(tmp)
		return "", nil
	}
	if _, err := os.Stat(tmp); err == nil {
		if _, verr := Verify(tmp, profileID, mk); verr == nil {
			return "recovered the vault written just before Keycat stopped", RenameDurable(tmp, vaultPath)
		}
		_ = os.Remove(tmp)
	}
	if _, err := os.Stat(bak); err == nil {
		return "restored the previous vault from its backup", RenameDurable(bak, vaultPath)
	}
	return "", fs.ErrNotExist
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

type progressReader struct {
	r  io.Reader
	fn func()
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.fn()
	return n, err
}
