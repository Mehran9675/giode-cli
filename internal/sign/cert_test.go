package sign

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"

	"software.sslmate.com/src/go-pkcs12"
)

func TestSelfSigned(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.pfx")
	if err := SelfSigned(p, "Acme Ltd", "secret", 2); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	key, cert, err := pkcs12.Decode(data, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if key == nil || cert.Subject.CommonName != "Acme Ltd" {
		t.Errorf("subject %q", cert.Subject.CommonName)
	}
	if len(cert.ExtKeyUsage) != 1 || cert.ExtKeyUsage[0] != x509.ExtKeyUsageCodeSigning {
		t.Errorf("extended key usage %v, want code signing", cert.ExtKeyUsage)
	}
	if _, _, err := pkcs12.Decode(data, "wrong"); err == nil {
		t.Error("decoded with the wrong password")
	}
}
