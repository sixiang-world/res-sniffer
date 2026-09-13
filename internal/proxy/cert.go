package proxy

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
)

// CertManager CA 证书管理器
type CertManager struct {
	dir       string
	certPath  string
	keyPath   string
	caCert    *tls.Certificate
}

// NewCertManager 创建证书管理器，证书保存在指定目录
func NewCertManager(configDir string) *CertManager {
	return &CertManager{
		dir:      configDir,
		certPath: filepath.Join(configDir, "ca.crt"),
		keyPath:  filepath.Join(configDir, "ca.key"),
	}
}

// LoadOrCreate 加载已有证书或生成新的自签 CA 证书
func (cm *CertManager) LoadOrCreate() error {
	if err := os.MkdirAll(cm.dir, 0750); err != nil {
		return fmt.Errorf("创建证书目录失败: %w", err)
	}

	// 尝试加载已有证书
	if _, err := os.Stat(cm.certPath); err == nil {
		if _, err := os.Stat(cm.keyPath); err == nil {
			cert, err := tls.LoadX509KeyPair(cm.certPath, cm.keyPath)
			if err == nil {
				if cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0]); err != nil {
					return fmt.Errorf("解析已有证书失败: %w", err)
				}
				cm.caCert = &cert
				log.Info().Msg("CA 证书加载成功")
				return nil
			}
		}
	}

	// 生成新的自签 CA 证书
	log.Info().Msg("未找到 CA 证书，正在生成...")
	return cm.generate()
}

// generate 生成 RSA 2048 自签 CA 证书
func (cm *CertManager) generate() error {
	// 生成 RSA 私钥
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("生成 RSA 私钥失败: %w", err)
	}

	// 创建 CA 证书模板
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return fmt.Errorf("生成序列号失败: %w", err)
	}

	tmpl := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"res-sniffer"},
			CommonName:   "res-sniffer CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0), // 10 年有效期
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	// 自签名
	derBytes, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("创建证书失败: %w", err)
	}

	// 写入证书 PEM
	certFile, err := os.Create(cm.certPath)
	if err != nil {
		return fmt.Errorf("创建证书文件失败: %w", err)
	}
	defer certFile.Close()
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return fmt.Errorf("写入证书 PEM 失败: %w", err)
	}

	// 写入私钥 PEM
	keyFile, err := os.Create(cm.keyPath)
	if err != nil {
		return fmt.Errorf("创建私钥文件失败: %w", err)
	}
	defer keyFile.Close()
	keyBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("序列化私钥失败: %w", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return fmt.Errorf("写入私钥 PEM 失败: %w", err)
	}

	// 加载到内存
	cert, err := tls.LoadX509KeyPair(cm.certPath, cm.keyPath)
	if err != nil {
		return fmt.Errorf("加载新证书失败: %w", err)
	}
	if cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0]); err != nil {
		return fmt.Errorf("解析新证书失败: %w", err)
	}
	cm.caCert = &cert

	log.Info().Str("path", cm.certPath).Msg("CA 证书生成成功")
	return nil
}

// GetCert 返回 TLS 证书
func (cm *CertManager) GetCert() *tls.Certificate {
	return cm.caCert
}

// ExportCert 导出证书内容
func (cm *CertManager) ExportCert() ([]byte, error) {
	return os.ReadFile(cm.certPath)
}

// InstallOnLinux 在 Linux 上安装证书到系统信任库
func (cm *CertManager) InstallOnLinux() (string, error) {
	// 复制到系统证书目录
	target := "/usr/local/share/ca-certificates/res-sniffer.crt"
	data, err := os.ReadFile(cm.certPath)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(target, data, 0644); err != nil {
		return "", fmt.Errorf("复制证书失败（需要 sudo）: %w", err)
	}
	return target, nil
}
