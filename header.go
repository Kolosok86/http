// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kolosok86/http/httptrace"
	"github.com/Kolosok86/http/internal/ascii"
	"github.com/Kolosok86/http/textproto"
	"golang.org/x/net/http/httpguts"
)

// A Header represents the Key-value pairs in an HTTP header.
//
// The keys should be in canonical form, as returned by
// CanonicalHeaderKey.
type Header map[string][]string

// Add adds the Key, value pair to the header.
// It appends to any existing Values associated with Key.
// The Key is case insensitive; it is canonicalized by
// CanonicalHeaderKey.
func (h Header) Add(key, value string) {
	textproto.MIMEHeader(h).Add(key, value)
}

// Set sets the header entries associated with Key to the
// single element value. It replaces any existing Values
// associated with Key. The Key is case insensitive; it is
// canonicalized by textproto.CanonicalMIMEHeaderKey.
// To use non-canonical keys, assign to the map directly.
func (h Header) Set(key, value string) {
	textproto.MIMEHeader(h).Set(key, value)
}

// Get gets the first value associated with the given Key. If
// there are no Values associated with the Key, Get returns "".
// It is case insensitive; textproto.CanonicalMIMEHeaderKey is
// used to canonicalize the provided Key. Get assumes that all
// keys are stored in canonical form. To use non-canonical keys,
// access the map directly.
func (h Header) Get(key string) string {
	return textproto.MIMEHeader(h).Get(key)
}

// Values returns all Values associated with the given Key.
// It is case insensitive; textproto.CanonicalMIMEHeaderKey is
// used to canonicalize the provided Key. To use non-canonical
// keys, access the map directly.
// The returned slice is not a copy.
func (h Header) Values(key string) []string {
	return textproto.MIMEHeader(h).Values(key)
}

