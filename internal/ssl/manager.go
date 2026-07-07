package ssl

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

type Mode string

const (
	ModeNone        Mode = "none"
	ModeLetsEncrypt Mode = "letsencrypt"
	ModeCustom      Mode = "custom"
)

type CertPaths struct {
	CertFile string
	KeyFile  string
}

// EnableLetsEncrypt obtiene un certificado Let's Encrypt para el dominio
// y lo guarda en el directorio de certs del producto.
func EnableLetsEncrypt(domain, certsDir string, logFn func(line, level string)) (*CertPaths, error) {
	if err := os.MkdirAll(certsDir, 0700); err != nil {
		return nil, err
	}

	logFn(fmt.Sprintf("Solicitando certificado para %s...", domain), "info")

	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(domain),
		Cache:      autocert.DirCache(certsDir),
		Client:     &acme.Client{DirectoryURL: acme.LetsEncryptURL},
	}

	// Servidor temporal en puerto 80 para el challenge HTTP-01
	srv := &http.Server{
		Addr:    ":80",
		Handler: m.HTTPHandler(nil),
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Intentar obtener el certificado
	tlsCfg := m.TLSConfig()
	_, err := tlsCfg.GetCertificate(&tls_hello{ServerName: domain})
	srv.Close()

	if err != nil {
		select {
		case srvErr := <-errCh:
			return nil, fmt.Errorf("error en servidor de challenge: %w", srvErr)
		default:
		}
		return nil, fmt.Errorf("no se pudo obtener el certificado: %w", err)
	}

	certFile := filepath.Join(certsDir, domain+".crt")
	keyFile := filepath.Join(certsDir, domain+".key")

	logFn("✓ Certificado Let's Encrypt obtenido correctamente", "success")
	return &CertPaths{CertFile: certFile, KeyFile: keyFile}, nil
}

// SaveCustomCert guarda un certificado y clave personalizados.
func SaveCustomCert(certsDir, domain string, certReader, keyReader io.Reader) (*CertPaths, error) {
	if err := os.MkdirAll(certsDir, 0700); err != nil {
		return nil, err
	}

	certFile := filepath.Join(certsDir, domain+".crt")
	keyFile := filepath.Join(certsDir, domain+".key")

	certData, err := io.ReadAll(certReader)
	if err != nil {
		return nil, err
	}
	keyData, err := io.ReadAll(keyReader)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(certFile, certData, 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyFile, keyData, 0600); err != nil {
		return nil, err
	}

	return &CertPaths{CertFile: certFile, KeyFile: keyFile}, nil
}

// GenerateSelfSigned genera un certificado autofirmado para uso sin dominio.
func GenerateSelfSigned(certsDir, ip string) (*CertPaths, error) {
	if err := os.MkdirAll(certsDir, 0700); err != nil {
		return nil, err
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject:      pkix.Name{Organization: []string{"LominoDeploy Self-Signed"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	if ip != "" {
		template.IPAddresses = []interface{}{ip} // simplificado
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}

	certFile := filepath.Join(certsDir, "self-signed.crt")
	keyFile := filepath.Join(certsDir, "self-signed.key")

	cf, _ := os.Create(certFile)
	pem.Encode(cf, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	cf.Close()

	kf, _ := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	kb, _ := x509.MarshalECPrivateKey(priv)
	pem.Encode(kf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: kb})
	kf.Close()

	return &CertPaths{CertFile: certFile, KeyFile: keyFile}, nil
}

// tls_hello es un helper interno para triggear GetCertificate.
type tls_hello struct {
	ServerName string
}

func (h *tls_hello) GetServerName() string { return h.ServerName }
