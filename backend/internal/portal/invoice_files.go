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
	var rows []operations.InvoiceFile
	var e error
	if buyer {
		if _, _, ok := a.staffScope(c, "procurement-invoices"); !ok {
			return
		}
		rows, e = a.Operations.ListProcurementInvoiceFiles(ctx, actor, operator, c.Param("id"))
	} else {
		if !customer {
			if _, _, ok := a.staffScope(c, "invoices"); !ok {
				return
			}
		}
		rows, e = a.Operations.ListInvoiceFiles(ctx, actor, operator, c.Param("id"), customer)
	}
	if e != nil {
		handleError(c, e)
		return
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
	result := gin.H{"items": items}
	if !customer && !buyer {
		details, e := a.Operations.InvoiceDetails(ctx, actor, operator, c.Param("id"))
		if e != nil {
			handleError(c, e)
			return
		}
		result["details"] = details
	}
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
	rows, e := a.Operations.ListProcurementInvoices(ctx, actor, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, row := range rows {
		r := entity(row.ID, row.State, 0, row)
		r["allowed_actions"] = []string{}
		if row.State == "voided" {
			r["allowed_actions"] = []string{"replacement"}
		}
		items = append(items, r)
	}
	p["title"] = "我方采购开票"
	p["description"] = "采购发票按经营方作为独立买方申请，由实际供货方办理。原收款开票容量与客户票据共享去重，已作废票据可按原来源重开。"
	p["items"] = items
	p["columns"] = []gin.H{col("operator_id", "实际供货开票方", "text"), col("amount_usd", "责任USD", "usd"), col("invoice_reference", "发票凭证编号", "text"), col("reason", "说明", "text")}
	fields := businessFields(operation{Path: "/api/v1/me/invoices", Fields: []field{jsonField("")}})
	p["operations"] = []operation{{Name: "request", Label: "向实际供货方申请采购发票", Method: "POST", Path: base, Fields: append([]field{f("supplier_id", "实际供货方编号", "text")}, fields...)}}
	p["row_operations"] = []operation{{Name: "replacement", Label: "作废后按原来源申请重开", Method: "POST", Path: base + "/{id}/replacement", Fields: fields}}
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
