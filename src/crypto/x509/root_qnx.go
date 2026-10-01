// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package x509

// QNX 6.5 ships OpenSSL with its directory at /etc/openssl; these are
// OpenSSL's default names under it. QNX installs no CA certificates, so
// they are typically provided with SSL_CERT_FILE or SSL_CERT_DIR.
var certFiles = []string{
	"/etc/openssl/cert.pem",
}

var certDirectories = []string{
	"/etc/openssl/certs",
}
