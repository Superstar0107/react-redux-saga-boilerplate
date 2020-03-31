// Copyright 2020 The benchmark. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"
)

var (
	bodyHeaderSepBytes    = []byte{13, 10, 13, 10}
	bodyHeaderSepBytesLen = 4
	headerSepBytes        = []byte{13, 10}
	contentLengthBytes    = []byte{67, 111, 110, 116, 101, 110, 116, 45, 76, 101, 110, 103, 116, 104, 58, 32}
	contentLengthBytesLen = 16
)
// ReqConn is used to create a connection and record data
type ReqConn struct {
	ErrorTimes int
	Count      int64
	NowNum     *int64
	timeout    int
	writeBytes []byte
	writeLen   int
	readLen    int
	reqTimes   []int
	conn       net.Conn
	remoteAddr string
	schema     string
	buf        []byte
	dialer     ProxyConn
}

// Connect to the server, http and socks5 proxy support
// If the target is https, convert connection to tls client
func (rc *ReqConn) dial() error {
	if rc.conn != nil {
		rc.conn.Close()
	}
	conn, err := rc.dialer.Dial("tcp", rc.remoteAddr, time.Millisecond*time.Duration(rc.timeout))
	if err != nil {
		return err
	}
	rc.conn = conn
	if rc.schema == "https" {
		conf := &tls.Config{
			InsecureSkipVerify: true,
		}
		rc.conn = tls.Client(rc.conn, conf)
	}
	return nil
}

// Start a connection, send request to server and read response from server
func (rc *ReqConn) Start() (err error) {
	var contentLen string
	var bodyHasRead int
	var headerHasRead int
	var n int
	var reqTime time.Time
re:
	if err != nil && err != io.EOF && !strings.Contains(err.Error(), "connection reset by peer") {
		rc.ErrorTimes += 1
	}
	if err = rc.dial(); err != nil {
		return
	}
	for {
		bodyHasRead = 0
		headerHasRead = 0
		reqTime = time.Now()
		n, err = rc.conn.Write(rc.writeBytes)
		if err != nil {
			goto re
		}
		rc.writeLen += n
	readHeader:
		rc.conn.SetReadDeadline(time.Now().Add(time.Millisecond * time.Duration(rc.timeout)))
		n, err = rc.conn.Read(rc.buf[headerHasRead:])
		if err != nil {
			goto re
		}
		headerHasRead += n
		rc.readLen += n
		var bbArr [2][]byte
		bodyPos := bytes.Index(rc.buf[:headerHasRead], bodyHeaderSepBytes)
		if bodyPos > -1 {
			bbArr[0] = rc.buf[:bodyPos]
			bbArr[1] = rc.buf[bodyPos+bodyHeaderSepBytesLen:]
		} else {
			goto readHeader
		}
		n := bytes.Index(bbArr[0], contentLengthBytes)
		start := n + contentLengthBytesLen
		end := bytes.Index(bbArr[0][start:], headerSepBytes)
		if end == -1 {
			contentLen = Bytes2str(bbArr[0][start:])
		} else {
			contentLen = Bytes2str(bbArr[0][start : start+end])
		}
		contentLenI, _ := strconv.Atoi(contentLen)
		bodyHasRead += len(bbArr[1])
		for {
			if bodyHasRead >= contentLenI {
				break
			}
			n, err = rc.conn.Read(rc.buf)
			if err != nil {
				goto re
			}
			rc.readLen += n
			bodyHasRead += n
		}
		rc.reqTimes = append(rc.reqTimes, int(time.Now().Sub(reqTime).Milliseconds()))
		if atomic.AddInt64(rc.NowNum, 1) >= rc.Count {
			return
		}
	}
}

// Convert bytes to strings
func Bytes2str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
