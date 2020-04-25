package types

import "github.com/jbpratt78/vimcollab/internal/span"

// TODO: we need to reflect somehow whether a buffer is file-based or not. A
// preview window is not, for example.

// A Buffer is govim's representation of the current state of a buffer in Vim
// i.e. it is versioned.
type Buffer struct {
	Num      int
	Name     string
	contents []byte
	Version  int

	ASTWait chan bool
	// Listener is the ID of the listener for the buffer. Listeners number from
	// 1 so the zero value indicates this buffer does not have a listener.
	Listener int

	// Loaded reflects vim's "loaded" buffer state. See :help bufloaded() for details.
	Loaded bool
}

func NewBuffer(num int, name string, contents []byte, loaded bool) *Buffer {
	return &Buffer{
		Num:      num,
		Name:     name,
		contents: contents,
		Loaded:   loaded,
	}
}

// Contents returns a Buffer's contents. These contents must not be
// mutated. To update a Buffer's contents, call SetContents
func (b *Buffer) Contents() []byte {
	return b.contents
}

// SetContents updates a Buffer's contents to byts
func (b *Buffer) SetContents(byts []byte) {
	b.contents = byts
}

// URI returns the b's Name as a span.URI, assuming it is a file.
//
// TODO: we should panic here is this is not a file-based buffer
func (b *Buffer) URI() span.URI {
	return span.URIFromPath(b.Name)
}

// Range represents a range within a Buffer. Create ranges using NewRange
type Range struct {
	Start Point
	End   Point
}

// Point represents a position within a Buffer
type Point struct {
	// line is Vim's line number within the buffer, i.e. 1-indexed
	Line int

	// col is the Vim representation of column number, i.e.  1-based byte index
	Col int

	// offset is the 0-index byte-offset
	offset int

	// is the 0-index character offset in line
	utf16Col int
}
