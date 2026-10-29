package mail

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/db/dbtest"
)

func testMailService(t *testing.T, fallback SMTPOptions) *Service {
	t.Helper()
	gdb := dbtest.New(t)
	name, fs := Migrations()
	if err := db.Apply(gdb, []db.ModuleMigrations{{Module: name, FS: fs}}); err != nil {
		t.Fatal(err)
	}
	return NewService(gdb, strings.Repeat("k", 32), fallback, "official")
}

func TestPostgresBrandMailInheritanceAndSMTPDelivery(t *testing.T) {
	ctx := context.Background()
	platform, received := localSMTP(t)
	platform.FromName = "Platform name must not leak"
	s := testMailService(t, platform)
	view, err := s.SettingsForBrand(ctx, "oem")
	if err != nil || !view.Available || !view.Inherited || view.Mode != "platform" || view.From != platform.From {
		t.Fatal(view, err)
	}
	if view.Host != "" || view.Username != "" || view.PasswordSet || view.FromName != "" {
		t.Fatal("shared credentials leaked", view)
	}
	if err = s.Send(ctx, Message{BrandID: "oem", Brand: "OEM Brand", To: "customer@example.test", Subject: "test", Text: "registration"}); err != nil {
		t.Fatal(err)
	}
	if raw := <-received; !strings.Contains(raw, "OEM Brand") || strings.Contains(raw, platform.FromName) {
		t.Fatal("inherited sender identity", raw)
	}
}

func TestPostgresBrandMailOverrideDisableAndReturnToDefault(t *testing.T) {
	ctx := context.Background()
	platform, receivedPlatform := localSMTP(t)
	s := testMailService(t, platform)
	custom, receivedCustom := localSMTP(t)
	custom.From = "oem@example.test"
	view, err := s.UpdateForBrand(ctx, SettingsInput{Mode: "custom", Enabled: true, Host: custom.Host, Port: custom.Port, From: custom.From, FromName: "OEM support", Security: "plain"}, "oem")
	if err != nil || view.Inherited || !view.Available || view.From != custom.From {
		t.Fatal(view, err)
	}
	if err = s.Send(ctx, Message{BrandID: "oem", Brand: "OEM Brand", To: "customer@example.test", Subject: "test"}); err != nil {
		t.Fatal(err)
	}
	if raw := <-receivedCustom; !strings.Contains(raw, "oem@example.test") || !strings.Contains(raw, "OEM support") {
		t.Fatal(raw)
	}
	// Another OEM and the official brand retain their own availability.
	if !s.AvailableForBrand("other-oem") || !s.AvailableForBrand("official") {
		t.Fatal("brand settings affected other brands")
	}
	_, err = s.UpdateForBrand(ctx, SettingsInput{Mode: "custom", Enabled: false}, "oem")
	if err != nil || s.AvailableForBrand("oem") {
		t.Fatal("disabled custom transport fell through to platform", err)
	}
	if err = s.Send(ctx, Message{BrandID: "oem", To: "customer@example.test"}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	view, err = s.UpdateForBrand(ctx, SettingsInput{Mode: "platform"}, "oem")
	if err != nil || !view.Inherited || !view.Available {
		t.Fatal(view, err)
	}
	if err = s.Send(ctx, Message{BrandID: "oem", Brand: "OEM Brand", To: "customer@example.test", Subject: "restored"}); err != nil {
		t.Fatal(err)
	}
	<-receivedPlatform
	var count int64
	if err = s.db.Model(&settingsRow{}).Where("id=?", "brand:oem").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("old custom credentials retained", count, err)
	}
}

func TestPostgresBrandMailDoesNotBorrowPasswords(t *testing.T) {
	ctx := context.Background()
	platform := SMTPOptions{Host: "smtp.platform.example", Port: 587, Username: "platform", Password: "platform-secret", From: "platform@example.test", Security: "starttls"}
	s := testMailService(t, platform)
	input := SettingsInput{Mode: "custom", Enabled: true, Host: "smtp.oem.example", Username: "oem", From: "oem@example.test", Security: "starttls"}
	if _, err := s.UpdateForBrand(ctx, input, "oem"); !errors.Is(err, ErrSettings) {
		t.Fatal("custom brand reused platform password", err)
	}
	password := "oem-secret"
	input.Password = &password
	if _, err := s.UpdateForBrand(ctx, input, "oem"); err != nil {
		t.Fatal(err)
	}
	input.Password = nil
	input.FromName = "OEM service"
	if _, err := s.UpdateForBrand(ctx, input, "oem"); err != nil {
		t.Fatal("blank password did not preserve own password", err)
	}
	var row settingsRow
	if err := s.db.First(&row, "id=?", "brand:oem").Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(row.PayloadCiphertext, password) {
		t.Fatal("unencrypted password")
	}
	plain, err := crypto.Open(s.key, row.PayloadCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	var opt SMTPOptions
	if err = json.Unmarshal([]byte(plain), &opt); err != nil || opt.Password != password {
		t.Fatal("own password not preserved", err)
	}
	if _, err = s.Update(ctx, SettingsInput{Mode: "platform"}); !errors.Is(err, ErrSettings) {
		t.Fatal("platform inherited itself", err)
	}
	input.FromName = "injected\r\nBcc: other@example.test"
	if _, err = s.UpdateForBrand(ctx, input, "oem"); !errors.Is(err, ErrSettings) {
		t.Fatal("sender header injection accepted", err)
	}
}
