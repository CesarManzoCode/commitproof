// Package canonjson encodes the canonical JSON of CONTRACT §6.1: object keys
// sorted lexicographically (bytewise on UTF-8), no insignificant whitespace,
// integers only (no floats), strings in Unicode NFC.
//
// Accepted values: nil, bool, string, all Go integer types, json.Number that
// holds an integer, []any, map[string]any, []string, []int64, []int,
// [][]any, and RawMessage (already-canonical bytes, re-validated). Anything
// else is an error, so a float or a struct can never slip into a history.
package canonjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// RawMessage is canonical JSON produced elsewhere (e.g. a decoded plan
// fragment). Marshal re-canonicalizes it.
type RawMessage []byte

// Marshal returns the canonical encoding of v.
func Marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := encode(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// MustMarshal is Marshal for values built from literals in this repository;
// it panics on programmer error.
func MustMarshal(v any) []byte {
	out, err := Marshal(v)
	if err != nil {
		panic(err)
	}
	return out
}

// Canonicalize parses arbitrary JSON (numbers must be integers) and returns
// its canonical form.
func Canonicalize(in []byte) ([]byte, error) {
	v, err := Decode(in)
	if err != nil {
		return nil, err
	}
	return Marshal(v)
}

// Decode parses JSON into map[string]any / []any / string / bool / nil /
// json.Number, rejecting trailing data.
func Decode(in []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(in))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("canonjson: decode: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("canonjson: trailing data after JSON value")
	}
	return v, nil
}

// IsCanonical reports whether line is exactly the canonical encoding of the
// value it holds.
func IsCanonical(line []byte) bool {
	c, err := Canonicalize(line)
	return err == nil && bytes.Equal(c, line)
}

func encode(b *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		writeString(b, x)
	case int:
		b.WriteString(strconv.FormatInt(int64(x), 10))
	case int8:
		b.WriteString(strconv.FormatInt(int64(x), 10))
	case int16:
		b.WriteString(strconv.FormatInt(int64(x), 10))
	case int32:
		b.WriteString(strconv.FormatInt(int64(x), 10))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case uint:
		b.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint8:
		b.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint16:
		b.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint32:
		b.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint64:
		b.WriteString(strconv.FormatUint(x, 10))
	case json.Number:
		n, ok := new(big.Int).SetString(string(x), 10)
		if !ok {
			return fmt.Errorf("canonjson: non-integer number %q", string(x))
		}
		b.WriteString(n.String())
	case RawMessage:
		inner, err := Canonicalize(x)
		if err != nil {
			return err
		}
		b.Write(inner)
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := encode(b, e); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case [][]any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := encode(b, e); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case []string:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			writeString(b, e)
		}
		b.WriteByte(']')
	case []int64:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.FormatInt(e, 10))
		}
		b.WriteByte(']')
	case []int:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.Itoa(e))
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		// Keys are NFC-normalized before sorting so the order is that of the
		// encoded bytes.
		type kv struct {
			k string
			v any
		}
		kvs := make([]kv, 0, len(keys))
		seen := make(map[string]bool, len(keys))
		for _, k := range keys {
			nk := norm.NFC.String(toValidUTF8(k))
			if seen[nk] {
				return fmt.Errorf("canonjson: duplicate key after NFC: %q", nk)
			}
			seen[nk] = true
			kvs = append(kvs, kv{nk, x[k]})
		}
		sort.Slice(kvs, func(i, j int) bool { return kvs[i].k < kvs[j].k })
		b.WriteByte('{')
		for i, e := range kvs {
			if i > 0 {
				b.WriteByte(',')
			}
			writeRawString(b, e.k)
			b.WriteByte(':')
			if err := encode(b, e.v); err != nil {
				return fmt.Errorf("%s: %w", e.k, err)
			}
		}
		b.WriteByte('}')
	case float32, float64:
		return fmt.Errorf("canonjson: floats are not allowed (%v)", x)
	default:
		return fmt.Errorf("canonjson: unsupported type %T", v)
	}
	return nil
}

func toValidUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return string([]rune(s)) // invalid bytes become U+FFFD
}

func writeString(b *bytes.Buffer, s string) {
	writeRawString(b, norm.NFC.String(toValidUTF8(s)))
}

// writeRawString escapes only what JSON requires: '"', '\\' and control
// characters below U+0020. Everything else is written as UTF-8.
func writeRawString(b *bytes.Buffer, s string) {
	const hex = "0123456789abcdef"
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			b.WriteString(`\"`)
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c == '\b':
			b.WriteString(`\b`)
		case c == '\f':
			b.WriteString(`\f`)
		case c < 0x20:
			b.WriteString(`\u00`)
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xf])
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
}
