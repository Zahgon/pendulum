package zoneinfo

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"io"
	"strings"
	"sync"
	"unicode/utf8"
)

// Error is a Python exception raised by Load: Class is the exception's class
// name (ZoneInfoNotFoundError, ValueError, IsADirectoryError, OSError) and
// Message is its str().
type Error struct{ Class, Message string }

// Error returns Class + ": " + Message, the last line of Python's traceback.
func (e *Error) Error() string {
	return e.Class + ": " + e.Message
}

//go:embed zoneinfo.zip
var zoneinfoZip []byte

// tzdataDir is where the reference interpreter's tzdata package keeps the
// tree; IsADirectoryError and OSError messages name paths below it.
const tzdataDir = "/usr/local/lib/python3.12/site-packages/tzdata/zoneinfo"

// nonTZifFiles are the files at the top of the tree that are not TZif.
// Only their first four bytes ever matter, so they are not embedded.
var nonTZifFiles = map[string]bool{
	"iso3166.tab": true, "leapseconds": true, "tzdata.zi": true,
	"zone.tab": true, "zone1970.tab": true, "zonenow.tab": true,
}

// tzdb is the embedded tree.
type tzdb struct {
	tzif map[string]*zip.File // TZif files by key
	dirs map[string]bool      // its directories, "" being the top
}

var database = sync.OnceValue(func() *tzdb {
	db := &tzdb{tzif: map[string]*zip.File{}, dirs: map[string]bool{"": true}}
	zr, err := zip.NewReader(bytes.NewReader(zoneinfoZip), int64(len(zoneinfoZip)))
	if err != nil {
		return db // unreachable with the committed zoneinfo.zip
	}
	for _, f := range zr.File {
		db.tzif[f.Name] = f
		for d := f.Name; strings.Contains(d, "/"); {
			d = d[:strings.LastIndexByte(d, '/')]
			db.dirs[d] = true
		}
	}
	return db
})

// The tzdata package adds, in every directory of the tree, the package's
// __init__.py and the __pycache__ directory pip compiled it into.
const (
	initPy  = "__init__.py"
	pycache = "__pycache__"
	initPyc = "__init__.cpython-312.pyc"
)

// isDir reports whether p is a directory of the installed tzdata package.
func (db *tzdb) isDir(p string) bool {
	if d, ok := strings.CutSuffix(p, pycache); ok && (d == "" || strings.HasSuffix(d, "/")) {
		return db.dirs[strings.TrimSuffix(d, "/")]
	}
	return db.dirs[p]
}

// treeFile looks key up in /usr/share/zoneinfo, which holds the TZif files
// and, at the top, the non-TZif data files. It returns the file's contents
// (nil for a non-TZif file: only its first four bytes would be read, and
// they are not "TZif") and whether key names a file at all.
func (db *tzdb) treeFile(key string) ([]byte, bool, error) {
	f, ok := db.tzif[key]
	if !ok {
		return nil, nonTZifFiles[key], nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil, true, &Error{"OSError", err.Error()}
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, true, &Error{"OSError", err.Error()}
	}
	return b, true, nil
}

// packageFile is treeFile for dir/name in the installed tzdata package,
// which adds an __init__.py to every directory and an __init__.pyc to every
// __pycache__.
func (db *tzdb) packageFile(dir, name string) ([]byte, bool, error) {
	if b, ok, err := db.treeFile(join(dir, name)); ok || err != nil {
		return b, ok, err
	}
	isFile := db.dirs[dir] && name == initPy || !db.dirs[dir] && db.isDir(dir) && name == initPyc
	return nil, isFile, nil
}

func join(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// findTZif follows ZoneInfo._new_instance up to the point where it has the
// file's contents: _tzpath.find_tzfile, then _common.load_tzdata.
func findTZif(key string) ([]byte, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	db := database()
	// TZPATH: /usr/share/zoneinfo is the tree without the package's Python
	// files; os.path.isfile is False for any path holding a NUL.
	if !strings.ContainsRune(key, 0) {
		if b, ok, err := db.treeFile(key); ok || err != nil {
			return b, err
		}
	}
	return db.loadTZData(key)
}

// dirName splits a key at its last slash.
func dirName(key string) (string, string) {
	if i := strings.LastIndexByte(key, '/'); i >= 0 {
		return key[:i], key[i+1:]
	}
	return "", key
}

// loadTZData is _common.load_tzdata: importlib.resources.files() of the
// package "tzdata.zoneinfo.<dirs joined by dots>", then opening the last
// component there.
func (db *tzdb) loadTZData(key string) ([]byte, error) {
	notFound := &Error{"ZoneInfoNotFoundError", pyRepr("No time zone found with key " + key)}
	dirs, resource := dirName(key)
	// Each dotted part must import as a subpackage: a directory of the
	// tree (a regular package) or a __pycache__ (a namespace package).
	// "__init__" imports the package's own __init__.py as a plain module,
	// whose files() is still the package directory but which has no
	// submodules.
	dir, isPkg := "", true
	if strings.Contains(key, "/") {
		for _, part := range strings.Split(strings.ReplaceAll(dirs, "/", "."), ".") {
			switch {
			case !isPkg || part == "":
				return nil, notFound
			case db.isDir(join(dir, part)):
				dir = join(dir, part)
			case part == "__init__" && db.dirs[dir]:
				isPkg = false
			default:
				return nil, notFound
			}
		}
	}
	if strings.ContainsRune(resource, 0) {
		return nil, &Error{"ValueError", "embedded null byte"}
	}
	full := tzdataDir + "/" + join(dir, resource)
	if len(resource) > 255 { // NAME_MAX
		return nil, &Error{"OSError", "[Errno 36] File name too long: " + pyRepr(full)}
	}
	if b, ok, err := db.packageFile(dir, resource); ok || err != nil {
		return b, err
	}
	if db.isDir(join(dir, resource)) {
		return nil, &Error{"IsADirectoryError", "[Errno 21] Is a directory: " + pyRepr(full)}
	}
	return nil, notFound
}

// validateKey is _tzpath._validate_tzfile_path.
func validateKey(key string) error {
	if strings.HasPrefix(key, "/") {
		return valueError("ZoneInfo keys may not be absolute paths, got: " + key)
	}
	norm := normpath(key)
	if utf8.RuneCountInString(norm) != utf8.RuneCountInString(key) {
		return valueError("ZoneInfo keys must be normalized relative paths, got: " + key)
	}
	if !strings.HasPrefix(normpath("_/"+norm), "_/") {
		return valueError("ZoneInfo keys must refer to subdirectories of TZPATH, got: " + key)
	}
	return nil
}

// normpath is posixpath.normpath for a relative path.
func normpath(p string) string {
	var comps []string
	for _, c := range strings.Split(p, "/") {
		switch {
		case c == "" || c == ".":
		case c != ".." || len(comps) == 0 || comps[len(comps)-1] == "..":
			comps = append(comps, c)
		default:
			comps = comps[:len(comps)-1]
		}
	}
	if len(comps) == 0 {
		return "."
	}
	return strings.Join(comps, "/")
}
