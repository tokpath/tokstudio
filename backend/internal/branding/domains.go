package branding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

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
	if err := s.directory.RequireBrandDomain(ctx, tx, actorID, r.BrandID, false); err != nil {
		return Domain{}, err
	}
	return Domain(r), nil
}

// Verification never grants business access. Ownership is persisted before TLS
// provisioning so Caddy may obtain the first certificate for this hostname.
func (s *Service) VerifyDomain(ctx context.Context, actorID, domainID string, verifier DomainVerifier) (Domain, error) {
	observed, err := s.GetDomain(ctx, nil, actorID, domainID)
	if err != nil {
		return Domain{}, err
	}
	return s.verifyDomain(ctx, actorID, observed, verifier)
}
func (s *Service) verifyDomain(ctx context.Context, actorID string, observed Domain, verifier DomainVerifier) (out Domain, err error) {
	if verifier == nil {
		return out, ErrVerificationUnavailable
	}
	domainID := observed.ID
	if v, ok := verifier.(interface {
		VerifyOwnership(context.Context, string, string) (bool, error)
	}); ok && observed.VerifiedAt == nil {
		owned, e := v.VerifyOwnership(ctx, observed.Hostname, observed.VerificationToken)
		if e != nil {
			return out, ErrVerificationUnavailable
		}
		if owned {
			now := s.now()
			before := observed
			err = s.withTx(ctx, nil, func(tx *gorm.DB) error {
				if actorID != uuid.Nil.String() {
					if e := s.directory.RequireBrandDomain(ctx, tx, actorID, observed.BrandID, false); e != nil {
						return e
					}
				}
				res := tx.Model(&domainRow{}).Where("id=? AND version=?", observed.ID, observed.Version).Updates(map[string]any{"verified_at": now, "version": observed.Version + 1})
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected != 1 {
					return ErrConflict
				}
				observed.VerifiedAt = &now
				observed.Version++
				return s.audit(tx, actorID, observed.BrandID, "domain", observed.ID, "domain.ownership_verified", before, observed)
			})
			if err != nil {
				return out, err
			}
		}
	}
	result, err := verifier.Verify(ctx, observed.Hostname, observed.VerificationToken)
	if errors.Is(err, ErrConflict) {
		result = DomainVerification{FailureReason: "Domain does not route to this brand on TokenHub"}
	} else if err != nil {
		return out, ErrVerificationUnavailable
	}

	err = s.withTx(ctx, nil, func(tx *gorm.DB) error {
		var r domainRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "id=?", domainID).Error; err != nil {
			return err
		}
		if r.Version != observed.Version || r.VerificationToken != observed.VerificationToken {
			return ErrConflict
		}
		if actorID != uuid.Nil.String() {
			if err := s.directory.RequireBrandDomain(ctx, tx, actorID, r.BrandID, false); err != nil {
				return err
			}
		}
		before := Domain(r)
		now := s.now()
		r.CheckedAt = &now
		r.TLSReady = result.TLSReady
		r.RouteReady = result.RouteReady
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
		if result.OwnershipVerified {
			r.VerifiedAt = &now
			r.Status = "verified"
			if before.Status == "active" && result.TLSReady && result.RouteReady {
				r.Status = "active"
			}
		} else {
			r.VerifiedAt = nil
			r.Status = "failed"
			r.TLSReady = false
			r.RouteReady = false
			if r.FailureReason == "" {
				r.FailureReason = "domain ownership verification failed"
			}
		}
		// Rechecking evidence must not reverse an explicit stop decision. A
		// disabled hostname requires the separate enable operation after checks.
		if before.Status == "disabled" {
			r.Status = "disabled"
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
		// Serialize changes within one brand so replacing a hostname is atomic.
		var binding domainRow
		if e := tx.First(&binding, "id=?", domainID).Error; e != nil {
			return e
		}
		if e := tx.Exec("SELECT id FROM organization_brands WHERE id=? FOR UPDATE", binding.BrandID).Error; e != nil {
			return e
		}
		var r domainRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "id=?", domainID).Error; err != nil {
			return err
		}
		if err := s.directory.RequireBrandDomain(ctx, tx, actorID, r.BrandID, true); err != nil {
			return err
		}
		if r.Version != expectedVersion {
			return ErrConflict
		}
		before := Domain(r)
		if enabled {
			if r.VerifiedAt == nil || !r.TLSReady || !r.RouteReady || r.ProviderReference == "" || r.CheckedAt == nil || s.now().Sub(*r.CheckedAt) > 10*time.Minute {
				return ErrDomainUnverified
			}
			var previous []domainRow
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("brand_id=? AND status='active' AND id<>?", r.BrandID, r.ID).Find(&previous).Error; e != nil {
				return e
			}
			for _, old := range previous {
				beforeOld := Domain(old)
				old.Status = "disabled"
				old.Version++
				if e := tx.Save(&old).Error; e != nil {
					return e
				}
				if e := s.audit(tx, actorID, r.BrandID, "domain", old.ID, "domain.replaced", beforeOld, Domain(old)); e != nil {
					return e
				}
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
	if err = s.dbFor(ctx, tx).First(&r, "hostname=? AND status='active' AND verified_at IS NOT NULL AND tls_ready=true AND route_ready=true", host).Error; err != nil {
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

// TLSBinding is intentionally less restrictive than ResolveHost: it authorizes
// certificates and a proof response only, never customer or operator requests.
func (s *Service) TLSBinding(ctx context.Context, host string) (Domain, error) {
	host, e := NormalizeHost(host)
	if e != nil {
		return Domain{}, e
	}
	var r domainRow
	e = s.dbFor(ctx, nil).Where("hostname=? AND verified_at IS NOT NULL", host).First(&r).Error
	if e != nil {
		return Domain{}, classify(e)
	}
	b, e := s.directory.GetBrand(ctx, nil, r.BrandID)
	if e != nil {
		return Domain{}, e
	}
	if b.Status != "active" {
		return Domain{}, ErrNotFound
	}
	return Domain(r), nil
}
func (s *Service) PendingDomainChecks(ctx context.Context) ([]Domain, error) {
	var rows []Domain
	e := s.dbFor(ctx, nil).Table("branding_domains").Where("status IN ('pending','failed','verified') AND (checked_at IS NULL OR checked_at < ?)", s.now().Add(-5*time.Minute)).Order("checked_at ASC NULLS FIRST,id").Limit(20).Find(&rows).Error
	return rows, e
}
func (s *Service) CheckPendingDomain(ctx context.Context, id string, verifier DomainVerifier) error {
	var r domainRow
	if e := s.dbFor(ctx, nil).First(&r, "id=?", id).Error; e != nil {
		return classify(e)
	}
	if r.Status == "disabled" || r.Status == "active" {
		return nil
	}
	_, e := s.verifyDomain(ctx, uuid.Nil.String(), Domain(r), verifier)
	return e
}
