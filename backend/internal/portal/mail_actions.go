package portal

import (
	"context"
	"errors"
	"net"
	"net/textproto"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"gorm.io/gorm"
)

func (a *Application) mailLanguage(c *gin.Context) {
	var in struct {
		OperatorID string `json:"operator_id"`
		Language   string `json:"language"`
	}
	if !body(c, &in) {
		return
	}
	id := subject(c)
	actor := id.Staff.ID
	if id.Session.Kind == "customer" {
		actor = id.Customer.Customer.ID
		in.OperatorID = id.Customer.Ownership.OperatorID
	}
	if uuid.Validate(in.OperatorID) != nil {
		handleError(c, organization.ErrInvalidInput)
		return
	}
	if err := a.Operations.SetMailLanguage(c.Request.Context(), actor, in.OperatorID, in.Language); err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, commandResult(actor, "saved"))
}

type testMailSendError struct{ cause error }

func (e *testMailSendError) Error() string { return "test mail submission failed" }
func (e *testMailSendError) Unwrap() error { return e.cause }

func mailTestFailure(err error) string {
	if errors.Is(err, mail.ErrUnavailable) {
		return "邮件服务尚未就绪，请先保存并启用邮件配置。"
	}
	var network net.Error
	if errors.As(err, &network) {
		return "无法连接邮件服务器，请核对 SMTP 主机、端口和网络连接。"
	}
	var smtpError *textproto.Error
	if errors.As(err, &smtpError) {
		if smtpError.Code == 535 || smtpError.Code == 534 || smtpError.Code == 530 {
			return "邮件服务器认证失败，请核对账号和 SMTP 密码或授权码。"
		}
		return "邮件服务器未接受邮件，请核对已授权发信地址、收件邮箱及服务商的发送限制。"
	}
	return "测试邮件提交失败，请核对加密方式和邮件配置；如仍失败，请联系邮件服务商。"
}

func (a *Application) testMail(c *gin.Context) {
	op, actor, ok := a.staffScope(c, "mail")
	if !ok {
		return
	}
	if op.Kind == organization.Channel {
		handleError(c, organization.ErrForbidden)
		return
	}
	var in struct{}
	if !body(c, &in) {
		return
	}
	if a.throttle(c.Request.Context(), "mail-test:"+op.ID+":"+actor) != nil {
		fail(c, 429, "auth_rate", "请求过于频繁", false)
		return
	}
	brand, err := a.Organization.GetBrand(c.Request.Context(), nil, op.BrandID)
	if err != nil {
		handleError(c, err)
		return
	}
	locale := httpx.RequestLocale(c.Request)
	raw, err := a.command(c, "mail-test:"+op.ID+":"+actor, in, func(tx *gorm.DB) (any, error) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		message := mail.Message{ID: uuid.NewString(), BrandID: op.BrandID, Brand: brand.Name, To: subject(c).Staff.Email, Subject: httpx.LocalizedText(locale, "邮件服务测试"), Text: httpx.LocalizedText(locale, "这是一封测试邮件。收到此邮件，说明当前品牌的邮件服务已能向你的邮箱发送邮件。")}
		if err := a.Mail.Send(ctx, message); err != nil {
			return nil, &testMailSendError{cause: err}
		}
		return gin.H{"id": op.ID, "status": "accepted", "message": "邮件服务器已接受测试邮件，请到当前账号邮箱查收（也请检查垃圾邮件）。"}, nil
	})
	if err != nil {
		var sendError *testMailSendError
		if errors.As(err, &sendError) {
			fail(c, 400, "mail_test_failed", mailTestFailure(sendError), false)
		} else {
			handleError(c, err)
		}
		return
	}
	jsonResponse(c, raw, nil)
}
