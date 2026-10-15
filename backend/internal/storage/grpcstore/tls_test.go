package grpcstore

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func testCertificate(t *testing.T, parent *x509.Certificate, parentKey *ecdsa.PrivateKey,
	serial int64, isCA bool, usage x509.ExtKeyUsage) (*x509.Certificate, *ecdsa.PrivateKey, []byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: fmt.Sprintf("storage-test-%d", serial)},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		BasicConstraintsValid: true, IsCA: isCA,
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{usage},
	}
	if isCA {
		template.KeyUsage |= x509.KeyUsageCertSign
		template.ExtKeyUsage = nil
	}
	if usage == x509.ExtKeyUsageServerAuth {
		template.DNSNames = []string{"localhost"}
	}
	if parent == nil {
		parent, parentKey = template, key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return certificate, key,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateDER})
}

func TestMutualTLSHealthCheck(t *testing.T) {
	ca, caKey, caPEM, _ := testCertificate(t, nil, nil, 1, true, x509.ExtKeyUsageAny)
	_, _, serverPEM, serverKey := testCertificate(t, ca, caKey, 2, false, x509.ExtKeyUsageServerAuth)
	_, _, clientPEM, clientKey := testCertificate(t, ca, caKey, 3, false, x509.ExtKeyUsageClientAuth)
	serverPair, err := tls.X509KeyPair(serverPEM, serverKey)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		t.Fatal("invalid test CA")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{serverPair},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots,
	})))
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(server, healthServer)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })

	directory := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	caPath := write("ca.pem", caPEM)
	certPath := write("client.pem", clientPEM)
	keyPath := write("client.key", clientKey)
	store, err := NewWithTLS(listener.Addr().String(), 2*time.Second, TLSOptions{
		CAFile: caPath, ServerName: "localhost", ClientCertFile: certPath, ClientKeyFile: keyPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Check(context.Background()); err != nil {
		t.Fatalf("mTLS health check: %v", err)
	}
	unauthorized, err := NewWithTLS(listener.Addr().String(), time.Second, TLSOptions{CAFile: caPath, ServerName: "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	defer unauthorized.Close()
	if err := unauthorized.Check(context.Background()); err == nil {
		t.Fatal("server accepted missing client certificate")
	}
}
