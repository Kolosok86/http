// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package multipart

import (
	_ "unsafe" // for go:linkname

	"github.com/Kolosok86/http/textproto"
)

// readMIMEHeader is defined in package net/textproto.
//
//go:linkname readMIMEHeader net/textproto.readMIMEHeader
func readMIMEHeader(r *textproto.Reader, lim int64) (textproto.MIMEHeader, error)
