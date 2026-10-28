package organization

import (
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestInviteCodeUsesSixUppercaseLettersAndDigits(t *testing.T) {
	for i := 0; i < 256; i++ {
		code, err := inviteCode()
		if err != nil || !inviteCodePattern.MatchString(code) {
			t.Fatal(code, err)
		}
	}
}

func newCodeCustomer(tx *gorm.DB, brand string) (string, error) {
	id := newID()
	err := tx.Create(&customerRow{ID: id, BrandID: brand, DisplayName: "Code test", Status: "active", RegisteredAt: time.Now(), NewCustomerRecordedAt: time.Now()}).Error
	return id, err
}

func TestPostgresInviteCodeCollisionsRetryAcrossBrandsAndKinds(t *testing.T) {
	w := world(t)
	channel, _ := w.child(t, w.platform, w.platformOwner, Channel, "Codes channel")
	oem, _ := w.child(t, w.platform, w.platformOwner, OEM, "Codes OEM")
	if err := w.db.Exec("UPDATE organization_invite_codes SET code='A1B2C3',enabled=false WHERE operator_id=?", channel.Operator.ID).Error; err != nil {
		t.Fatal(err)
	}
	var got string
	calls := 0
	err := w.db.Transaction(func(tx *gorm.DB) error {
		owner, err := newCodeCustomer(tx, oem.Brand.ID)
		if err != nil {
			return err
		}
		got, err = claimInviteCode(w.ctx, tx, oem.Brand.ID, "user", owner, time.Now(), func() (string, error) {
			calls++
			if calls == 1 {
				return "A1B2C3", nil
			}
			return "000001", nil
		})
		return err
	})
	if err != nil || got != "000001" || calls != 2 {
		t.Fatal("collision aborted registration or lost leading zeroes", got, calls, err)
	}
	const count = 12
	var wg sync.WaitGroup
	results := make(chan error, count)
	for i := 0; i < count; i++ {
		brand := w.platform.Brand.ID
		if i%2 == 1 {
			brand = oem.Brand.ID
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- w.db.Transaction(func(tx *gorm.DB) error {
				owner, err := newCodeCustomer(tx, brand)
				if err != nil {
					return err
				}
				first := true
				_, err = claimInviteCode(w.ctx, tx, brand, "user", owner, time.Now(), func() (string, error) {
					if first {
						first = false
						return "A1B2C3", nil
					}
					return inviteCode()
				})
				return err
			})
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	var totals struct{ Total, Distinct int64 }
	if err = w.db.Raw("SELECT count(*) AS total,count(DISTINCT code) AS distinct FROM organization_invite_codes").Scan(&totals).Error; err != nil || totals.Total != count+2 || totals.Distinct != totals.Total {
		t.Fatal("global duplicate code", totals, err)
	}
	var failedOwner string
	err = w.db.Transaction(func(tx *gorm.DB) error {
		var err error
		failedOwner, err = newCodeCustomer(tx, oem.Brand.ID)
		if err != nil {
			return err
		}
		_, err = claimInviteCode(w.ctx, tx, oem.Brand.ID, "user", failedOwner, time.Now(), func() (string, error) { return "A1B2C3", nil })
		return err
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatal("unbounded or successful duplicate claim", err)
	}
	var remaining int64
	w.db.Model(&customerRow{}).Where("id=?", failedOwner).Count(&remaining)
	if remaining != 0 {
		t.Fatal("failed allocation left a partial customer")
	}
}

func TestPostgresShortCodeMigrationPreservesOwnersAndRegistration(t *testing.T) {
	w := world(t)
	channel, _ := w.child(t, w.platform, w.platformOwner, Channel, "Migration channel")
	inviter := w.register(t, w.platform.Brand.ID, "inviter@code.test", channel.InviteCode)
	child := w.register(t, w.platform.Brand.ID, "child@code.test", inviter.InviteCode)
	pending := w.challenge(t, w.platform.Brand.ID, "pending@code.test", channel.InviteCode)
	if err := w.db.Exec("ALTER TABLE organization_invite_codes DROP CONSTRAINT organization_invite_code_six_chars").Error; err != nil {
		t.Fatal(err)
	}
	if err := w.db.Exec("UPDATE organization_invite_codes SET code=upper(substr(md5(id::text),1,24))").Error; err != nil {
		t.Fatal(err)
	}
	if err := w.db.Exec("UPDATE organization_invite_codes SET code='KEEP01' WHERE customer_id=?", inviter.Customer.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := w.db.Exec("UPDATE organization_invite_codes SET enabled=false WHERE customer_id=?", child.Customer.ID).Error; err != nil {
		t.Fatal(err)
	}
	var before []codeRow
	if err := w.db.Order("id").Find(&before).Error; err != nil {
		t.Fatal(err)
	}
	var oldChannelCode string
	for _, r := range before {
		if r.Kind == "channel" {
			oldChannelCode = r.Code
		}
	}
	stateID := newID()
	if err := w.db.Exec(`INSERT INTO organization_google_states(id,brand_id,state_hash,redirect_uri,invite_code,config_version,expires_at,created_at) VALUES(?,?,?,?,?,1,?,?)`, stateID, w.platform.Brand.ID, strings.Repeat("a", 64), "https://brand.test/callback", oldChannelCode, time.Now().Add(time.Hour), time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	_, migrations := Migrations()
	sql, err := fs.ReadFile(migrations, "0010_six_character_invite_codes.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = w.db.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(sql)).Error }); err != nil {
		t.Fatal(err)
	}
	var after []codeRow
	if err = w.db.Order("id").Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatal("migration recreated or deleted codes")
	}
	var newChannelCode string
	for i, r := range after {
		old := before[i]
		if r.ID != old.ID || r.BrandID != old.BrandID || r.Enabled != old.Enabled || r.Kind != old.Kind || !reflect.DeepEqual(r.CustomerID, old.CustomerID) || !reflect.DeepEqual(r.OperatorID, old.OperatorID) || !inviteCodePattern.MatchString(r.Code) {
			t.Fatal(old, r)
		}
		if old.Code == "KEEP01" && r.Code != old.Code {
			t.Fatal("existing short code changed")
		}
		if r.Kind == "channel" {
			newChannelCode = r.Code
		}
	}
	var rawStateCode string
	w.db.Table("organization_google_states").Select("invite_code").Where("id=?", stateID).Scan(&rawStateCode)
	if rawStateCode != newChannelCode {
		t.Fatal("pending Google attribution lost", rawStateCode, newChannelCode)
	}
	var inviterID string
	w.db.Table("organization_referral_edges").Select("inviter_id").Where("invitee_id=?", child.Customer.ID).Scan(&inviterID)
	if inviterID != inviter.Customer.ID {
		t.Fatal("referral relation changed")
	}
	completed, err := w.s.CompleteRegistration(w.ctx, nil, CompleteRegistrationInput{IntentID: pending.Intent.ID, VerificationToken: pending.VerificationToken})
	if err != nil || completed.Ownership.OperatorID != channel.Operator.ID || completed.InviterID != nil || !inviteCodePattern.MatchString(completed.InviteCode) {
		t.Fatal("pending registration changed attribution", completed, err)
	}
	if _, err = w.s.ResolveRegistrationIntent(w.ctx, nil, w.platform.Brand.ID, strings.ToLower(newChannelCode)); err != nil {
		t.Fatal("lowercase short code not accepted", err)
	}
	if _, err = w.s.ResolveRegistrationIntent(w.ctx, nil, w.platform.Brand.ID, oldChannelCode); !errors.Is(err, ErrInvalidInviteCode) {
		t.Fatal("unexpected legacy alias", err)
	}
	if err = w.db.Exec("UPDATE organization_invite_codes SET code='TOO-LONG' WHERE operator_id=?", channel.Operator.ID).Error; err == nil {
		t.Fatal("database accepted an invalid format")
	}
}
