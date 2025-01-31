package service

import (
	randc "crypto/rand"
	"errors"
	"fmt"
	"math"
	randm "math/rand"
	"sync"
	"time"
)

// DefaultABC is the default URL-friendly alphabet.
const DefaultABC = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-"

// Abc represents a shuffled alphabet used to generate the Ids and provides methods to
// encode data.
type Abc struct {
	alphabet []rune
}

// Shortid type represents a short Id generator working with a given alphabet.
type Shortid struct {
	abc    Abc
	worker uint
	epoch  time.Time  // ids can be generated for 34 years since this date
	ms     uint       // ms since epoch for the last id
	count  uint       // request count within the same ms
	mx     sync.Mutex // locks access to ms and count
}

var shortid *Shortid

func init() {
	shortid = MustNew(0, DefaultABC, 1)
}

// Generate generates an Id using the default generator.
func Generate() (string, error) {
	return shortid.Generate()
}

// ShortUUID generates a new short Id using the default generator.
func ShortUUID() string {
	id, err := Generate()
	if err == nil {
		return id
	}
	panic(err)
}

// UUID generates a new UUID v4.
func UUID() string {
	uuid := make([]byte, 16)
	if _, err := randc.Read(uuid); err != nil {
		return ""
	}

	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
}

// New constructs an instance of the short Id generator for the given worker number [0,31], alphabet
// (64 unique symbols) and seed value (to shuffle the alphabet). The worker number should be
// different for multiple or distributed processes generating Ids into the same data space. The
// seed, on contrary, should be identical.
func New(worker uint8, alphabet string, seed uint64) (*Shortid, error) {
	if worker > 31 {
		return nil, errors.New("expected worker in the range [0,31]")
	}
	abc, err := NewAbc(alphabet, seed)
	if err == nil {
		sid := &Shortid{
			abc:    abc,
			worker: uint(worker),
			epoch:  time.Date(2016, time.January, 1, 0, 0, 0, 0, time.UTC),
			ms:     0,
			count:  0,
		}
		return sid, nil
	}
	return nil, err
}

// MustNew acts just like New, but panics instead of returning errors.
func MustNew(worker uint8, alphabet string, seed uint64) *Shortid {
	sid, err := New(worker, alphabet, seed)
	if err == nil {
		return sid
	}
	panic(err)
}

// Generate generates a new short Id.
func (sid *Shortid) Generate() (string, error) {
	return sid.GenerateInternal(nil, sid.epoch)
}

// MustGenerate acts just like Generate, but panics instead of returning errors.
func (sid *Shortid) MustGenerate() string {
	id, err := sid.Generate()
	if err == nil {
		return id
	}
	panic(err)
}

// GenerateInternal should only be used for testing purposes.
func (sid *Shortid) GenerateInternal(tm *time.Time, epoch time.Time) (string, error) {
	ms, count := sid.getMsAndCounter(tm, epoch)
	idrunes := make([]rune, 9)
	if tmp, err := sid.abc.Encode(ms, 8, 5); err == nil {
		copy(idrunes, tmp) // first 8 symbols
	} else {
		return "", err
	}
	if tmp, err := sid.abc.Encode(sid.worker, 1, 5); err == nil {
		idrunes[8] = tmp[0]
	} else {
		return "", err
	}
	if count > 0 {
		if countrunes, err := sid.abc.Encode(count, 0, 6); err == nil {
			// only extend if really need it
			idrunes = append(idrunes, countrunes...)
		} else {
			return "", err
		}
	}
	return string(idrunes), nil
}

func (sid *Shortid) getMsAndCounter(tm *time.Time, epoch time.Time) (uint, uint) {
	sid.mx.Lock()
	defer sid.mx.Unlock()
	var ms uint
	if tm != nil {
		ms = uint(tm.Sub(epoch).Nanoseconds() / 1000000)
	} else {
		ms = uint(time.Now().Sub(epoch).Nanoseconds() / 1000000)
	}
	if ms == sid.ms {
		sid.count++
	} else {
		sid.count = 0
		sid.ms = ms
	}
	return sid.ms, sid.count
}

// NewAbc constructs a new instance of shuffled alphabet to be used for Id representation.
func NewAbc(alphabet string, seed uint64) (Abc, error) {
	runes := []rune(alphabet)
	if len(runes) != len(DefaultABC) {
		return Abc{}, fmt.Errorf("alphabet must contain %v unique characters", len(DefaultABC))
	}
	if nonUnique(runes) {
		return Abc{}, errors.New("alphabet must contain unique characters only")
	}
	abc := Abc{alphabet: nil}
	abc.shuffle(alphabet, seed)
	return abc, nil
}

func nonUnique(runes []rune) bool {
	found := make(map[rune]struct{})
	for _, r := range runes {
		if _, seen := found[r]; !seen {
			found[r] = struct{}{}
		}
	}
	return len(found) < len(runes)
}

func (abc *Abc) shuffle(alphabet string, seed uint64) {
	source := []rune(alphabet)
	for len(source) > 1 {
		seed = (seed*9301 + 49297) % 233280
		i := int(seed * uint64(len(source)) / 233280)

		abc.alphabet = append(abc.alphabet, source[i])
		source = append(source[:i], source[i+1:]...)
	}
	abc.alphabet = append(abc.alphabet, source[0])
}

// Encode encodes a given value into a slice of runes of length nsymbols. In case nsymbols==0, the
// length of the result is automatically computed from data. Even if fewer symbols is required to
// encode the data than nsymbols, all positions are used encoding 0 where required to guarantee
// uniqueness in case further data is added to the sequence. The value of digits [4,6] represents
// represents n in 2^n, which defines how much randomness flows into the algorithm: 4 -- every value
// can be represented by 4 symbols in the alphabet (permitting at most 16 values), 5 -- every value
// can be represented by 2 symbols in the alphabet (permitting at most 32 values), 6 -- every value
// is represented by exactly 1 symbol with no randomness (permitting 64 values).
func (abc *Abc) Encode(val, nsymbols, digits uint) ([]rune, error) {
	if digits < 4 || 6 < digits {
		return nil, fmt.Errorf("allowed digits range [4,6], found %v", digits)
	}

	var computedSize uint = 1
	if val >= 1 {
		computedSize = uint(math.Log2(float64(val)))/digits + 1
	}
	if nsymbols == 0 {
		nsymbols = computedSize
	} else if nsymbols < computedSize {
		return nil, fmt.Errorf("cannot accommodate data, need %v digits, got %v", computedSize, nsymbols)
	}

	mask := 1<<digits - 1

	random := make([]int, int(nsymbols))
	// no random component if digits == 6
	if digits < 6 {
		copy(random, maskedRandomInts(len(random), 0x3f-mask))
	}

	res := make([]rune, int(nsymbols))
	for i := range res {
		shift := digits * uint(i)
		index := (int(val>>shift) & mask) | random[i]
		res[i] = abc.alphabet[index]
	}
	return res, nil
}

// MustEncode acts just like Encode, but panics instead of returning errors.
func (abc *Abc) MustEncode(val, size, digits uint) []rune {
	res, err := abc.Encode(val, size, digits)
	if err == nil {
		return res
	}
	panic(err)
}

func maskedRandomInts(size, mask int) []int {
	ints := make([]int, size)
	bytes := make([]byte, size)
	if _, err := randc.Read(bytes); err == nil {
		for i, b := range bytes {
			ints[i] = int(b) & mask
		}
	} else {
		for i := range ints {
			ints[i] = randm.Intn(0xff) & mask
		}
	}
	return ints
}
