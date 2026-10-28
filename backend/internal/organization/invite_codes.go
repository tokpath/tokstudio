package organization

import (
	"context"
	"crypto/rand"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const inviteCodeLength = 6
const inviteCodeSpace int64 = 2176782336 // 36^6

var inviteCodePattern = regexp.MustCompile(`^[0-9A-Z]{6}$`)

func inviteCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(inviteCodeSpace))
	if err != nil {
		return "", err
	}
	code := strings.ToUpper(strconv.FormatInt(n.Int64(), 36))
	return strings.Repeat("0", inviteCodeLength-len(code)) + code, nil
}

func createInviteCode(ctx context.Context, tx *gorm.DB, brand, kind, owner string, at time.Time) (string, error) {
	return claimInviteCode(ctx, tx, brand, kind, owner, at, inviteCode)
}

func claimInviteCode(ctx context.Context, tx *gorm.DB, brand, kind, owner string, at time.Time, next func() (string, error)) (string, error) {
	var customer, operator any
	switch kind {
	case "user":
		customer = owner
	case "channel":
		operator = owner
	default:
		return "", ErrInvalidInput
	}
	for attempt := 0; attempt < 32; attempt++ {
		code, err := next()
		if err != nil {
			return "", err
		}
		if !inviteCodePattern.MatchString(code) {
			return "", ErrInvalidInput
		}
		// A global unique index arbitrates concurrent claims. DO NOTHING keeps
		// a collision from aborting the surrounding registration transaction.
		result := tx.WithContext(ctx).Exec(`INSERT INTO organization_invite_codes(id,code,brand_id,kind,customer_id,operator_id,enabled,created_at) VALUES(?,?,?,?,?,?,true,?) ON CONFLICT (code) DO NOTHING`, newID(), code, brand, kind, customer, operator, at)
		if result.Error != nil {
			return "", result.Error
		}
		if result.RowsAffected == 1 {
			return code, nil
		}
	}
	return "", ErrConflict
}
