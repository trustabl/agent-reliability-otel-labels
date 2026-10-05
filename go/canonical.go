package otellabels

// Fingerprints: sameness of a tool call without its contents.
//
// The canonical form is RFC 8785 (JCS), the same in every binding. The value
// is walked here rather than handed to jcs.Transform, which rounds integers
// beyond 2^53 without complaint (9007199254740993 becomes 9007199254740992):
// two different calls would then fingerprint alike. The library is used only
// for float formatting, via jcs.NumberToJSON. Strings are written here too,
// because encoding/json escapes <, > and &.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
)

// FPLength is the number of hex characters in a fingerprint.
const FPLength = 16

// maxSafeInteger is the largest integer a double holds exactly. JSON numbers
// are doubles in JavaScript, so an integral value beyond it may already have
// been rounded there.
const maxSafeInteger = 1<<53 - 1

// maxDepth bounds nesting. A Go stack overflow is fatal, not recoverable, so a
// cyclic map or slice must be refused before it recurses forever.
const maxDepth = 1000

// ErrNotCanonical is wrapped by every error for a value with no faithful
// canonical form. Such a value gets no fingerprint rather than a wrong one.
var ErrNotCanonical = errors.New("otellabels: value cannot be canonicalized")

func notCanonical(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrNotCanonical, fmt.Sprintf(format, args...))
}

// volatileKeys are stripped before hashing: transport and bookkeeping that
// changes between two otherwise identical calls. Matched EXACTLY and
// case-insensitively, never as substrings - customer_id and date are real
// input. Identical to Python's VOLATILE_KEYS.
var volatileKeys = map[string]bool{
	"timestamp": true, "ts": true, "_ts": true, "created_at": true, "updated_at": true,
	"sent_at": true, "received_at": true,
	"request_id": true, "requestid": true, "req_id": true,
	"trace_id": true, "traceid": true, "span_id": true, "spanid": true, "correlation_id": true,
	"nonce": true, "uuid": true, "guid": true,
	"cursor": true, "page_token": true, "next_page_token": true, "continuation_token": true,
	"idempotency_key": true,
}

func isVolatile(key string) bool {
	// Python and JavaScript lowercase U+0130 (İ) to "i̇", two code points, so a
	// key containing it never matches. Go's simple case mapping gives "i",
	// which would strip "TİMESTAMP" in Go alone. It is the only character
	// whose full lowercase mapping differs from the simple one.
	if strings.ContainsRune(key, '\u0130') {
		return false
	}
	return volatileKeys[strings.ToLower(key)]
}

// loneSurrogate stands, in a decoded json.RawMessage, for a string with an
// unpaired surrogate escape or an object with such a key. It is refused only
// if it survives volatile stripping, as the other bindings refuse it.
type loneSurrogate struct{}

var (
	rawMessageType    = reflect.TypeFor[json.RawMessage]()
	numberType        = reflect.TypeFor[json.Number]()
	loneSurrogateType = reflect.TypeFor[loneSurrogate]()
)

// StripVolatile returns v with volatile fields removed at any depth, as plain
// JSON values: nil, bool, string, int64, float64, []any and map[string]any.
// It returns an error wrapping ErrNotCanonical for a value that has no
// canonical form.
func StripVolatile(v any) (out any, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, notCanonical("panic while stripping volatile fields: %v", r)
		}
	}()
	return normalize(reflect.ValueOf(v), 0)
}

