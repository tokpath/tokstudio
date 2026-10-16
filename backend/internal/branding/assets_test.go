package branding

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
)

func logoPNG(t *testing.T) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 3, 2))
	im.Set(0, 0, color.RGBA{R: 80, A: 255})
	var b bytes.Buffer
	if e := png.Encode(&b, im); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestLogoRequiresCompleteSafeImage(t *testing.T) {
	pngBytes := logoPNG(t)
	for _, data := range [][]byte{[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script/></svg>`), pngBytes[:30], bytes.Repeat([]byte{1}, MaxLogoBytes+1)} {
		if _, _, e := validateLogo(data, ""); !errors.Is(e, ErrInvalidInput) {
			t.Fatalf("accepted malformed logo: %v", e)
		}
	}
	if _, _, e := validateLogo(pngBytes, "image/jpeg"); !errors.Is(e, ErrInvalidInput) {
		t.Fatal("accepted false content type", e)
	}
}

func TestPostgresS3LogoScopeInheritanceAndPrivateObjectIntegrity(t *testing.T) {
	endpoint := os.Getenv("TOKENHUB_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TOKENHUB_TEST_S3_ENDPOINT not set; real private S3 logo integration was not run")
	}
	f := setup(t)
	store, e := objectstore.New(f.ctx, objectstore.Settings{Endpoint: endpoint, Region: "us-east-1", Bucket: os.Getenv("TOKENHUB_TEST_S3_BUCKET"), AccessKey: os.Getenv("TOKENHUB_TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("TOKENHUB_TEST_S3_SECRET_KEY"), ForcePathStyle: true})
	if e != nil {
		t.Fatal(e)
	}
	if e = store.Ready(f.ctx); e != nil {
		t.Fatal(e)
	}
	f.s.SetAssetStore(store)
	logo, e := f.s.UploadLogo(f.ctx, UploadLogoInput{ActorStaffID: f.owner, ScopeType: "brand", ScopeID: f.platform.Brand.ID, Filename: "logo.png", ContentType: "image/png", Data: logoPNG(t)})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := store.Delete(context.Background(), "branding/logos/"+logo.ID); e != nil {
			t.Error(e)
		}
	})
	if _, _, e = f.s.PublicLogo(f.ctx, f.platform.Brand.ID, "", logo.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("unpublished asset became public", e)
	}
	if _, e = f.s.SetBrandTheme(f.ctx, nil, SetThemeInput{ActorStaffID: f.owner, TargetID: f.platform.Brand.ID, Values: ThemeOverrides{LogoURL: &logo.URL}}); e != nil {
		t.Fatal(e)
	}
	c, cOwner := f.child(t, f.platform, f.owner, organization.Channel)
	_, body, e := f.s.PublicLogo(f.ctx, f.platform.Brand.ID, c.Operator.ID, logo.ID)
	if e != nil || !bytes.Equal(body, logoPNG(t)) {
		t.Fatal("inherited logo missing/corrupt", e)
	}
	if _, e = f.s.UploadLogo(f.ctx, UploadLogoInput{ActorStaffID: cOwner, ScopeType: "channel", ScopeID: c.Operator.ID, Filename: "logo.png", Data: logoPNG(t)}); !errors.Is(e, organization.ErrForbidden) {
		t.Fatal("channel self changed parent branding", e)
	}
	channelLogo, e := f.s.UploadLogo(f.ctx, UploadLogoInput{ActorStaffID: f.owner, ScopeType: "channel", ScopeID: c.Operator.ID, Filename: "channel.png", Data: logoPNG(t)})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := store.Delete(context.Background(), "branding/logos/"+channelLogo.ID); e != nil {
			t.Error(e)
		}
	})
	if _, e = f.s.SetChannelTheme(f.ctx, nil, SetThemeInput{ActorStaffID: f.owner, TargetID: c.Operator.ID, Values: ThemeOverrides{LogoURL: &channelLogo.URL}}); e != nil {
		t.Fatal(e)
	}
	if _, _, e = f.s.PublicLogo(f.ctx, f.platform.Brand.ID, "", channelLogo.ID); e != nil {
		t.Fatal("published channel asset needs hidden query context", e)
	}
	other, _ := f.child(t, f.platform, f.owner, organization.Channel)
	if _, e = f.s.SetChannelTheme(f.ctx, nil, SetThemeInput{ActorStaffID: f.owner, TargetID: other.Operator.ID, Values: ThemeOverrides{LogoURL: &channelLogo.URL}}); !errors.Is(e, organization.ErrForbidden) {
		t.Fatal("cross-channel logo reference", e)
	}
	o, oOwner := f.child(t, f.platform, f.owner, organization.OEM)
	if _, e = f.s.SetBrandTheme(f.ctx, nil, SetThemeInput{ActorStaffID: oOwner, TargetID: o.Brand.ID, Values: ThemeOverrides{LogoURL: &logo.URL}}); !errors.Is(e, organization.ErrForbidden) {
		t.Fatal("cross-brand logo reference", e)
	}
	if _, _, e = f.s.ReadLogo(f.ctx, oOwner, logo.ID); !errors.Is(e, organization.ErrForbidden) {
		t.Fatal("foreign actor read private asset", e)
	}
	if e = store.Put(f.ctx, "branding/logos/"+logo.ID, []byte("corrupted private object"), "image/png", time.Time{}); e != nil {
		t.Fatal(e)
	}
	if _, _, e = f.s.PublicLogo(f.ctx, f.platform.Brand.ID, "", logo.ID); !errors.Is(e, objectstore.ErrUnavailable) {
		t.Fatal("tampered blob served", e)
	}
}

type failedUploadStore struct{}

func (failedUploadStore) Put(context.Context, string, []byte, string, time.Time) error {
	return objectstore.ErrUnavailable
}
func (failedUploadStore) Get(context.Context, string) ([]byte, error) {
	return nil, objectstore.ErrUnavailable
}
func (failedUploadStore) Delete(context.Context, string) error { return nil }
func TestPostgresFailedLogoUploadCannotBecomeThemeReference(t *testing.T) {
	f := setup(t)
	f.s.SetAssetStore(failedUploadStore{})
	if _, e := f.s.UploadLogo(f.ctx, UploadLogoInput{ActorStaffID: f.owner, ScopeType: "brand", ScopeID: f.platform.Brand.ID, Filename: "logo.png", Data: logoPNG(t)}); !errors.Is(e, objectstore.ErrUnavailable) {
		t.Fatal(e)
	}
	var row logoAssetRow
	if e := f.db.First(&row).Error; e != nil {
		t.Fatal(e)
	}
	url := logoURL(row.ID)
	if row.Status != "failed" {
		t.Fatal("failed object reported ready", row.Status)
	}
	if _, e := f.s.SetBrandTheme(f.ctx, nil, SetThemeInput{ActorStaffID: f.owner, TargetID: f.platform.Brand.ID, Values: ThemeOverrides{LogoURL: &url}}); !errors.Is(e, ErrNotFound) {
		t.Fatal("referenced failed upload", e)
	}
}
