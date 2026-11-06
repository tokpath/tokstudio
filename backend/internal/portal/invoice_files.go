package portal

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"io"
	"mime"
	"net/http"
)

func (a *Application) invoiceFiles(c *gin.Context) {
	ctx := c.Request.Context()
	id := subject(c)
	operator := c.Param("operatorId")
	customer := id.Session.Kind == "customer"
	buyer := c.Param("resource") == "procurement-invoices"
	actor := id.Session.SubjectID
	if customer {
		invoice, err := a.Operations.CustomerInvoice(ctx, actor, c.Param("id"))
		if err != nil {
			handleError(c, err)
			return
		}
		operator = invoice.OperatorID
	}
	if !customer {
		resource := "invoices"
		if buyer {
			resource = "procurement-invoices"
		}
		if _, _, ok := a.staffScope(c, resource); !ok {
			return
		}
	}
	buyerID := ""
	if buyer {
		buyerID = operator
	}
	application, e := a.Operations.ReadInvoiceApplication(ctx, actor, operator, c.Param("id"), customer, buyerID)
	if e != nil {
		handleError(c, e)
		return
	}
	filesAvailable := !customer && !buyer || application.Invoice.State == "issued" || application.Invoice.State == "review_required" || application.Invoice.State == "voided"
	var rows []operations.InvoiceFile
	if filesAvailable {
		if buyer {
			rows, e = a.Operations.ListProcurementInvoiceFiles(ctx, actor, operator, c.Param("id"))
		} else {
			rows, e = a.Operations.ListInvoiceFiles(ctx, actor, operator, c.Param("id"), customer)
		}
		if e != nil {
			handleError(c, e)
			return
		}
	}
	if rows == nil {
		rows = []operations.InvoiceFile{}
	}
	base := "/api/v1/me/invoices"
	if !customer {
		base = "/api/v1/operators/" + operator + "/invoices"
		if buyer {
			base = "/api/v1/operators/" + operator + "/procurement-invoices"
		}
	}
	items := []gin.H{}
	for _, r := range rows {
		items = append(items, gin.H{"id": r.ID, "filename": r.Filename, "content_type": r.ContentType, "size_bytes": r.SizeBytes, "state": r.State, "created_at": r.CreatedAt, "download_url": base + "/" + c.Param("id") + "/files/" + r.ID})
	}
	orders, e := a.invoiceApplicationOrders(c, application)
	if e != nil {
		handleError(c, e)
		return
	}
	result := gin.H{"items": items, "details": application.Details, "orders": orders, "files_available": filesAvailable}
	c.JSON(200, result)
}
func (a *Application) downloadInvoiceFile(c *gin.Context) {
	ctx := c.Request.Context()
	id := subject(c)
	operator := c.Param("operatorId")
	customer := id.Session.Kind == "customer"
	buyer := c.Param("resource") == "procurement-invoices"
	actor := id.Session.SubjectID
	if customer {
		invoice, err := a.Operations.CustomerInvoice(ctx, actor, c.Param("id"))
		if err != nil {
			handleError(c, err)
			return
		}
		operator = invoice.OperatorID
	}
	var out operations.InvoiceDownload
	var e error
	if buyer {
		if _, _, ok := a.staffScope(c, "procurement-invoices"); !ok {
			return
		}
		out, e = a.Operations.DownloadProcurementInvoiceFile(ctx, actor, operator, c.Param("id"), c.Param("fileID"))
	} else {
		if !customer {
			if _, _, ok := a.staffScope(c, "invoices"); !ok {
				return
			}
		}
		out, e = a.Operations.DownloadInvoiceFile(ctx, actor, operator, c.Param("id"), c.Param("fileID"), customer)
	}
	if e != nil {
		handleError(c, e)
		return
	}
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": out.Filename}))
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Data(200, out.ContentType, out.Data)
}
func (a *Application) uploadInvoiceFile(c *gin.Context) {
	op, actor, ok := a.staffScope(c, "invoices")
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 11<<20)
	if e := c.Request.ParseMultipartForm(11 << 20); e != nil {
		handleError(c, operations.ErrInvalid)
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	file, header, e := c.Request.FormFile("file")
	if e != nil {
		handleError(c, operations.ErrInvalid)
		return
	}
	defer file.Close()
	data, e := io.ReadAll(io.LimitReader(file, operations.MaxInvoiceBytes+1))
	if e != nil || len(data) > operations.MaxInvoiceBytes {
		handleError(c, operations.ErrInvalid)
		return
	}
	key, _ := key(c)
	out, e := a.Operations.UploadInvoiceFile(c.Request.Context(), actor, op.ID, c.Param("id"), key, operations.InvoiceUpload{Filename: header.Filename, ContentType: header.Header.Get("Content-Type"), Data: data})
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(out.ID, out.State))
}
func (a *Application) deleteInvoiceFile(c *gin.Context) {
	op, actor, ok := a.staffScope(c, "invoices")
	if !ok {
		return
	}
	if err := a.Operations.DeleteInvoiceFile(c.Request.Context(), actor, op.ID, c.Param("id"), c.Param("fileID")); err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, commandResult(c.Param("fileID"), "removed"))
}
func (a *Application) replaceInvoice(c *gin.Context) {
	raw, ok := decodeConfiguration(c)
	if !ok {
		return
	}
	var in struct {
		Lines   []operations.InvoiceLine  `json:"lines"`
		Details operations.InvoiceDetails `json:"details"`
	}
	if json.Unmarshal(raw, &in) != nil {
		handleError(c, operations.ErrInvalid)
		return
	}
	id := subject(c)
	key, _ := key(c)
	var out operations.Invoice
	var e error
	if id.Session.Kind == "customer" {
		invoice, err := a.Operations.CustomerInvoice(c.Request.Context(), id.Customer.Customer.ID, c.Param("id"))
		if err != nil {
			handleError(c, err)
			return
		}

		out, e = a.Operations.RequestInvoiceReplacement(c.Request.Context(), id.Customer.Customer.ID, invoice.OperatorID, c.Param("id"), key, in.Lines, in.Details)
	} else {
		op, actor, ok := a.staffScope(c, "procurement-invoices")
		if !ok {
			return
		}
		out, e = a.Operations.RequestProcurementInvoiceReplacement(c.Request.Context(), actor, op.ID, c.Param("id"), key, in.Lines, in.Details)
	}
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(out.ID, out.State))
}
func (a *Application) procurementInvoicesResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	ctx := c.Request.Context()
	matches, e := a.invoiceSearchParties(c, op.ID, true)
	if e != nil {
		handleError(c, e)
		return
	}
	rows, next, e := a.Operations.ListProcurementInvoicesPage(ctx, actor, op.ID, c.Query("cursor"), c.Query("q"), c.Query("status"), 50, matches...)
	p["next_cursor"] = next
	p["search_supported"], p["status_supported"] = true, true
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, row := range rows {
		r, err := a.invoiceEntity(c, row)
		if err != nil {
			handleError(c, err)
			return
		}
		r["allowed_actions"] = []string{}
		if row.State == "voided" {
			policy, err := a.Operations.GetProcurementInvoicePolicy(ctx, actor, op.ID, row.OperatorID)
			if err != nil {
				handleError(c, err)
				return
			}
			if policy.Enabled {
				r["allowed_actions"] = []string{"replacement"}
			}
		}
		items = append(items, r)
	}
	p["title"] = "我方收票"
	p["description"] = "选择已完成的采购订单，按订单实付金额全额申请，由实际供货方办理。同一订单不能重复开票。"
	p["items"] = items
	p["columns"] = []gin.H{col("issuer_name", "实际供货开票方", "text"), col("amount_money", "开票金额", "money"), col("invoice_reference", "发票凭证编号", "text"), col("reason", "说明", "text")}
	fields := businessFields(operation{Path: "/api/v1/me/invoices", Fields: []field{jsonField("")}})
	p["operations"] = []operation{{Name: "request", Label: "向实际供货方申请采购发票", Method: "POST", Path: base, Fields: append([]field{optional("supplier_id", "实际供货方", "hidden")}, fields...)}}
	p["row_operations"] = []operation{{Name: "replacement", Label: "按原订单申请重开", Method: "POST", Path: base + "/{id}/replacement", Fields: fields}}
	if err := a.procurementInvoiceAvailability(c, op, p); err != nil {
		handleError(c, err)
		return
	}
	a.resourceResponse(c, p)
}
func (a *Application) procurementInvoicesCommand(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		SupplierID string                    `json:"supplier_id"`
		Lines      []operations.InvoiceLine  `json:"lines"`
		Details    operations.InvoiceDetails `json:"details"`
	}
	if !body(c, &in) {
		return
	}
	key, _ := key(c)
	out, e := a.Operations.RequestProcurementInvoice(c.Request.Context(), actor, op.ID, in.SupplierID, key, in.Lines, in.Details)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(out.ID, out.State))
}
