package native

import (
	"debug/pe"
	"encoding/binary"
	"strings"
)

// peImports returns the DLLs a PE file imports, from its import and
// delay-load import directories. Go's (*pe.File).ImportedLibraries is
// unimplemented (it always returns nil), and ImportedSymbols skips
// imports by ordinal, so the directories are read here.
func peImports(f *pe.File) []string {
	var dirs []pe.DataDirectory
	var imageBase uint64
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		dirs, imageBase = oh.DataDirectory[:oh.NumberOfRvaAndSizes], uint64(oh.ImageBase)
	case *pe.OptionalHeader64:
		dirs, imageBase = oh.DataDirectory[:oh.NumberOfRvaAndSizes], oh.ImageBase
	default:
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if name != "" && !seen[strings.ToLower(name)] {
			seen[strings.ToLower(name)] = true
			out = append(out, name)
		}
	}
	// The import directory: 20-byte descriptors, the DLL name's RVA at
	// offset 12, ended by an all-zero one.
	if len(dirs) > pe.IMAGE_DIRECTORY_ENTRY_IMPORT {
		d := dirs[pe.IMAGE_DIRECTORY_ENTRY_IMPORT]
		for off := uint32(0); d.VirtualAddress != 0; off += 20 {
			desc := readRVA(f, d.VirtualAddress+off, 20)
			if len(desc) < 20 || allZero(desc) {
				break
			}
			add(cString(f, binary.LittleEndian.Uint32(desc[12:])))
		}
	}
	// The delay-load directory: 32-byte descriptors, attributes at 0 (bit
	// 0 set when the addresses are RVAs, else VAs) and the name at 4.
	if len(dirs) > pe.IMAGE_DIRECTORY_ENTRY_DELAY_IMPORT {
		d := dirs[pe.IMAGE_DIRECTORY_ENTRY_DELAY_IMPORT]
		for off := uint32(0); d.VirtualAddress != 0; off += 32 {
			desc := readRVA(f, d.VirtualAddress+off, 32)
			if len(desc) < 32 || allZero(desc) {
				break
			}
			name := uint64(binary.LittleEndian.Uint32(desc[4:]))
			if binary.LittleEndian.Uint32(desc[0:])&1 == 0 && name >= imageBase {
				name -= imageBase
			}
			add(cString(f, uint32(name)))
		}
	}
	return out
}

// readRVA reads n bytes at a relative virtual address.
func readRVA(f *pe.File, rva, n uint32) []byte {
	for _, s := range f.Sections {
		size := max(s.VirtualSize, s.Size)
		if rva < s.VirtualAddress || rva >= s.VirtualAddress+size {
			continue
		}
		data, err := s.Data()
		if err != nil {
			return nil
		}
		start := rva - s.VirtualAddress
		if start >= uint32(len(data)) {
			return nil
		}
		end := min(start+n, uint32(len(data)))
		return data[start:end]
	}
	return nil
}

// cString reads a NUL-terminated string at an RVA.
func cString(f *pe.File, rva uint32) string {
	b := readRVA(f, rva, 260)
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return ""
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}
