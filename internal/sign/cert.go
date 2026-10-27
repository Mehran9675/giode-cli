package sign

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

// SelfSigned writes a self-signed Authenticode code signing certificate
// and its key to path as a password-protected .pfx file. Windows doesn't
// trust it - SmartScreen and installers still warn - so it's for
// testing the signing setup, or for machines that install it as
// trusted (a company's own computers).
func SelfSigned(path, subject, password string, years int) error {
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: subject},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(years, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return err
	}
	pfx, err := pkcs12.Modern.Encode(key, cert, nil, password)
	if err != nil {
		return err
	}
	return os.WriteFile(path, pfx, 0o600)
}