func normalize(rv reflect.Value, depth int) (any, error) {
	if depth > maxDepth {
		return nil, notCanonical("nested deeper than %d levels, or cyclic", maxDepth)
	}
	if !rv.IsValid() {
		return nil, nil
	}
	switch rv.Type() {
	case rawMessageType:
		// As encoding/json.Marshal renders it.
		if rv.IsNil() {
			return nil, nil
		}
		return normalizeRaw(rv.Bytes(), depth)
	case numberType:
		return normalizeNumber(rv.String())
	case loneSurrogateType:
		return nil, notCanonical("json.RawMessage has an unpaired surrogate escape")
	}
	// Depth counts JSON levels and pointer hops. Unwrapping an interface is
	// not a level, but a pointer hop must count: it is what bounds
	// var v any; v = &v.
	switch rv.Kind() {
	case reflect.Interface:
		if rv.IsNil() {
			return nil, nil
		}
		return normalize(rv.Elem(), depth)
	case reflect.Pointer:
		if rv.IsNil() {
			return nil, nil
		}
		return normalize(rv.Elem(), depth+1)
	case reflect.Bool:
		return rv.Bool(), nil
	case reflect.String:
		s := rv.String()
		if !utf8.ValidString(s) {
			return nil, notCanonical("string %q is not valid UTF-8", s)
		}
		return s, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := rv.Int()
		if n > maxSafeInteger || n < -maxSafeInteger {
			return nil, notCanonical("%d is beyond the safe integer range ±(2^53-1)", n)
		}
		return n, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u := rv.Uint()
		if u > maxSafeInteger {
			return nil, notCanonical("%d is beyond the safe integer range ±(2^53-1)", u)
		}
		return int64(u), nil
	case reflect.Float32, reflect.Float64:
		return checkFloat(rv.Float())
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return nil, notCanonical("map with %s keys; only string keys are JSON", rv.Type().Key())
		}
		if rv.IsNil() {
			return nil, nil
		}
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			if !utf8.ValidString(key) {
				return nil, notCanonical("key %q is not valid UTF-8", key)
			}
			if isVolatile(key) {
				continue
			}
			child, err := normalize(iter.Value(), depth+1)
			if err != nil {
				return nil, err
			}
			out[key] = child
		}
		return out, nil
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return nil, notCanonical("%s is bytes, not JSON text; pass json.RawMessage", rv.Type())
		}
		if rv.IsNil() {
			return nil, nil
		}
		return normalizeList(rv, depth)
	case reflect.Array:
		return normalizeList(rv, depth)
	case reflect.Struct:
		return nil, notCanonical("%s is a struct; marshal it with encoding/json and pass json.RawMessage", rv.Type())
	}
	return nil, notCanonical("cannot canonicalize a value of type %s", rv.Type())
}

func normalizeList(rv reflect.Value, depth int) (any, error) {
	out := make([]any, rv.Len())
	for i := range out {
		child, err := normalize(rv.Index(i), depth+1)
		if err != nil {
			return nil, err
		}
		out[i] = child
	}
	return out, nil
}

// normalizeNumber keeps an integer exact: its text is range-checked as an
// integer and never passes through float64, which is where jcs.Transform
// loses precision.
func normalizeNumber(text string) (any, error) {
	// strconv also accepts "+1", "007", ".5" and "0x1.8p1".
	if !json.Valid([]byte(text)) {
		return nil, notCanonical("%q is not a JSON number", text)
	}
	if !strings.ContainsAny(text, ".eE") {
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil || n > maxSafeInteger || n < -maxSafeInteger {
			return nil, notCanonical("%s is not an integer within the safe range ±(2^53-1)", text)
		}
		return n, nil
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, notCanonical("%s is not a finite JSON number", text)
	}
	return checkFloat(f)
}

func checkFloat(f float64) (any, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, notCanonical("%v is not a finite number", f)
	}
	// JSON.parse gives 1e16 and 10000000000000000 the same value, so every
	// binding refuses an integral number past 2^53, float or not.
	if f == math.Trunc(f) && math.Abs(f) > maxSafeInteger {
		return nil, notCanonical("%v is beyond the safe integer range ±(2^53-1)", f)
	}
	return f, nil
}

// normalizeRaw decodes JSON text with UseNumber, so integers stay exact.
func normalizeRaw(raw []byte, depth int) (any, error) {
	// encoding/json replaces invalid UTF-8 and unpaired surrogate escapes
	// with U+FFFD instead of failing. Invalid UTF-8 is refused here first;
	// surrogates are checked string by string as the text is decoded.
	if !utf8.Valid(raw) {
		return nil, notCanonical("json.RawMessage is not valid UTF-8")
	}
	d := rawDecoder{raw: raw, dec: json.NewDecoder(bytes.NewReader(raw))}
	d.dec.UseNumber()
	v, err := d.value(depth)
	if err != nil {
		return nil, err
	}
	if _, err := d.dec.Token(); err != io.EOF {
		return nil, notCanonical("json.RawMessage has data after its JSON value")
	}
	return normalize(reflect.ValueOf(v), depth)
}

// rawDecoder builds the tree encoding/json would - map[string]any, []any,
// json.Number, string, bool and nil, the last duplicate key winning - except
// that a string with an unpaired surrogate escape becomes loneSurrogate{},
// and so does an object with such a key. Checking each string where it is
// used, not the whole text up front, lets a bad escape inside a volatile
// field be stripped with the field, as in the other bindings.
type rawDecoder struct {
	raw []byte
	dec *json.Decoder
}

// token returns the next token and, for a string, whether its literal holds
// an unpaired surrogate escape.
func (d *rawDecoder) token() (json.Token, bool, error) {
	start := d.dec.InputOffset()
	tok, err := d.dec.Token()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, false, notCanonical("json.RawMessage is not valid JSON: %v", err)
	}
	if _, ok := tok.(string); !ok {
		return tok, false, nil
	}
	// The bytes since the previous token: whitespace, a colon or a comma,
	// then the quoted literal.
	literal := d.raw[start:d.dec.InputOffset()]
	literal = literal[bytes.IndexByte(literal, '"'):]
	return tok, hasUnpairedSurrogate(literal), nil
}

