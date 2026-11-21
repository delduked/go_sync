package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

func trustCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("use trust init or trust issue")
	}
	flags := flag.NewFlagSet("trust "+args[0], flag.ContinueOnError)
	out := flags.String("out", "", "New private output directory")
	authority := flags.String("authority", "", "Authority directory (issue only)")
	name := flags.String("name", "", "Peer name (issue only)")
	days := flags.Int("days", 365, "Peer certificate lifetime in days")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *out == "" || flags.NArg() != 0 || *days < 1 || *days > 3650 {
		return fmt.Errorf("provide --out and a lifetime of 1–3650 days")
	}
	switch args[0] {
	case "init":
		return createAuthority(*out)
	case "issue":
		if *authority == "" || *name == "" {
			return fmt.Errorf("issue requires --authority and --name")
		}
		return issuePeer(*authority, *out, *name, *days)
	default:
		return fmt.Errorf("unknown trust command %q", args[0])
	}
}
func newCertificate(name string, days int) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	return &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: name}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.Add(time.Duration(days) * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}, key, nil
}
func writeIdentity(dir string, files map[string][]byte) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return err
	}
	// Existing directories are never reused, avoiding accidental key replacement.
	if err := os.Mkdir(dir, 0700); err != nil {
		return err
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			return err
		}
	}
	return nil
}
func privatePEM(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}
func createAuthority(out string) error {
	cert, key, err := newCertificate("GoSync private authority", 3650)
	if err != nil {
		return err
	}
	cert.IsCA = true
	cert.BasicConstraintsValid = true
	cert.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	cert.MaxPathLenZero = true
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		return err
	}
	secret, err := privatePEM(key)
	if err != nil {
		return err
	}
	if err = writeIdentity(out, map[string][]byte{"ca.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), "ca-key.pem": secret}); err != nil {
		return err
	}
	fmt.Printf("Created authority %s; CA SHA-256 %x\n", out, sha256.Sum256(der))
	return nil
}
func issuePeer(authority, out, name string, days int) error {
	public, err := os.ReadFile(filepath.Join(authority, "ca.pem"))
	if err != nil {
		return err
	}
	block, _ := pem.Decode(public)
	if block == nil {
		return fmt.Errorf("invalid authority certificate")
	}
	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}
	private, err := os.ReadFile(filepath.Join(authority, "ca-key.pem"))
	if err != nil {
		return err
	}
	block, _ = pem.Decode(private)
	if block == nil {
		return fmt.Errorf("invalid authority key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	caKey, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || !ca.IsCA {
		return fmt.Errorf("unsupported authority key or certificate")
	}
	cert, key, err := newCertificate(name, days)
	if err != nil {
		return err
	}
	if !ca.NotAfter.After(time.Now()) {
		return fmt.Errorf("authority expired")
	}
	if cert.NotAfter.After(ca.NotAfter) {
		cert.NotAfter = ca.NotAfter
	}
	cert.DNSNames = []string{"gosync"}
	cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}
	der, err := x509.CreateCertificate(rand.Reader, cert, ca, &key.PublicKey, caKey)
	if err != nil {
		return err
	}
	secret, err := privatePEM(key)
	if err != nil {
		return err
	}
	if err = writeIdentity(out, map[string][]byte{"ca.pem": public, "peer.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), "peer-key.pem": secret}); err != nil {
		return err
	}
	fmt.Printf("Created peer %s at %s; expires %s\n", name, out, cert.NotAfter.Format(time.RFC3339))
	return nil
}