// get is like Get, but Key must already be in CanonicalHeaderKey form.
func (h Header) get(key string) string {
	if v := h[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// has reports whether h has the provided Key defined, even if it's
// set to 0-length slice.
func (h Header) has(key string) bool {
	_, ok := h[key]
	return ok
}

// Del deletes the Values associated with Key.
// The Key is case insensitive; it is canonicalized by
// CanonicalHeaderKey.
func (h Header) Del(key string) {
	textproto.MIMEHeader(h).Del(key)
}

// Write writes a header in wire format.
func (h Header) Write(w io.Writer) error {
	return h.write(w, nil, textproto.HeaderOrder{})
}

func (h Header) write(w io.Writer, trace *httptrace.ClientTrace, order textproto.HeaderOrder) error {
	return h.writeSubset(w, nil, order, trace)
}

// Clone returns a copy of h or nil if h is nil.
func (h Header) Clone() Header {
	if h == nil {
		return nil
	}

	// Find total number of Values.
	nv := 0
	for _, vv := range h {
		nv += len(vv)
	}
	sv := make([]string, nv) // shared backing array for headers' Values
	h2 := make(Header, len(h))
	for k, vv := range h {
		if vv == nil {
			// Preserve nil Values. ReverseProxy distinguishes
			// between nil and zero-length header Values.
			h2[k] = nil
			continue
		}
		n := copy(sv, vv)
		h2[k] = sv[:n:n]
		sv = sv[n:]
	}
	return h2
}

var timeFormats = []string{
	TimeFormat,
	time.RFC850,
	time.ANSIC,
}

// ParseTime parses a time header (such as the Date: header),
// trying each of the three formats allowed by HTTP/1.1:
// TimeFormat, time.RFC850, and time.ANSIC.
func ParseTime(text string) (t time.Time, err error) {
	for _, layout := range timeFormats {
		t, err = time.Parse(layout, text)
		if err == nil {
			return
		}
	}
	return
}

var headerNewlineToSpace = strings.NewReplacer("\n", " ", "\r", " ")

// stringWriter implements WriteString on a Writer.
type stringWriter struct {
	w io.Writer
}

func (w stringWriter) WriteString(s string) (n int, err error) {
	return w.w.Write([]byte(s))
}

type keyValues struct {
	Key    string
	Values []string
}

// A headerSorter implements sort.Interface by sorting a []keyValues
// by Key. It's used as a pointer, so it can fit in a sort.Interface
// interface value without allocation.
type headerSorter struct {
	order textproto.HeaderOrder
	kvs   []keyValues
}

func (s *headerSorter) Len() int      { return len(s.kvs) }
func (s *headerSorter) Swap(i, j int) { s.kvs[i], s.kvs[j] = s.kvs[j], s.kvs[i] }
func (s *headerSorter) Less(i, j int) bool {
	// If the order isn't defined, sort lexicographically.
	if len(s.order.Order) == 0 {
		return s.kvs[i].Key < s.kvs[j].Key
	}

	ii := s.order.FindIndex(strings.ToLower(s.kvs[i].Key))
	ji := s.order.FindIndex(strings.ToLower(s.kvs[j].Key))

	if ii == -1 && ji == -1 {
		return s.kvs[i].Key < s.kvs[j].Key
	} else if ii == -1 && ji > -1 {
		return false
	} else if ji == -1 && ii > -1 {
		return true
	}

	return ii < ji
}

var headerSorterPool = sync.Pool{
	New: func() any { return new(headerSorter) },
}

// SortedKeyValues returns h's keys sorted in the returned kvs
// slice. The headerSorter used to sort is also returned, for possible
// return to headerSorterCache.
func (h Header) SortedKeyValues(exclude map[string]bool, order textproto.HeaderOrder) (kvs []keyValues, hs *headerSorter) {
	hs = headerSorterPool.Get().(*headerSorter)
	if cap(hs.kvs) < len(h) {
		hs.kvs = make([]keyValues, 0, len(h))
	}
	kvs = hs.kvs[:0]
	for k, vv := range h {
		if !exclude[k] {
			kvs = append(kvs, keyValues{k, vv})
		}
	}
	hs.order = order
	hs.kvs = kvs
	sort.Sort(hs)
	return kvs, hs
}

// WriteSubset writes a header in wire format.
// If exclude is not nil, keys where exclude[Key] == true are not written.
// Keys are not canonicalized before checking the exclude map.
func (h Header) WriteSubset(w io.Writer, exclude map[string]bool) error {
	return h.writeSubset(w, exclude, textproto.HeaderOrder{}, nil)
}

func (h Header) writeSubset(w io.Writer, exclude map[string]bool, order textproto.HeaderOrder, trace *httptrace.ClientTrace) error {
	ws, ok := w.(io.StringWriter)
	if !ok {
		ws = stringWriter{w}
	}
	kvs, sorter := h.SortedKeyValues(exclude, order)
	var formattedVals []string
	for _, kv := range kvs {
		if !httpguts.ValidHeaderFieldName(kv.Key) {
			// This could be an error. In the common case of
			// writing response headers, however, we have no good
			// way to provide the error back to the server
			// handler, so just drop invalid headers instead.
			continue
		}
		for _, v := range kv.Values {
			v = headerNewlineToSpace.Replace(v)
			v = textproto.TrimString(v)
			for _, s := range []string{kv.Key, ": ", v, "\r\n"} {
				if _, err := ws.WriteString(s); err != nil {
					headerSorterPool.Put(sorter)
					return err
				}
			}
			if trace != nil && trace.WroteHeaderField != nil {
				formattedVals = append(formattedVals, v)
			}
		}
		if trace != nil && trace.WroteHeaderField != nil {
			trace.WroteHeaderField(kv.Key, formattedVals)
			formattedVals = nil
		}
	}
	headerSorterPool.Put(sorter)
	return nil
}

// CanonicalHeaderKey returns the canonical format of the
// header Key s. The canonicalization converts the first
// letter and any letter following a hyphen to upper case;
// the rest are converted to lowercase. For example, the
// canonical Key for "accept-encoding" is "Accept-Encoding".
// If s contains a space or invalid header field bytes, it is
// returned without modifications.
func CanonicalHeaderKey(s string) string { return textproto.CanonicalMIMEHeaderKey(s) }

// hasToken reports whether token appears with v, ASCII
// case-insensitive, with space or comma boundaries.
// token must be all lowercase.
// v may contain mixed cased.
func hasToken(v, token string) bool {
	if len(token) > len(v) || token == "" {
		return false
	}
	if v == token {
		return true
	}
	for sp := 0; sp <= len(v)-len(token); sp++ {
		// Check that first character is good.
		// The token is ASCII, so checking only a single byte
		// is sufficient. We skip this potential starting
		// position if both the first byte and its potential
		// ASCII uppercase equivalent (b|0x20) don't match.
		// False positives ('^' => '~') are caught by EqualFold.
		if b := v[sp]; b != token[0] && b|0x20 != token[0] {
			continue
		}
		// Check that start pos is on a valid token boundary.
		if sp > 0 && !isTokenBoundary(v[sp-1]) {
			continue
		}
		// Check that end pos is on a valid token boundary.
		if endPos := sp + len(token); endPos != len(v) && !isTokenBoundary(v[endPos]) {
			continue
		}
		if ascii.EqualFold(v[sp:sp+len(token)], token) {
			return true
		}
	}
	return false
}

func isTokenBoundary(b byte) bool {
	return b == ' ' || b == ',' || b == '\t'
}
