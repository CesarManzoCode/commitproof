package runner

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// NodeUID is the postgres user of the pgctld image (postgres:17.7). Both
// processes of a node (pgctld and multipooler) run as this user, as the
// containers of one Kubernetes pod share /data (ADR-0021).
const NodeUID = 999

// InitCerts translates demo/k8s/k8s-generate-certs-job.yaml: a self-signed
// RSA-4096 root CA "Multigres Root CA" (3650 days) signs an RSA-2048
// certificate CN=pgbackrest (3650 days, keyUsage critical digitalSignature +
// keyEncipherment, extKeyUsage serverAuth + clientAuth); the CA key is
// discarded. SANs: the demo's pgbackrest, localhost, 127.0.0.1, plus the
// Compose host names of every node in place of the Kubernetes wildcard
// *.multipooler-zone1.default.svc.cluster.local (DERIVATION.md).
//
// Files: ca.crt, pgbackrest.crt (0644) and pgbackrest.key (0600, as in the
// demo), owned by the node user so pgctld's pgBackRest server and the
// pooler's pgBackRest client can read it (pgBackRest refuses a key with
// group or other permissions unless it is owned by root).
func InitCerts(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "pgbackrest.key")); err == nil {
		return fmt.Errorf("init: %s already holds certificates (a cluster's volumes were not removed)", dir)
	}
	now := time.Now()
	caKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}
	serial := func() *big.Int {
		n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
		return n
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "Multigres Root CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.AddDate(0, 0, 3650),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		return err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	dns := []string{"pgbackrest", "localhost"}
	for _, n := range []string{"n1", "n2", "n3", "n4"} {
		dns = append(dns, "pg-"+n, "pooler-"+n)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: "pgbackrest"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.AddDate(0, 0, 3650),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     dns,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return err
	}
	if _, err := x509.ParseCertificate(leafDER); err != nil {
		return err
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	leaf, _ := x509.ParseCertificate(leafDER)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); err != nil {
		return fmt.Errorf("init: certificate chain does not verify: %w", err)
	}
	write := func(name string, block *pem.Block, mode os.FileMode, uid, gid int) error {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, pem.EncodeToMemory(block), mode); err != nil {
			return err
		}
		if err := os.Chmod(p, mode); err != nil {
			return err
		}
		return os.Chown(p, uid, gid)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	if err := write("ca.crt", &pem.Block{Type: "CERTIFICATE", Bytes: caDER}, 0o644, 0, 0); err != nil {
		return err
	}
	if err := write("pgbackrest.crt", &pem.Block{Type: "CERTIFICATE", Bytes: leafDER}, 0o644, 0, 0); err != nil {
		return err
	}
	if err := write("pgbackrest.key", &pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}, 0o600, NodeUID, NodeUID); err != nil {
		return err
	}
	return os.Chmod(dir, 0o755)
}

// InitVolume gives a shared volume (data-nX, backups) to the node user, as
// the demo's hostPath directories are writable by the pod's processes.
func InitVolume(dir string) error {
	if err := os.Chown(dir, NodeUID, NodeUID); err != nil {
		return err
	}
	return os.Chmod(dir, 0o755)
}
