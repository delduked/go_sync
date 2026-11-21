package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestProvisionDistinctTrustedPeersWithoutReplacingKeys(t *testing.T) {
	base := t.TempDir()
	authority := filepath.Join(base, "authority")
	if err := createAuthority(authority); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"host-a", "host-b"} {
		dir := filepath.Join(base, name)
		if err := issuePeer(authority, dir, name, 30); err != nil {
			t.Fatal(err)
		}
		cert, err := tls.LoadX509KeyPair(filepath.Join(dir, "peer.pem"), filepath.Join(dir, "peer-key.pem"))
		if err != nil {
			t.Fatal(err)
		}
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			t.Fatal(err)
		}
		ca, err := os.ReadFile(filepath.Join(dir, "ca.pem"))
		if err != nil {
			t.Fatal(err)
		}
		roots := x509.NewCertPool()
		roots.AppendCertsFromPEM(ca)
		for _, usage := range []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth} {
			if _, err = leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "gosync", KeyUsages: []x509.ExtKeyUsage{usage}}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = os.Stat(filepath.Join(dir, "ca-key.pem")); !os.IsNotExist(err) {
			t.Fatal("distributed CA private key")
		}
		before, _ := os.ReadFile(filepath.Join(dir, "peer-key.pem"))
		if err = issuePeer(authority, dir, name, 30); err == nil {
			t.Fatal("replaced existing identity")
		}
		after, _ := os.ReadFile(filepath.Join(dir, "peer-key.pem"))
		if string(before) != string(after) {
			t.Fatal("changed existing key")
		}
		block, _ := pem.Decode(before)
		if block == nil {
			t.Fatal("invalid private key")
		}
	}
	if err := createAuthority(authority); err == nil {
		t.Fatal("replaced authority")
	}
}
