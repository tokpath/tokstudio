package branding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"golang.org/x/net/idna"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type domainRow Domain

func (domainRow) TableName() string { return "branding_domains" }

func NormalizeDomain(host string) (string, error) {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if strings.ContainsAny(host, ":/*?#@\\") || net.ParseIP(host) != nil {
		return "", ErrInvalidInput
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil || len(ascii) > 253 || !strings.Contains(ascii, ".") {
		return "", ErrInvalidInput
	}
	for _, label := range strings.Split(ascii, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", ErrInvalidInput
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", ErrInvalidInput
			}
		}
	}
	return ascii, nil
}
func NormalizeHost(host string) (string, error) {
	if strings.Contains(host, ":") {
		h, p, err := net.SplitHostPort(host)
		if err != nil {
			return "", ErrInvalidInput
		}
		port, err := strconv.Atoi(p)
		if err != nil || port < 1 || port > 65535 {
			return "", ErrInvalidInput
		}
		host = h
	}
	return NormalizeDomain(host)
}

func (s *Service) RequestDomain(ctx context.Context, tx *gorm.DB, actorID, brandID, hostname string) (out DomainChallenge, err error) {
	hostname, err = NormalizeDomain(hostname)
	if err != nil {
		return
	}
	if !validID(actorID) || !validID(brandID) {
		return out, ErrInvalidInput
	}
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		if err := s.directory.RequireBrandBranding(ctx, tx, actorID, brandID); err != nil {
			return err
		}
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		r := domainRow{ID: uuid.NewString(), BrandID: brandID, Hostname: hostname, VerificationToken: hex.EncodeToString(b), Status: "pending", Version: 1, CreatedAt: s.now()}
		if err := tx.Create(&r).Error; err != nil {
			return err
		}
		out = DomainChallenge{Domain: Domain(r), TXTName: "_tokenhub." + hostname, TXTValue: "tokenhub-verification=" + r.VerificationToken}
		return s.audit(tx, actorID, brandID, "domain", r.ID, "domain.requested", nil, out.Domain)
	})
	return
}

func (s *Service) GetDomain(ctx context.Context, tx *gorm.DB, actorID, domainID string) (Domain, error) {
	if !validID(domainID) {
		return Domain{}, ErrInvalidInput
	}
	var r domainRow
	if err := s.dbFor(ctx, tx).First(&r, "id=?", domainID).Error; err != nil {
		return Domain{}, classify(err)
	}
	if err := s.directory.RequireBrandBranding(ctx, tx, actorID, r.BrandID); err != nil {
		return Domain{}, err
	}
	return Domain(r), nil
}

// VerifyAndActivateDomain performs provider I/O before opening the database
// transaction and then checks the observed version under a row lock. The
// provider must verify both DNS ownership and TLS before host resolution works.
func (s *Service) VerifyAndActivateDomain(ctx context.Context, actorID, domainID string, verifier DomainVerifier) (out Domain, err error) {
	if verifier == nil {
		return out, ErrVerificationUnavailable
	}
	observed, err := s.GetDomain(ctx, nil, actorID, domainID)
	if err != nil {
		return out, err
	}
	result, err := verifier.Verify(ctx, observed.Hostname, observed.VerificationToken)
	if err != nil {
		return out, err
	}
	err = s.withTx(ctx, nil, func(tx *gorm.DB) error {
		var r domainRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "id=?", domainID).Error; err != nil {
			return err
		}
		if r.Version != observed.Version || r.VerificationToken != observed.VerificationToken {
			return ErrConflict
		}
		if err := s.directory.RequireBrandBranding(ctx, tx, actorID, r.BrandID); err != nil {
			return err
		}
		before := Domain(r)
		now := s.now()
		r.CheckedAt = &now
		r.TLSReady = result.TLSReady
		r.ProviderReference = result.ProviderReference
		r.ProviderStatus = result.ProviderStatus
		r.TLSStatus = result.TLSStatus
		instructions := result.Instructions
		if instructions == nil {
			instructions = []DNSInstruction{}
		}
		r.ProviderInstructions, err = json.Marshal(instructions)
		if err != nil {
			return err
		}
		r.FailureReason = result.FailureReason
		r.Version++
		if result.OwnershipVerified && strings.TrimSpace(result.ProviderReference) != "" {
			r.VerifiedAt = &now
			r.Status = "verified"
			if result.TLSReady {
				r.Status = "active"
			}
		} else {
			r.VerifiedAt = nil
			r.Status = "failed"
			r.TLSReady = false
			if r.FailureReason == "" {
				r.FailureReason = "domain ownership verification failed"
			}
		}
		if err := tx.Save(&r).Error; err != nil {
			return err
		}
		out = Domain(r)
		return s.audit(tx, actorID, r.BrandID, "domain", r.ID, "domain.verified", before, out)
	})
	return
}

func (s *Service) SetDomainEnabled(ctx context.Context, tx *gorm.DB, actorID, domainID string, enabled bool, expectedVersion int64) (out Domain, err error) {
	if !validID(domainID) || expectedVersion < 1 {
		return out, ErrInvalidInput
	}
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		var r domainRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "id=?", domainID).Error; err != nil {
			return err
		}
		if err := s.directory.RequireBrandBranding(ctx, tx, actorID, r.BrandID); err != nil {
			return err
		}
		if r.Version != expectedVersion {
			return ErrConflict
		}
		before := Domain(r)
		if enabled {
			if r.VerifiedAt == nil || !r.TLSReady || r.ProviderReference == "" {
				return ErrDomainUnverified
			}
			r.Status = "active"
		} else {
			r.Status = "disabled"
		}
		r.Version++
		if err := tx.Save(&r).Error; err != nil {
			return err
		}
		out = Domain(r)
		return s.audit(tx, actorID, r.BrandID, "domain", r.ID, "domain.enabled_changed", before, out)
	})
	return
}

func (s *Service) ResolveHost(ctx context.Context, tx *gorm.DB, host string) (organization.Brand, error) {
	host, err := NormalizeHost(host)
	if err != nil {
		return organization.Brand{}, err
	}
	var r domainRow
	if err = s.dbFor(ctx, tx).First(&r, "hostname=? AND status='active' AND verified_at IS NOT NULL AND tls_ready=true", host).Error; err != nil {
		return organization.Brand{}, classify(err)
	}
	b, err := s.directory.GetBrand(ctx, tx, r.BrandID)
	if err != nil {
		return b, err
	}
	if b.Status != "active" {
		return organization.Brand{}, ErrNotFound
	}
	return b, nil
}
