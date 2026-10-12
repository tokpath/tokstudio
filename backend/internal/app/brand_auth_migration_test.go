package app_test

import (
	"io/fs"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
)

func TestBrandAuthMigrationPreservesLegacyIdentities(t *testing.T) {
	dsn := os.Getenv("TOKENHUB_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated postgres required")
	}
	root, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	rootSQL, _ := root.DB()
	defer rootSQL.Close()
	schema := "brand_auth_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := root.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	defer root.Exec("DROP SCHEMA " + schema + " CASCADE")
	target, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	values := target.Query()
	values.Set("search_path", schema)
	target.RawQuery = values.Encode()
	isolated, err := db.Open(target.String())
	if err != nil {
		t.Fatal(err)
	}
	isolatedSQL, _ := isolated.DB()
	defer isolatedSQL.Close()
	module, migrations := identity.Migrations()
	old := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") && entry.Name() < "0021" {
			body, _ := fs.ReadFile(migrations, entry.Name())
			old[entry.Name()] = &fstest.MapFile{Data: body}
		}
	}
	if err := db.Apply(isolated, []db.ModuleMigrations{{Module: module, FS: old}}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO identity_brands(id,name,primary_domain,api_domain,admin_domain) VALUES('brd_official','Official','official.test','api.official.test','admin.official.test'),('brd_second','Second','second.test','api.second.test','admin.second.test'),('brd_third','Third','third.test','api.third.test','admin.third.test')`,
		`INSERT INTO identity_channel_orgs(id,code,type,brand_id) VALUES('legacy_channel','legacy','A','brd_official')`,
		`INSERT INTO identity_users(id,email,google_sub,channel_org_id) VALUES('original_user','Legacy@Example.test','google-stable','legacy_channel'),('unattributed_user','unattributed@example.test',NULL,NULL)`,
		`INSERT INTO identity_users(id,email,brand_id) VALUES('legacy_second_user','legacy@example.test','brd_second')`,
		`INSERT INTO identity_access_tokens(id,user_id,token_hash,prefix,status) VALUES('original_token','original_user','stable_token_hash','thses_','active')`,
	} {
		if err := isolated.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Apply(isolated, []db.ModuleMigrations{{Module: module, FS: migrations}}); err == nil {
		t.Fatal("migration guessed the brand of an unattributed user")
	}
	var exists bool
	isolated.Raw("SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='identity_users_email_key' AND conrelid='identity_users'::regclass)").Scan(&exists)
	if !exists {
		t.Fatal("failed migration partially removed global uniqueness")
	}
	if err := isolated.Exec("UPDATE identity_users SET channel_org_id='legacy_channel' WHERE id='unattributed_user'").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Apply(isolated, []db.ModuleMigrations{{Module: module, FS: migrations}}); err != nil {
		t.Fatal(err)
	}
	var row struct{ ID, Email, BrandID, GoogleSub string }
	if err := isolated.Table("identity_users").Where("id='original_user'").Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.ID != "original_user" || row.Email != "legacy@example.test" || row.BrandID != "brd_official" || row.GoogleSub != "google-stable" {
		t.Fatalf("legacy user changed %+v", row)
	}
	var tokenUser string
	isolated.Table("identity_access_tokens").Select("user_id").Where("id='original_token' AND token_hash='stable_token_hash'").Scan(&tokenUser)
	if tokenUser != "original_user" {
		t.Fatal("migration changed token ownership")
	}
	if err := isolated.Exec(`INSERT INTO identity_users(id,email,brand_id,google_sub) VALUES('third_user','legacy@example.test','brd_third','google-stable')`).Error; err != nil {
		t.Fatal("other-brand duplicate identity rejected", err)
	}
	if err := isolated.Exec(`INSERT INTO identity_users(id,email,brand_id) VALUES('same_brand_duplicate','LEGACY@example.test','brd_official')`).Error; err == nil {
		t.Fatal("same-brand duplicate email accepted")
	}
}
