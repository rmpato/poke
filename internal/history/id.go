package history

import (
	"crypto/rand"
	"encoding/binary"
	"time"
)

// idAlphabet is Crockford base32: no I, L, O or U, so an id survives being read
// aloud, copied out of a terminal, or typed back in.
const idAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// idTimeChars covers a millisecond timestamp; idRandChars is collision margin.
const (
	idTimeChars = 10 // 50 bits: enough until the year 37000
	idRandChars = 6  // 30 bits of randomness
)

// NewID returns a lexicographically sortable, collision-resistant identifier.
//
// Sortability keeps the append-only log roughly ordered even when several poke
// processes write at once, and makes ids safe to use directly as blob
// filenames. The random suffix is a collision guard, not a security boundary.
func NewID() string {
	out := make([]byte, idTimeChars+idRandChars)

	ms := uint64(time.Now().UnixMilli())
	for i := idTimeChars - 1; i >= 0; i-- {
		out[i] = idAlphabet[ms&31]
		ms >>= 5
	}

	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		binary.BigEndian.PutUint32(buf[:], uint32(time.Now().UnixNano()))
	}
	r := binary.BigEndian.Uint32(buf[:])
	for i := idTimeChars + idRandChars - 1; i >= idTimeChars; i-- {
		out[i] = idAlphabet[r&31]
		r >>= 5
	}
	return string(out)
}
