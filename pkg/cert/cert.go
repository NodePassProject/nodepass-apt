// Package cert 提供了一个简单的TLS配置生成器，使用ECDSA密钥和自签名证书。
package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"
)

// NewTLSConfig 生成TLS配置
func NewTLSConfig(name string) (*tls.Config, error) {
	// 生成ECDSA私钥
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	// 生成随机序列号
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	// 创建证书模板
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{name},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(1, 0, 0), // 证书有效期一年
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	// 创建自签名证书
	crtBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &private.PublicKey, private)
	if err != nil {
		return nil, err
	}

	// 将私钥转换为PKCS8格式
	keyBytes, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return nil, err
	}

	// 编码证书和私钥为PEM格式
	crtPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: crtBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})

	// 从PEM编码的证书和私钥创建TLS证书
	cert, err := tls.X509KeyPair(crtPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	// 返回TLS配置
	return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
}
