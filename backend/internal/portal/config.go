package portal

import (
	"context"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

type Config struct {
	*config.Config
	OfficialHost                                     string
	AppKey                                           string
	BootstrapEmail                                   string
	BootstrapPassword                                string
	BootstrapName                                    string
	BrandName                                        string
	OperatorName                                     string
	LegalName                                        string
	TrustedProxies                                   []string
	CloudflareToken, CloudflareZone, CloudflareCNAME string
	MetricsToken                                     string
	TestLoopback                                     bool
}

func LoadConfig() (Config, error) {
	base, err := config.Load()
	if err != nil {
		return Config{}, err
	}
	c := Config{Config: base, OfficialHost: os.Getenv("TOKENHUB_OFFICIAL_HOST"), AppKey: os.Getenv("TOKENHUB_APP_KEY"), BootstrapEmail: os.Getenv("TOKENHUB_BOOTSTRAP_STAFF_EMAIL"), BootstrapPassword: os.Getenv("TOKENHUB_BOOTSTRAP_STAFF_PASSWORD"), BootstrapName: os.Getenv("TOKENHUB_BOOTSTRAP_STAFF_NAME"), BrandName: os.Getenv("TOKENHUB_BRAND_NAME"), OperatorName: os.Getenv("TOKENHUB_OPERATOR_NAME"), LegalName: os.Getenv("TOKENHUB_LEGAL_NAME"), TestLoopback: os.Getenv("TOKENHUB_INTEGRATION_TEST_LOOPBACK") == "true"}
	c.CloudflareToken = os.Getenv("TOKENHUB_CLOUDFLARE_API_TOKEN")
	c.CloudflareZone = os.Getenv("TOKENHUB_CLOUDFLARE_ZONE_ID")
	c.CloudflareCNAME = os.Getenv("TOKENHUB_CLOUDFLARE_CNAME_TARGET")
	c.MetricsToken = os.Getenv("TOKENHUB_METRICS_TOKEN")
	if c.AppKey == "" {
		c.AppKey = base.EncryptionKey
	}
	if c.OfficialHost == "" {
		u, e := url.Parse(base.WebOrigin)
		if e == nil {
			c.OfficialHost = u.Hostname()
		}
	}
	for _, v := range strings.Split(os.Getenv("TOKENHUB_TRUSTED_PROXIES"), ",") {
		if v = strings.TrimSpace(v); v != "" {
			c.TrustedProxies = append(c.TrustedProxies, v)
		}
	}
	return c, c.Validate()
}
func (c Config) Validate() error {
	if c.Config == nil || len(c.AppKey) < 32 || strings.TrimSpace(c.OfficialHost) == "" {
		return errors.New("TOKENHUB_APP_KEY (at least 32 characters) and TOKENHUB_OFFICIAL_HOST are required")
	}
	if c.TestLoopback && c.IsProduction() {
		return errors.New("integration loopback cannot be enabled in production")
	}
	if strings.ContainsAny(c.OfficialHost, "/:?#@\\ ") {
		return errors.New("official host must be a bare hostname")
	}
	if c.IsProduction() && (c.OfficialHost == "localhost" || net.ParseIP(c.OfficialHost) != nil) {
		return errors.New("production official host must be a DNS name")
	}
	for _, cidr := range c.TrustedProxies {
		if _, _, e := net.ParseCIDR(cidr); e != nil && net.ParseIP(cidr) == nil {
			return errors.New("invalid trusted proxy IP/CIDR")
		}
	}
	return nil
}

func NewObjectStore(ctx context.Context, c Config) (*objectstore.Store, error) {
	return objectstore.New(ctx, objectstore.Settings{Endpoint: c.S3Endpoint, Region: c.S3Region, Bucket: c.S3Bucket, AccessKey: c.S3AccessKey, SecretKey: c.S3SecretKey, ForcePathStyle: c.S3ForcePathStyle})
}