func (d *rawDecoder) value(depth int) (any, error) {
	if depth > maxDepth {
		return nil, notCanonical("nested deeper than %d levels, or cyclic", maxDepth)
	}
	tok, lone, err := d.token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '[':
			list := []any{}
			for d.dec.More() {
				child, err := d.value(depth + 1)
				if err != nil {
					return nil, err
				}
				list = append(list, child)
			}
			if _, _, err := d.token(); err != nil {
				return nil, err
			}
			return list, nil
		case '{':
			obj := map[string]any{}
			badKey := false
			for d.dec.More() {
				keyTok, lone, err := d.token()
				if err != nil {
					return nil, err
				}
				key, ok := keyTok.(string)
				if !ok {
					return nil, notCanonical("json.RawMessage is not valid JSON: object key %v", keyTok)
				}
				badKey = badKey || lone
				child, err := d.value(depth + 1)
				if err != nil {
					return nil, err
				}
				obj[key] = child
			}
			if _, _, err := d.token(); err != nil {
				return nil, err
			}
			if badKey {
				return loneSurrogate{}, nil
			}
			return obj, nil
		}
		return nil, notCanonical("json.RawMessage is not valid JSON: unexpected %v", t)
	case string:
		if lone {
			return loneSurrogate{}, nil
		}
		return t, nil
	}
	return tok, nil // json.Number, bool or nil
}

// hasUnpairedSurrogate reports whether a JSON string literal holds a
// \uD800-\uDFFF escape that is not half of a high-low pair. Malformed
// escapes are left for the decoder to report.
func hasUnpairedSurrogate(raw []byte) bool {
	inString, pendingHigh := false, false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if !inString {
			inString = c == '"'
			continue
		}
		if c != '\\' {
			if pendingHigh {
				return true
			}
			inString = c != '"'
			continue
		}
		if i+1 >= len(raw) {
			return false
		}
		i++
		if raw[i] != 'u' {
			if pendingHigh {
				return true
			}
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		unit, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		switch {
		case unit >= 0xD800 && unit <= 0xDBFF:
			if pendingHigh {
				return true
			}
			pendingHigh = true
		case unit >= 0xDC00 && unit <= 0xDFFF:
			if !pendingHigh {
				return true
			}
			pendingHigh = false
		default:
			if pendingHigh {
				return true
			}
		}
	}
	return false
}

// CanonicalJSON returns the exact string that gets hashed: RFC 8785 after
// volatile fields are stripped. It is exported so a port can diff characters
// instead of guessing why two hashes disagree.
func CanonicalJSON(v any) (s string, err error) {
	defer func() {
		if r := recover(); r != nil {
			s, err = "", notCanonical("panic while canonicalizing: %v", r)
		}
	}()
	stripped, err := StripVolatile(v)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := serialize(&b, stripped); err != nil {
		return "", err
	}
	return b.String(), nil
}

// CanonicalFP returns the fixed-length fingerprint of v, volatile fields
// removed.
func CanonicalFP(v any) (string, error) {
	s, err := CanonicalJSON(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:FPLength], nil
}

func serialize(b *strings.Builder, v any) error {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case string:
		writeString(b, x)
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case float64:
		s, err := jcs.NumberToJSON(x)
		if err != nil {
			return notCanonical("%v: %v", x, err)
		}
		b.WriteString(s)
	case []any:
		b.WriteByte('[')
		for i, child := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := serialize(b, child); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		type member struct {
			key   string
			units []uint16
		}
		members := make([]member, 0, len(x))
		for key := range x {
			members = append(members, member{key, utf16.Encode([]rune(key))})
		}
		// RFC 8785 sorts by UTF-16 code units. Go strings are UTF-8, and
		// byte order would put U+FB01 before U+1F600.
		slices.SortFunc(members, func(a, b member) int { return slices.Compare(a.units, b.units) })
		b.WriteByte('{')
		for i, m := range members {
			if i > 0 {
				b.WriteByte(',')
			}
			writeString(b, m.key)
			b.WriteByte(':')
			if err := serialize(b, x[m.key]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		return notCanonical("cannot serialize %T", v)
	}
	return nil
}

// writeString writes s as ECMAScript's JSON.stringify does: only the quote,
// the backslash and C0 controls are escaped; <, >, & and all other
// characters are written raw.
func writeString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}
