package par2

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"hash"
	"crypto/md5"
	"bytes"
	"errors"
	"math/big"
	"os"
)

type Fileset struct {
	setID [16]byte
	slicelen uint64
	complete bool
	files map[[16]byte]*File
	checksums map[[16]byte]chksum 
}

type File struct {
	Name string
	md5 [16]byte
	md5_16k [16]byte
	length uint64
	checksums [][16]byte
}

func (f *File) numBlocks(fset *Fileset) int {
	blockcount := int(f.length / fset.slicelen)
	if f.length % fset.slicelen != 0 {
		blockcount++
	}
	return blockcount
}

type chksum struct {
	*File
	blockno int
}

// NewFileset reads r and returns a Fileset that can be used for verification and recovery of the files.
func NewFileset(r io.Reader) (*Fileset, error) {
	fset := &Fileset{}
	fset.files = make(map[[16]byte]*File)
	fset.checksums = make(map[[16]byte]chksum)
	bufr := bufio.NewReader(r)
	for {
		hdr, err := readHeader(bufr)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if fset.setID == ([16]byte{}) {
			fset.setID = hdr.setID
		} else if hdr.setID != fset.setID {
			// this is weird and shouldn't happen
			return nil, errors.New("mismatched set ID in one file")
		}
		switch hdr.typ {
		case typeFileDesc:
			f, id, _ := readFileDesc(hdr, bufr)
			if f == nil {
				continue
			}
			fi, ok := fset.files[id]
			if ok {
				if fi.Name == "" {
					// file was discovered through some means
					// that didn't include the file info, usually IFSC
					// pkt. Fill in the information now that we know what file this is.
					f.checksums = fi.checksums
					fset.files[id] = f
				}
			} else {
				fset.files[id] = f
			}
		case typeIFSC:
			chksums, id, _ := readIFSC(hdr, bufr)
			if chksums == nil {
				continue
			}
			fi, ok := fset.files[id]
			if !ok {
				fi = new(File)
				fset.files[id] = fi
			}
			if fi.checksums == nil {
				fi.checksums = chksums
			}
			for i, chk := range fi.checksums {
				fset.checksums[chk] = chksum{
					File: fi,
					blockno: i,
				}
			}
		case typeMain:
			slicelen, ids, _ := readMain(hdr, bufr)
			for _, id := range ids {
				if _, ok := fset.files[id]; !ok {
					fset.files[id] = new(File)
				}
			}
			fset.slicelen = slicelen
		default:
		}
	}
	if !fset.CanVerify() {
		return fset, nil
	}
	return fset, nil
}


// CanVerify returns whether the current fileset can be
// used for verification.
func (f *Fileset) CanVerify() bool {
	if f.complete {
		return true
	}
	if f.slicelen == 0 {
		return false
	}
	for _, file := range f.files {
		if file.Name == "" || file.checksums == nil {
			return false
		}
	}
	f.complete = true
	return true
}

// Verify verifies the files at paths against the fileset.
// It returns a list of matches and how many blocks are needed in order to repair.
func (f *Fileset) Verify(paths []string) ([]*FileMatch, int) {
	if !f.complete {
		return nil, 0
	}
	files := make(map[[16]byte]*File, len(f.files))
	for _, v := range f.files {
		files[v.md5] = v
	}
	matches := make([]*FileMatch, 0, len(paths))
	blocksNeeded := 0
	for _, s := range paths {
		fm, blocksmissing := f.verifyfile(files, s)
		if fm != nil && fm.File != nil {
			delete(files, fm.File.md5)
			blocksNeeded += blocksmissing
			matches = append(matches, fm)
		}
	}
	for _, fi := range files {
		matches = append(matches, &FileMatch{ Err:ErrMissing, File: fi })
		blocksNeeded += fi.numBlocks(f)
	}
	return matches, blocksNeeded
}

var ErrMissing = errors.New("par2: file missing")

func (fset *Fileset) verifyfile(files map[[16]byte]*File, s string) (*FileMatch, int) {
	file, err := os.Open(s)
	if err != nil {
		return &FileMatch{Err: err}, 0
	}
	defer file.Close()
	
	chk := md5.New()
	_, err = io.Copy(chk, file)
	if err != nil {
		return &FileMatch{Err: err},0
	}
	var md5sum [16]byte
	chk.Sum(md5sum[:0])
	fi := files[md5sum]
	if fi != nil {
		return &FileMatch{ Path: s, File: fi },0
	}
	
	// ok, this file is either corrupted or not part of the recovery set.
	// Since we're downloading these files via yenc (hopefully), we can always
	// assume that they will have holes where they were corrupted.	
	_, err = file.Seek(0, os.SEEK_SET)
	if err != nil {
		return &FileMatch{Err: err}, 0
	}
	partialMatch := &FileMatch{}
	for {
		mdchk := md5.New()
		_, err := io.CopyN(mdchk, file, int64(fset.slicelen))
		mdchk.Sum(md5sum[:0])
		if f, ok := fset.checksums[md5sum]; ok {
			if partialMatch.File == nil {
				// ok we have a match, init the block bitmap
				partialMatch.blocks = &big.Int{}
				partialMatch.File = f.File
				partialMatch.Path = s
			} else if partialMatch.File != f.File {
				// shit
				continue
			}
			partialMatch.blocks.SetBit(partialMatch.blocks, f.blockno, 1)
		}
		if err != nil {
			break
		}
	}
	if partialMatch.File == nil {
		// not part of the recovery set.
		return nil, 0
	}
	blockcount := partialMatch.File.numBlocks(fset)
	blocksmissing := 0
	for i := 0; i < blockcount; i++ {
		if partialMatch.blocks.Bit(i) == 0 {
			blocksmissing++
		}
	}
	return partialMatch, blocksmissing
}

