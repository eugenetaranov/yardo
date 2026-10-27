package steps

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
)

func testCert(t *testing.T, cn string) []byte {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn}, NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestCorpCA(t *testing.T) {
	x := &fakeExec{}
	e := testEnv(t, x)
	bundle := append(append([]byte("# Acme roots\n"), testCert(t, "Acme Root")...), testCert(t, "Acme Issuing")...)
	put(t, e.Path("/root/acme.pem"), string(bundle))
	e.Answers = &answers.Answers{CA: answers.CA{Source: "/root/acme.pem"}}
	st := step(t, catalog.Step{Type: "go:corpca"})
	apply(t, e, st)
	for _, n := range []string{"1", "2"} {
		if got := read(t, e.Path(caDir+"/yardo-corp-"+n+".crt")); !strings.HasPrefix(got, "-----BEGIN CERTIFICATE-----") {
			t.Errorf("cert %s: %q", n, got)
		}
	}
	if got := x.commands(); got[0] != "update-ca-certificates --fresh" {
		t.Errorf("ran %q", got)
	}
	if !check(t, e, st) {
		t.Error("Check = false after Apply")
	}
	// A smaller bundle later: the extra file goes.
	put(t, e.Path("/root/acme.pem"), string(testCert(t, "Acme Root 2")))
	apply(t, e, st)
	if _, err := os.Stat(e.Path(caDir + "/yardo-corp-2.crt")); err == nil {
		t.Error("yardo-corp-2.crt left behind")
	}
	// From a URL: the sha256 must match.
	e.Answers.CA = answers.CA{Source: "https://pki.acme/roots.pem", SHA256: strings.Repeat("0", 64)}
	e.Download = func(context.Context, string) ([]byte, error) { return bundle, nil }
	if err := st.Apply(context.Background(), e); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Errorf("wrong sha256: err = %v", err)
	}
	sum := sha256.Sum256(bundle)
	e.Answers.CA.SHA256 = hex.EncodeToString(sum[:])
	apply(t, e, st)
	if _, err := os.Stat(e.Path(caDir + "/yardo-corp-2.crt")); err != nil {
		t.Errorf("from the URL: %v", err)
	}
	put(t, e.Path("/root/bad.pem"), "not a cert")
	e.Answers.CA = answers.CA{Source: "/root/bad.pem"}
	if err := st.Apply(context.Background(), e); err == nil || !strings.Contains(err.Error(), "no PEM certificates") {
		t.Errorf("not PEM: err = %v", err)
	}
}
