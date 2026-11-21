package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TypeTerrors/go_sync/conf"
	pb "github.com/TypeTerrors/go_sync/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type tlsService struct {
	pb.UnimplementedFileSyncServiceServer
}

func (tlsService) ListFiles(context.Context, *pb.Empty) (*pb.FileList, error) {
	return &pb.FileList{}, nil
}
func TestMutualTLS(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"gosync"}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalPKCS8PrivateKey(leafKey)
	dir := t.TempDir()
	cfg := conf.Config{TLSCert: filepath.Join(dir, "cert.pem"), TLSKey: filepath.Join(dir, "key.pem"), TLSCA: filepath.Join(dir, "ca.pem")}
	for file, block := range map[string]*pem.Block{cfg.TLSCert: {Type: "CERTIFICATE", Bytes: leafDER}, cfg.TLSKey: {Type: "PRIVATE KEY", Bytes: keyDER}, cfg.TLSCA: {Type: "CERTIFICATE", Bytes: caDER}} {
		if err := os.WriteFile(file, pem.EncodeToMemory(block), 0600); err != nil {
			t.Fatal(err)
		}
	}
	serverCreds, clientCreds, err := transport(cfg)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(grpc.Creds(serverCreds))
	pb.RegisterFileSyncServiceServer(server, tlsService{})
	go server.Serve(listener)
	defer server.Stop()
	client, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(clientCreds))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err = pb.NewFileSyncServiceClient(client).ListFiles(ctx, &pb.Empty{}); err != nil {
		t.Fatalf("authenticated request: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}))
	unauth, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: "gosync"})))
	if err != nil {
		t.Fatal(err)
	}
	defer unauth.Close()
	if _, err = pb.NewFileSyncServiceClient(unauth).ListFiles(ctx, &pb.Empty{}); err == nil {
		t.Fatal("accepted peer without client certificate")
	}
}
func TestTransportRequiresExplicitChoice(t *testing.T) {
	if _, _, err := transport(conf.Config{}); err == nil {
		t.Fatal("accepted implicit plaintext")
	}
	if _, _, err := transport(conf.Config{Insecure: true, TLSCert: "cert"}); err == nil {
		t.Fatal("accepted ambiguous security flags")
	}
	if _, _, err := transport(conf.Config{Insecure: true}); err != nil {
		t.Fatal(err)
	}
}