type FileMatch struct {
	Err error
	Path string
	File *File
	blocks *big.Int
}

type hdr struct {
	length uint64
	hash   [16]byte
	setID  [16]byte
	typ  typ
	partialhash hash.Hash
}

func (h *hdr) String() string {
	return fmt.Sprintf("{\n\t%d\n\t%x\n\t%x\n\t%s\n}", h.length, h.hash, h.setID, h.typ)
}

func readHeader(r *bufio.Reader) (h hdr, err error) {
	err = findHeader(r)
	if err != nil {
		return h, err
	}
	defer func() {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
	}()
	var buf [56]byte
	b := buf[:]
	_, err = io.ReadFull(r, b)
	if err != nil {
		return h, err
	}
	h.length, b = readint(b)
	// for convenience, turn the length into
	// length after the header has been read.
	h.length -= 64
	h.hash, b = readmd5(b)
	
	// create a partial hash so that we can create a hash of the
	// entire packet.
	h.partialhash = md5.New()
	h.partialhash.Write(b)
	
	h.setID, b = readmd5(b)
	
	// i know, it's not an md5sum
	typbuf, b := readmd5(b)
	switch typbuf {
	case magicMain:
		h.typ = typeMain
	case magicFiledesc:
		h.typ = typeFileDesc
	case magicIFSC:
		h.typ = typeIFSC
	case magicRecvSlic:
		h.typ = typeRecvSlic
	case magicCreator:
		h.typ = typeCreator
	default:
		h.typ = typeUnknown
	}

	return h, nil
}

// These are all the required types.
// Going to leave it at that
var (
	magicMain     = [16]byte{'P', 'A', 'R', ' ', '2', '.', '0', 0, 'M', 'a', 'i', 'n', 0, 0, 0, 0}
	magicFiledesc = [16]byte{'P', 'A', 'R', ' ', '2', '.', '0', 0, 'F', 'i', 'l', 'e', 'D', 'e', 's', 'c'}
	magicIFSC     = [16]byte{'P', 'A', 'R', ' ', '2', '.', '0', 0, 'I', 'F', 'S', 'C', 0, 0, 0, 0}
	magicRecvSlic = [16]byte{'P', 'A', 'R', ' ', '2', '.', '0', 0, 'R', 'e', 'c', 'v', 'S', 'l', 'i', 'c'}
	magicCreator  = [16]byte{'P', 'A', 'R', ' ', '2', '.', '0', 0, 'C', 'r', 'e', 'a', 't', 'o', 'r', 0}
)

// corruption might cause the header
// to become unaligned, so just search for the damn
// header.
func findHeader(r *bufio.Reader) error {
	i := 0
	str := "PAR2\x00PKT"
	for {
		//when completely matched
		if i == len(str) {
			return nil
		}
		c, err := r.ReadByte()
		if err != nil {
			return err
		}
		if str[i] == c {
			i++
			continue
		}
		i = 0
		if str[0] == c {
			i++
		}
	}
}

func readFileDesc(h hdr, r *bufio.Reader) (f *File, id [16]byte, err error) {
	buf, err := readPkt(h, r)
	if err != nil {
		return nil, id, nil
	}
	f = new(File)
	id, buf = readmd5(buf)
	f.md5, buf = readmd5(buf)
	f.md5_16k, buf = readmd5(buf)
	f.length, buf = readint(buf)
	
	// rest of block is name, trim 0 padding.
	i := bytes.LastIndex(buf, zero)
	if i != -1 {
		buf = buf[:i]
	}
	f.Name = string(buf)
	return f, id, nil
}

var zero = []byte{0}

func readIFSC(h hdr, r *bufio.Reader) (ss [][16]byte, id [16]byte, err error) {
	buf, err := readPkt(h, r)
	if err != nil {
		return nil, id, nil
	}
	id, buf = readmd5(buf)
	ss = make([][16]byte, 0, len(buf)/20)
	for len(buf) > 0 {
		var md5h [16]byte
		md5h, buf = readmd5(buf)
		// don't care about the crc, just that it gets consumed
		_, buf = readcrc(buf)
		ss = append(ss, md5h)
	}
	return ss, id, nil
}

func readMain(h hdr, r *bufio.Reader) (slicesize uint64, ids [][16]byte, err error) {
	buf, err := readPkt(h, r)
	if err != nil {
		return 0, nil, nil
	}
	slicesize, buf = readint(buf)
	numfiles, buf := readcrc(buf)
	ids = make([][16]byte, 0, numfiles)
	for i := uint32(0); i < numfiles; i++ {
		var nid [16]byte
		nid, buf = readmd5(buf)
		ids = append(ids, nid)
	}

	return slicesize, ids, nil
}

func readmd5(b []byte) ([16]byte, []byte) {
	var ret [16]byte
	copy(ret[:], b[:16])
	b = b[16:]
	return ret, b
}

func readint(b []byte) (uint64, []byte) {
	ret := binary.LittleEndian.Uint64(b)
	b = b[8:]
	return ret, b
}

func readcrc(b []byte) (uint32, []byte) {
	ret := binary.LittleEndian.Uint32(b)
	b = b[4:]
	return ret, b
}

// to be used for packets that can be read in a single go. Verifies
// the md5 sum of the packet as well
func readPkt(h hdr, r *bufio.Reader) ([]byte, error) {
	buf := make([]byte, h.length)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	// check if the packet is valid
	h.partialhash.Write(buf)
	hash := h.partialhash.Sum(nil)
	for i, b := range h.hash {
		if hash[i] != b {
			return nil, errors.New("mismatch packet md5 and packet contents")
		}
	}
	return buf, nil
}
