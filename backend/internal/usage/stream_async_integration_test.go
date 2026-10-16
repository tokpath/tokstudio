package usage

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"gorm.io/gorm"
)

func externalRuntime(t *testing.T, f fixture) string {
	t.Helper()
	base := strings.TrimRight(os.Getenv("TOKENHUB_MODEL_MOCK_URL"), "/")
	if base == "" {
		t.Skip("separately deployed model mock required")
	}
	origin, err := url.Parse(base)
	if err != nil || origin.Hostname() != "127.0.0.1" {
		t.Fatal("explicit test loopback required")
	}
	if f.s.ResultStore == nil {
		t.Fatal("private S3 required")
	}
	runtime, err := modelruntime.NewBifrost(f.ctx, modelruntime.BifrostConfig{Providers: []string{"openai"}, AllowTestLoopback: true, Credentials: f.cat, EndpointGuard: func(_ context.Context, target string) error {
		u, err := url.Parse(target)
		if err != nil || u.Host != origin.Host {
			return modelruntime.ErrInvalidRequest
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtime.Close)
	f.s.Runtime = runtime
	return base
}

func TestExternalStreamingBillsReliableDisconnectedUsageAndNeverRegenerates(t *testing.T) {
	if os.Getenv("TOKENHUB_MODEL_MOCK_URL") == "" {
		t.Skip("separately deployed model mock required")
	}
	for _, tc := range []struct {
		name, model                   string
		disconnectAfterUsage, charged bool
	}{
		{"success", "test-chat", false, true},
		{"client_disconnect_after_metering", "test-chat", true, true},
		{"provider_disconnect_after_output", "test-chat-stream-drop", false, false},
		{"provider_omits_usage", "test-chat-usage-missing", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, "100", nil)
			externalRuntime(t, f)
			var row struct{ Definition []byte }
			if err := f.db.Table("tokenhub_upstreams").Select("definition").Where("id='upstream'").Take(&row).Error; err != nil {
				t.Fatal(err)
			}
			var upstream catalog.UpstreamModel
			if err := json.Unmarshal(row.Definition, &upstream); err != nil {
				t.Fatal(err)
			}
			upstream.Model = tc.model
			if err := f.cat.SaveUpstream(f.ctx, f.operator, upstream, 1); err != nil {
				t.Fatal(err)
			}
			// A healthy second candidate makes accidental regeneration observable.
			upstream.ID, upstream.Model, upstream.Priority = "fallback", "test-chat", 99
			if err := f.cat.SaveUpstream(f.ctx, f.operator, upstream, 0); err != nil {
				t.Fatal(err)
			}
			price := catalog.PriceVersion{ID: uuid.NewString(), Scope: catalog.PriceScope{Kind: "supplier", SellerID: "provider", BuyerID: "fallback"}, ModelID: "public-model", Layer: "regular", EffectiveAt: time.Now().Add(-time.Hour), Published: true, NoticeID: "stream-contract", Rule: catalog.PriceRule{Mode: "manual", Components: []catalog.ComponentRate{{Key: "input", Unit: "token", Per: "1", Rate: "0.001"}, {Key: "output", Unit: "token", Per: "1", Rate: "0.001"}}}}
			// This additional platform supplier cost changes no retail price or
			// procurement edge, so there is no downstream coverage to recompute.
			if err := f.cat.PublishPrice(f.ctx, f.operator, price, func(_ context.Context, _ *gorm.DB, p catalog.PriceVersion) error {
				if p.Scope.Kind != "supplier" {
					return errors.New("fixture only permits supplier price")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			output := 0
			clientGone := errors.New("test downstream disconnected after receiving output")
			result, err := f.s.Execute(f.ctx, f.key.Key, "stream", request(t, true), func(_ context.Context, event modelruntime.Event) error {
				if tc.disconnectAfterUsage && event.Usage != nil {
					return clientGone
				}
				if len(event.Data) > 0 {
					output++
				}
				return nil
			})
			if output == 0 {
				t.Fatal("no real upstream output reached the customer", err)
			}
			if tc.disconnectAfterUsage && !errors.Is(err, clientGone) {
				t.Fatal("downstream disconnect not preserved", err)
			}
			if tc.charged {
				if result.Request.Status != "completed" || result.Request.ChargedATG.String() != "0.017" {
					t.Fatal("reliable stream not settled", result.Request, err)
				}
			} else {
				if result.Request.Status != "awaiting_usage" || !result.Request.ChargedATG.IsZero() {
					t.Fatal("unverified stream charged", result.Request, err)
				}
				f.s.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
				free, err := f.s.ResolveTimeout(f.ctx, result.Request.ID)
				if err != nil || free.Status != "free" {
					t.Fatal(free, err)
				}
				late, err := f.s.Settle(f.ctx, nil, result.Request.ID, measured("10", "7"))
				if err != nil || !late.ChargedATG.IsZero() || late.Status != "free" {
					t.Fatal("late usage charged customer", late, err)
				}
			}
			_, _ = f.s.Execute(f.ctx, f.key.Key, "stream", request(t, true), func(context.Context, modelruntime.Event) error { return nil })
			var attempts int64
			if err := f.db.Model(&AttemptRecord{}).Where("request_id=?", result.Request.ID).Count(&attempts).Error; err != nil || attempts != 1 {
				t.Fatal("stream regenerated", attempts, err)
			}
			grants, err := f.l.CustomerGrants(f.ctx, nil, f.operator, f.customer)
			if err != nil {
				t.Fatal(err)
			}
			for _, g := range grants {
				if err = f.l.Reconcile(f.ctx, nil, g.AccountID); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestExternalAsyncVideoUsesOriginalTaskAndBillsOnceAcrossPollingAndDownloads(t *testing.T) {
	if os.Getenv("TOKENHUB_MODEL_MOCK_URL") == "" {
		t.Skip("separately deployed model mock required")
	}
	capability := modelruntime.Capability{Operation: modelruntime.Video, InputTypes: []string{"text"}, Parameters: []string{"seconds", "size"}, RequiredParameters: []string{"seconds", "size"}, MaxInputBytes: 10000, ParameterConstraints: map[string]modelruntime.ParameterConstraint{"seconds": {Values: []string{"4"}}, "size": {Values: []string{"720x1280"}}}, UsageRules: []modelruntime.UsageRule{{Component: "seconds", Unit: "second", Path: "/provider/seconds"}}, BillableBounds: map[string]string{"seconds": "4"}, EvidenceID: "external-lab-completed-duration", VerifiedAt: time.Now(), ParametersVerified: true, UsageVerified: true, BoundsVerified: true, RecoveryVerified: true}
	f := setup(t, "100", nil, capability)
	externalRuntime(t, f)
	input, err := modelruntime.DecodeProtocol("incoming", modelruntime.Video, []byte(`{"model":"public-model","prompt":"a blue dot","seconds":"4","size":"720x1280"}`))
	if err != nil {
		t.Fatal(err)
	}
	started, err := f.s.Execute(f.ctx, f.key.Key, "video-original", input, nil)
	if err != nil || started.Request.Status != "submitted" || !started.Request.ChargedATG.IsZero() {
		t.Fatal("submission billed before actual usage", started.Request.Status, err)
	}
	_, err = f.s.Execute(f.ctx, f.key.Key, "video-original", input, nil)
	if !errors.Is(err, ErrInProgress) {
		t.Fatal("duplicate submission should follow original", err)
	}
	completed := started.Request
	deadline := time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		completed, err = f.s.RefreshAsync(f.ctx, started.Request.ID)
		if err != nil {
			t.Fatal("original task lookup failed", err)
		}
		if completed.Status == "completed" {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if completed.Status != "completed" || completed.ChargedATG.String() != "0.04" {
		t.Fatal("actual duration not settled", completed.Status, completed.ChargedATG.String())
	}
	for i := 0; i < 2; i++ {
		again, err := f.s.RefreshAsync(f.ctx, started.Request.ID)
		if err != nil || again.ChargedATG.String() != "0.04" {
			t.Fatal("polling changed charge", err)
		}
		video, err := f.s.DownloadAsync(f.ctx, f.customer, started.Request.ID)
		if err != nil || len(video.Binary) == 0 {
			t.Fatal("original video download failed", err)
		}
	}
	if _, err = f.s.DownloadAsync(f.ctx, uuid.NewString(), started.Request.ID); err == nil {
		t.Fatal("other customer downloaded task")
	}
	var attempts, postings int64
	if err = f.db.Model(&AttemptRecord{}).Where("request_id=?", started.Request.ID).Count(&attempts).Error; err != nil || attempts != 1 {
		t.Fatal("task resubmitted", attempts, err)
	}
	if err = f.db.Table("ledger_journals").Where("operator_id=? AND business_key=?", f.operator, "usage:"+started.Request.ID).Count(&postings).Error; err != nil || postings != 1 {
		t.Fatal("task double billed", postings, err)
	}
	f.s.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	if _, err = f.s.DownloadAsync(f.ctx, f.customer, started.Request.ID); err == nil {
		t.Fatal("provider retention bypassed local retention")
	}
}
