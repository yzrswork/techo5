// Generates an independent TLS identity and PTT token in a private directory.
// Never prints private keys or bearer tokens. Existing identities are never overwritten.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func main() {
	dir := flag.String("out", "", "private output directory")
	ip := flag.String("ip", "", "Show's reserved LAN IP; 127.0.0.1 for the PC fixture")
	flag.Parse()
	if *dir == "" || net.ParseIP(*ip) == nil {
		panic("out and valid ip required")
	}
	check(os.MkdirAll(*dir, 0700))
	for _, name := range []string{"ptt-cert.pem", "ptt-key.pem", "ptt-token.txt"} {
		if _, e := os.Stat(filepath.Join(*dir, name)); !os.IsNotExist(e) {
			panic("identity already exists")
		}
	}
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(e)
	serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	check(e)
	now := time.Now()
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "YZRS TECHO5 PTT"}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.AddDate(1, 0, 0), IPAddresses: []net.IP{net.ParseIP(*ip)}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	cert, e := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	check(e)
	secret, e := x509.MarshalPKCS8PrivateKey(key)
	check(e)
	token := make([]byte, 32)
	_, e = rand.Read(token)
	check(e)
	write := func(name string, raw []byte) {
		f, e := os.OpenFile(filepath.Join(*dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		check(e)
		_, e = f.Write(raw)
		check(e)
		check(f.Close())
	}
	write("ptt-cert.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert}))
	write("ptt-key.pem", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: secret}))
	write("ptt-token.txt", []byte(hex.EncodeToString(token)))
	fmt.Println("独立した PTT TLS 証明書・鍵・トークンを保存しました。秘密値は表示しません。")
}
func check(e error) {
	if e != nil {
		panic(e)
	}
}
